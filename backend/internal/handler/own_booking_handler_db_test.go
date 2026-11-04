package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// EVENTHUB-236: the own booking endpoints against a real database. Bookings are made through the
// cart, checked out, paid, declined and cancelled through the API like a visitor does; only
// states the API cannot reach (an expired reservation, a deleted event or location, events in
// the past) are set up with SQL.

func newOwnBookingRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := newCartAPI(db)
	api.register(r, principalSetter(principal))
	own := NewOwnBookingHandler(service.NewOwnBookingService(repository.NewOwnBookingRepository(db)), api.userRepo)

	setPrincipal := principalSetter(principal)
	r.GET("/api/v1/me/tickets", setPrincipal, own.ListOwnTicketsHandler)
	r.GET("/api/v1/me/bookings", setPrincipal, own.ListOwnBookingsHandler)
	r.GET("/api/v1/me/bookings/:bookingId", setPrincipal, own.GetOwnBookingHandler)
	return r
}

// payViaAPI checks the cart of the caller out and completes the payment with the outcome,
// confirming the amount shown at the checkout like the frontend does.
func payViaAPI(t *testing.T, router http.Handler, outcome model.SimulatedPaymentOutcome) model.PaymentResponse {
	t.Helper()
	started := checkoutViaAPI(t, router)
	return simulateViaAPI(t, router, started.PaymentID, outcome, started.Amount)
}

func simulateViaAPI(t *testing.T, router http.Handler, paymentID uuid.UUID, outcome model.SimulatedPaymentOutcome, amount decimal.Decimal) model.PaymentResponse {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodPost, paymentPath(paymentID)+"/simulate", map[string]any{"outcome": outcome, "amount": amount.String()})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate %s: %d %s", outcome, rec.Code, rec.Body.String())
	}
	return decodePayment(t, rec)
}

// paymentIDOf reads the payment a booking belongs to; false when it has none.
func paymentIDOf(t *testing.T, db *gorm.DB, bookingID uuid.UUID) (uuid.UUID, bool) {
	t.Helper()
	var id uuid.NullUUID
	if err := db.Raw("SELECT payment_id FROM bookings WHERE booking_id = ?", bookingID).Scan(&id).Error; err != nil {
		t.Fatalf("read payment of booking %s: %v", bookingID, err)
	}
	return id.UUID, id.Valid
}

func cancelViaAPI(t *testing.T, router http.Handler, bookingID uuid.UUID) {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/bookings/"+bookingID.String()+"/cancel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body.String())
	}
}

func withdrawViaAPI(t *testing.T, db *gorm.DB, eventID uuid.UUID) {
	t.Helper()
	if rec := postWithdraw(t, newEventRouter(db, principalManaging("org-a")), eventID.String()); rec.Code != http.StatusOK {
		t.Fatalf("withdraw: %d %s", rec.Code, rec.Body.String())
	}
}

// expireOwnReservation lets the reservation run out, as if its 15 minutes had passed.
func expireOwnReservation(t *testing.T, db *gorm.DB, bookingID uuid.UUID) {
	t.Helper()
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", bookingID).Error; err != nil {
		t.Fatalf("expire reservation: %v", err)
	}
}

func getOwnBookingList(t *testing.T, router http.Handler, path string) []model.OwnBookingResponse {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodGet, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	var list []model.OwnBookingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || list == nil {
		t.Fatalf("GET %s: expected a JSON array, got %s (%v)", path, rec.Body.String(), err)
	}
	return list
}

func getOwnBookingDetail(t *testing.T, router http.Handler, bookingID uuid.UUID) model.OwnBookingResponse {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodGet, "/api/v1/me/bookings/"+bookingID.String(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET own booking %s: %d %s", bookingID, rec.Code, rec.Body.String())
	}
	var booking model.OwnBookingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &booking); err != nil {
		t.Fatalf("decode own booking: %v", err)
	}
	return booking
}

func findOwnBooking(list []model.OwnBookingResponse, bookingID uuid.UUID) *model.OwnBookingResponse {
	for i := range list {
		if list[i].BookingID == bookingID {
			return &list[i]
		}
	}
	return nil
}

func ownBookingIDs(list []model.OwnBookingResponse) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(list))
	for _, b := range list {
		ids = append(ids, b.BookingID)
	}
	return ids
}

func ownBookingJSON(t *testing.T, booking model.OwnBookingResponse) string {
	t.Helper()
	data, err := json.Marshal(booking)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(data)
}

// A1, A3, A5, C1, C2, D1, D3: a booking paid through the API is a ticket with its event, location
// and payment, and its bookingId opens the same data as details.
func TestListOwnTickets_PaidViaAPI_AppearsWithEventLocationAndPayment(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	bookingID := addToCartViaAPI(t, router, event.eventID, 2)
	paid := payViaAPI(t, router, model.SimulatedPaymentSuccess)

	tickets := getOwnBookingList(t, router, "/api/v1/me/tickets")
	if len(tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d: %+v", len(tickets), tickets)
	}
	ticket := tickets[0]
	if ticket.BookingID != bookingID || ticket.NumberOfTickets != 2 || !ticket.TicketsValid ||
		ticket.BookingStatus != model.BookingStatusConfirmed || ticket.BookedAt.IsZero() || ticket.ExpiresAt != nil {
		t.Errorf("unexpected booking data: %s", ownBookingJSON(t, ticket))
	}
	if ticket.PaymentStatus == nil || *ticket.PaymentStatus != model.PaymentStatusPaid || ticket.Amount == nil ||
		!ticket.Amount.Equal(decimal.NewFromInt(40)) || ticket.FailureReason != nil || ticket.Message != paid.Message {
		t.Errorf("expected paid payment of 40 with the message of the payment result, got %s", ownBookingJSON(t, ticket))
	}
	ev := ticket.Event
	if ev == nil || ev.EventID != event.eventID || ev.Title != "Altes Konzert" || ev.Status != model.EventStatusPublished ||
		ev.StartTime.IsZero() || !ev.EndTime.After(ev.StartTime) {
		t.Fatalf("unexpected event: %+v", ev)
	}
	loc := ev.Location
	if loc == nil || loc.LocationID != event.locationID || loc.Name != "loc-"+event.locationID.String() ||
		loc.City != "Bonn" || loc.PostalCode != "53111" || loc.Street != "Weg" {
		t.Errorf("unexpected location: %+v", loc)
	}

	// A5: the list entry opens the details of the same booking.
	detail := getOwnBookingDetail(t, router, ticket.BookingID)
	if got, want := ownBookingJSON(t, detail), ownBookingJSON(t, ticket); got != want {
		t.Errorf("details differ from the list entry:\n got  %s\n want %s", got, want)
	}
}

// A4, B1, B3, D2: a declined payment and open reservations are no tickets, but the history shows
// them with their status. A live cart item and a pending checkout cannot coexist, so the item is
// checked out in the second half of the test.
func TestListOwnBookings_DeclinedAndOpenBookings_InHistoryNotInTickets(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	other := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	declined := addToCartViaAPI(t, router, event.eventID, 2)
	payViaAPI(t, router, model.SimulatedPaymentFailure)
	live := addToCartViaAPI(t, router, other.eventID, 1)

	if tickets := getOwnBookingList(t, router, "/api/v1/me/tickets"); len(tickets) != 0 {
		t.Fatalf("expected no tickets, got %+v", tickets)
	}
	history := getOwnBookingList(t, router, "/api/v1/me/bookings")
	if len(history) != 2 {
		t.Fatalf("expected 2 bookings in the history, got %d", len(history))
	}

	failed := findOwnBooking(history, declined)
	if failed == nil || failed.BookingStatus != model.BookingStatusFailed || failed.TicketsValid ||
		failed.PaymentStatus == nil || *failed.PaymentStatus != model.PaymentStatusFailed ||
		failed.FailureReason == nil || *failed.FailureReason != model.PaymentFailureDeclined || failed.Message == "" {
		t.Errorf("expected failed booking with failureReason declined, got %+v", failed)
	}
	open := findOwnBooking(history, live)
	if open == nil || open.BookingStatus != model.BookingStatusReserved || open.TicketsValid ||
		open.PaymentStatus != nil || open.Amount != nil || open.ExpiresAt == nil || open.Message != "" {
		t.Errorf("expected live reservation without payment, got %+v", open)
	}

	// The live item becomes a pending checkout: still no ticket, but a payment to complete.
	checkoutViaAPI(t, router)
	history = getOwnBookingList(t, router, "/api/v1/me/bookings")
	awaiting := findOwnBooking(history, live)
	if awaiting == nil || awaiting.BookingStatus != model.BookingStatusReserved || awaiting.TicketsValid ||
		awaiting.PaymentStatus == nil || *awaiting.PaymentStatus != model.PaymentStatusPending || awaiting.ExpiresAt == nil {
		t.Errorf("expected reservation with pending payment, got %+v", awaiting)
	}
}

// B3, C3: a paid booking cancelled through the API leaves the ticket list and shows as cancelled
// and refunded.
func TestListOwnBookings_CancelledAfterPayment_CancelledRefundedNotInTickets(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	bookingID := addToCartViaAPI(t, router, event.eventID, 2)
	payViaAPI(t, router, model.SimulatedPaymentSuccess)
	if tickets := getOwnBookingList(t, router, "/api/v1/me/tickets"); len(tickets) != 1 {
		t.Fatalf("expected the paid ticket before cancelling, got %+v", tickets)
	}
	cancelViaAPI(t, router, bookingID)

	if tickets := getOwnBookingList(t, router, "/api/v1/me/tickets"); len(tickets) != 0 {
		t.Errorf("a cancelled booking must leave the ticket list, got %+v", tickets)
	}
	entry := findOwnBooking(getOwnBookingList(t, router, "/api/v1/me/bookings"), bookingID)
	if entry == nil || entry.BookingStatus != model.BookingStatusCancelled || entry.TicketsValid ||
		entry.PaymentStatus == nil || *entry.PaymentStatus != model.PaymentStatusRefunded ||
		entry.RefundAmount == nil || !entry.RefundAmount.Equal(decimal.NewFromInt(40)) || entry.ExpiresAt != nil {
		t.Errorf("expected cancelled and refunded booking in the history, got %+v", entry)
	}
	detail := getOwnBookingDetail(t, router, bookingID)
	if detail.TicketsValid || detail.BookingStatus != model.BookingStatusCancelled {
		t.Errorf("details must mark the cancelled booking, got %s", ownBookingJSON(t, detail))
	}
}

// A4, C3 (F5): tickets of an event withdrawn after the payment are no longer valid.
func TestGetOwnBooking_EventWithdrawnAfterPayment_TicketInvalid(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	bookingID := addToCartViaAPI(t, router, event.eventID, 2)
	paid := payViaAPI(t, router, model.SimulatedPaymentSuccess)
	withdrawViaAPI(t, db, event.eventID)

	if tickets := getOwnBookingList(t, router, "/api/v1/me/tickets"); len(tickets) != 0 {
		t.Errorf("tickets of a withdrawn event must leave the ticket list, got %+v", tickets)
	}
	detail := getOwnBookingDetail(t, router, bookingID)
	if detail.TicketsValid || detail.BookingStatus != model.BookingStatusConfirmed ||
		detail.PaymentStatus == nil || *detail.PaymentStatus != model.PaymentStatusPaid ||
		detail.Event == nil || detail.Event.Status != model.EventStatusCancelled {
		t.Errorf("expected invalid ticket of a cancelled event, got %s", ownBookingJSON(t, detail))
	}
	payment := decodePayment(t, doPaymentRequest(t, router, http.MethodGet, paymentPath(paid.PaymentID), nil))
	if payment.TicketsValid || payment.Message != detail.Message {
		t.Errorf("expected the payment result to agree, got ticketsValid=%v message=%q", payment.TicketsValid, payment.Message)
	}
}

// A2, B4, C4, D4: bookings of other visitors are neither listed nor readable, and look like
// unknown bookings.
func TestOwnBookings_ForeignBookings_NotListedAndDetail404(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	mine := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	theirs := newOwnBookingRouter(db, &middleware.Principal{Subject: otherBookingKeycloakUserID})

	own := addToCartViaAPI(t, mine, event.eventID, 1)
	payViaAPI(t, mine, model.SimulatedPaymentSuccess)
	foreignPaid := addToCartViaAPI(t, theirs, event.eventID, 2)
	payViaAPI(t, theirs, model.SimulatedPaymentSuccess)
	foreignFailed := addToCartViaAPI(t, theirs, event.eventID, 1)
	payViaAPI(t, theirs, model.SimulatedPaymentFailure)

	for _, path := range []string{"/api/v1/me/tickets", "/api/v1/me/bookings"} {
		if ids := ownBookingIDs(getOwnBookingList(t, mine, path)); len(ids) != 1 || ids[0] != own {
			t.Errorf("%s: expected only the own booking %s, got %v", path, own, ids)
		}
	}
	if ids := ownBookingIDs(getOwnBookingList(t, theirs, "/api/v1/me/bookings")); len(ids) != 2 || ids[0] != foreignFailed || ids[1] != foreignPaid {
		t.Errorf("the other visitor must see both own bookings, newest first, got %v", ids)
	}

	foreign := doPaymentRequest(t, mine, http.MethodGet, "/api/v1/me/bookings/"+foreignPaid.String(), nil)
	unknown := doPaymentRequest(t, mine, http.MethodGet, "/api/v1/me/bookings/"+uuid.NewString(), nil)
	wantProblem(t, foreign, http.StatusNotFound)
	wantProblem(t, unknown, http.StatusNotFound)
	if foreign.Body.String() != unknown.Body.String() {
		t.Errorf("a foreign booking must look like an unknown one: %s vs %s", foreign.Body.String(), unknown.Body.String())
	}
	wantProblem(t, doPaymentRequest(t, mine, http.MethodGet, "/api/v1/me/bookings/not-a-uuid", nil), http.StatusBadRequest)
}

// ownBookingStates makes bookings in every state the API can reach and returns them by name.
// A visitor holds at most one open checkout at a time, so the pending payment is created last;
// the states with a closed payment come first. A reservation that ran out is covered by
// TestListOwnBookings_ExpiredReservation_ShownExpiredAndFiltered, which needs its own flow.
func ownBookingStates(t *testing.T, db *gorm.DB, router http.Handler) map[string]uuid.UUID {
	t.Helper()
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	withdrawn := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	cartEvent := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	states := map[string]uuid.UUID{}

	// cycle books two tickets, pays them and records the booking of the closed payment.
	cycle := func(name string, eventID uuid.UUID, outcome model.SimulatedPaymentOutcome) uuid.UUID {
		t.Helper()
		addToCartViaAPI(t, router, eventID, 2)
		paid := payViaAPI(t, router, outcome)
		if len(paid.Items) != 1 {
			t.Fatalf("%s: unexpected checkout: %+v", name, paid.Items)
		}
		states[name] = paid.Items[0].BookingID
		return states[name]
	}

	cycle("paid", event.eventID, model.SimulatedPaymentSuccess)
	cycle("declined", event.eventID, model.SimulatedPaymentFailure)
	cycle("cancelled after payment", event.eventID, model.SimulatedPaymentSuccess)
	cancelViaAPI(t, router, states["cancelled after payment"])
	addToCartViaAPI(t, router, event.eventID, 2)
	cancelledPending := checkoutViaAPI(t, router)
	states["cancelled while pending"] = cancelledPending.Items[0].BookingID
	cancelViaAPI(t, router, states["cancelled while pending"])
	cycle("paid, event withdrawn", withdrawn.eventID, model.SimulatedPaymentSuccess)
	withdrawViaAPI(t, db, withdrawn.eventID)

	// The last checkout stays open, and a cart item lives beside it.
	addToCartViaAPI(t, router, event.eventID, 2)
	pending := checkoutViaAPI(t, router)
	states["pending"] = pending.Items[0].BookingID
	states["reserved without payment"] = addToCartViaAPI(t, router, cartEvent.eventID, 2)
	return states
}

// C5: for every booking with a payment, the details say what GET /payments/{paymentId} says.
func TestGetOwnBooking_BookingsWithPayment_MatchPaymentEndpoint(t *testing.T) {
	db := testdb.Open(t)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	states := ownBookingStates(t, db, router)

	compared, valid := 0, 0
	for name, bookingID := range states {
		paymentID, has := paymentIDOf(t, db, bookingID)
		if !has {
			continue // no payment process was started
		}
		rec := doPaymentRequest(t, router, http.MethodGet, paymentPath(paymentID), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: get payment: %d %s", name, rec.Code, rec.Body.String())
		}
		payment := decodePayment(t, rec)
		item := paymentItemOf(&payment, bookingID)
		if item == nil {
			t.Fatalf("%s: booking %s is not an item of its payment", name, bookingID)
		}
		own := getOwnBookingDetail(t, router, bookingID)
		compared++
		if own.TicketsValid {
			valid++
		}
		if own.TicketsValid != payment.TicketsValid || own.Message != payment.Message {
			t.Errorf("%s: ticketsValid=%v message=%q, payment endpoint says ticketsValid=%v message=%q",
				name, own.TicketsValid, own.Message, payment.TicketsValid, payment.Message)
		}
		if own.PaymentStatus == nil || *own.PaymentStatus != payment.Status {
			t.Errorf("%s: payment %v, payment endpoint says %s", name, own.PaymentStatus, payment.Status)
		}
		if own.Amount == nil || !own.Amount.Equal(item.LineAmount) {
			t.Errorf("%s: amount %v, payment endpoint says line %s", name, own.Amount, item.LineAmount)
		}
		if (own.FailureReason == nil) != (payment.FailureReason == nil) || (own.FailureReason != nil && *own.FailureReason != *payment.FailureReason) {
			t.Errorf("%s: failureReason %v, payment endpoint says %v", name, own.FailureReason, payment.FailureReason)
		}
		if own.BookingStatus != item.BookingStatus {
			t.Errorf("%s: bookingStatus %s, payment endpoint says %s", name, own.BookingStatus, item.BookingStatus)
		}
	}
	if compared != len(states)-1 || valid != 1 {
		t.Errorf("expected %d bookings with payment, one of them valid; compared %d with %d valid", len(states)-1, compared, valid)
	}
}

// B5, C5, D5: ticket list, history and details show one booking identically, because all three
// read the same data and apply the same rule.
func TestOwnBookings_AllViews_ShowSameBooking(t *testing.T) {
	db := testdb.Open(t)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	states := ownBookingStates(t, db, router)

	history := getOwnBookingList(t, router, "/api/v1/me/bookings")
	if len(history) != len(states) {
		t.Fatalf("expected %d bookings in the history, got %d", len(states), len(history))
	}
	validInHistory := map[uuid.UUID]string{}
	for _, entry := range history {
		if got, want := ownBookingJSON(t, getOwnBookingDetail(t, router, entry.BookingID)), ownBookingJSON(t, entry); got != want {
			t.Errorf("details differ from the history:\n got  %s\n want %s", got, want)
		}
		if entry.TicketsValid {
			validInHistory[entry.BookingID] = ownBookingJSON(t, entry)
		}
	}
	tickets := getOwnBookingList(t, router, "/api/v1/me/tickets")
	if len(tickets) != len(validInHistory) || len(tickets) != 1 || tickets[0].BookingID != states["paid"] {
		t.Fatalf("expected exactly the paid booking as ticket, got %v", ownBookingIDs(tickets))
	}
	for _, ticket := range tickets {
		if ownBookingJSON(t, ticket) != validInHistory[ticket.BookingID] {
			t.Errorf("ticket differs from its history entry: %s", ownBookingJSON(t, ticket))
		}
	}
}

// B3, D2 (F4): a reservation that ran out is shown as expired although it is stored as reserved,
// and the status filter finds it next to stored expired bookings.
func TestListOwnBookings_ExpiredReservation_ShownExpiredAndFiltered(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	second := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	third := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	ranOut := addToCartViaAPI(t, router, event.eventID, 1)
	expireOwnReservation(t, db, ranOut)
	storedExpired := addToCartViaAPI(t, router, second.eventID, 1)
	started := checkoutViaAPI(t, router)
	expireOwnReservation(t, db, storedExpired)
	simulateViaAPI(t, router, started.PaymentID, model.SimulatedPaymentSuccess, started.Amount)
	paid := addToCartViaAPI(t, router, second.eventID, 1)
	payViaAPI(t, router, model.SimulatedPaymentSuccess)
	live := addToCartViaAPI(t, router, third.eventID, 1)

	var stored string
	if err := db.Raw("SELECT status FROM bookings WHERE booking_id = ?", ranOut).Scan(&stored).Error; err != nil || stored != "reserved" {
		t.Fatalf("the reservation without payment stays stored as reserved, got %q (%v)", stored, err)
	}
	entry := findOwnBooking(getOwnBookingList(t, router, "/api/v1/me/bookings"), ranOut)
	if entry == nil || entry.BookingStatus != model.BookingStatusExpired || entry.ExpiresAt != nil || entry.TicketsValid {
		t.Errorf("expected the reservation shown as expired, got %+v", entry)
	}

	cases := []struct {
		status string
		want   []uuid.UUID
	}{
		{"expired", []uuid.UUID{storedExpired, ranOut}},
		{"reserved", []uuid.UUID{live}},
		{"confirmed", []uuid.UUID{paid}},
		{"cancelled", []uuid.UUID{}},
	}
	for _, tc := range cases {
		got := ownBookingIDs(getOwnBookingList(t, router, "/api/v1/me/bookings?status="+tc.status))
		if len(got) != len(tc.want) {
			t.Errorf("status=%s: expected %v, got %v", tc.status, tc.want, got)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("status=%s: expected %v (newest first), got %v", tc.status, tc.want, got)
				break
			}
		}
	}
	wantProblem(t, doPaymentRequest(t, router, http.MethodGet, "/api/v1/me/bookings?status=storniert", nil), http.StatusBadRequest)
}

// B2: the history shows event, date, number of tickets and status of every booking, the latest
// booking first.
func TestListOwnBookings_History_ContainsEventDateTicketsStatusNewestFirst(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	first := addToCartViaAPI(t, router, event.eventID, 3)
	payViaAPI(t, router, model.SimulatedPaymentSuccess)
	second := addToCartViaAPI(t, router, event.eventID, 1)
	payViaAPI(t, router, model.SimulatedPaymentFailure)
	third := addToCartViaAPI(t, router, event.eventID, 2)

	history := getOwnBookingList(t, router, "/api/v1/me/bookings")
	want := []struct {
		id      uuid.UUID
		tickets int
		status  model.BookingStatus
	}{{third, 2, model.BookingStatusReserved}, {second, 1, model.BookingStatusFailed}, {first, 3, model.BookingStatusConfirmed}}
	if len(history) != len(want) {
		t.Fatalf("expected %d bookings, got %d", len(want), len(history))
	}
	for i, w := range want {
		got := history[i]
		if got.BookingID != w.id || got.NumberOfTickets != w.tickets || got.BookingStatus != w.status {
			t.Errorf("position %d: expected %s with %d tickets in status %s, got %s with %d in %s",
				i, w.id, w.tickets, w.status, got.BookingID, got.NumberOfTickets, got.BookingStatus)
		}
		if got.Event == nil || got.Event.Title == "" || got.Event.StartTime.IsZero() || got.BookedAt.IsZero() {
			t.Errorf("position %d: expected event title, start time and booking date, got %s", i, ownBookingJSON(t, got))
		}
		if i > 0 && got.BookedAt.After(history[i-1].BookedAt) {
			t.Errorf("position %d: booked after the entry above it", i)
		}
	}
}

// Ticket list order: upcoming events first by start time, then past events, the latest first. A
// running event counts as upcoming until it has ended.
func TestListOwnTickets_UpcomingFirstThenPast_OrderedByStart(t *testing.T) {
	db := testdb.Open(t)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	// Booked in a different order than expected, so the creation order cannot pass the test.
	times := []struct {
		name       string
		start, end string
	}{
		{"later", "5 days", "5 days 2 hours"},
		{"long ago", "-60 days", "-60 days 2 hours"},
		{"soon", "2 days", "2 days 2 hours"},
		{"recently", "-30 days", "-30 days 2 hours"},
		{"running", "-1 hour", "1 hour"},
	}
	booked := map[string]uuid.UUID{}
	for _, tt := range times {
		event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
		booked[tt.name] = addToCartViaAPI(t, router, event.eventID, 1)
		payViaAPI(t, router, model.SimulatedPaymentSuccess)
		err := db.Exec("UPDATE events SET start_time = NOW() + ?::interval, end_time = NOW() + ?::interval WHERE event_id = ?",
			tt.start, tt.end, event.eventID).Error
		if err != nil {
			t.Fatalf("move event %s: %v", tt.name, err)
		}
	}

	got := ownBookingIDs(getOwnBookingList(t, router, "/api/v1/me/tickets"))
	want := []uuid.UUID{booked["running"], booked["soon"], booked["later"], booked["recently"], booked["long ago"]}
	if len(got) != len(want) {
		t.Fatalf("expected %d tickets, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order running, soon, later, recently, long ago:\n got  %v\n want %v", got, want)
		}
	}
}

// F7: a deleted location shows as null and keeps the ticket valid; a deleted event shows as null
// and leaves no valid ticket, but the booking stays in the history. The API deletes neither, so
// SQL stands in for it.
func TestGetOwnBooking_DeletedLocationOrEvent_ShowsNull(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	bookingID := addToCartViaAPI(t, router, event.eventID, 1)
	payViaAPI(t, router, model.SimulatedPaymentSuccess)

	if err := db.Exec("DELETE FROM locations WHERE location_id = ?", event.locationID).Error; err != nil {
		t.Fatalf("delete location: %v", err)
	}
	detail := getOwnBookingDetail(t, router, bookingID)
	if detail.Event == nil || detail.Event.Location != nil || !detail.TicketsValid {
		t.Errorf("expected a valid ticket without location, got %s", ownBookingJSON(t, detail))
	}

	if err := db.Exec("DELETE FROM events WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatalf("delete event: %v", err)
	}
	detail = getOwnBookingDetail(t, router, bookingID)
	if detail.Event != nil || detail.TicketsValid {
		t.Errorf("expected an invalid ticket without event, got %s", ownBookingJSON(t, detail))
	}
	if tickets := getOwnBookingList(t, router, "/api/v1/me/tickets"); len(tickets) != 0 {
		t.Errorf("a booking without event is no ticket, got %+v", tickets)
	}
	if findOwnBooking(getOwnBookingList(t, router, "/api/v1/me/bookings"), bookingID) == nil {
		t.Error("the booking must stay in the history")
	}
}

func TestListOwnTickets_NoOwnBookings_ReturnsEmptyArray(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	theirs := newOwnBookingRouter(db, &middleware.Principal{Subject: otherBookingKeycloakUserID})
	addToCartViaAPI(t, theirs, event.eventID, 1)
	payViaAPI(t, theirs, model.SimulatedPaymentSuccess)

	mine := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	for _, path := range []string{"/api/v1/me/tickets", "/api/v1/me/bookings"} {
		rec := httptest.NewRecorder()
		mine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("%s: expected 200 with [], got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

// ratingScore of the ticket list and the details follows the rating of the caller: null until the
// event is rated through the rating API, then the given score. The event is moved into the past,
// because only ended events can be rated; validity does not depend on the event time.
func TestListOwnTickets_RatingScore_FollowsOwnRating(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newOwnBookingRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	bookingID := addToCartViaAPI(t, router, event.eventID, 1)
	payViaAPI(t, router, model.SimulatedPaymentSuccess)
	if err := db.Exec("UPDATE events SET start_time = NOW() - interval '2 day', end_time = NOW() - interval '1 day' WHERE event_id = ?",
		event.eventID).Error; err != nil {
		t.Fatalf("end event: %v", err)
	}

	before := findOwnBooking(getOwnBookingList(t, router, "/api/v1/me/tickets"), bookingID)
	if before == nil || before.RatingScore != nil {
		t.Errorf("expected ratingScore null before rating, got %s", ownBookingJSON(t, *before))
	}
	if detail := getOwnBookingDetail(t, router, bookingID); detail.RatingScore != nil {
		t.Errorf("details must report ratingScore null before rating, got %s", ownBookingJSON(t, detail))
	}

	rate := newRatingSubmissionRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	if rec := postEventRating(t, rate, event.eventID, `{"score":4}`); rec.Code != http.StatusCreated {
		t.Fatalf("rate: %d %s", rec.Code, rec.Body.String())
	}

	after := findOwnBooking(getOwnBookingList(t, router, "/api/v1/me/tickets"), bookingID)
	if after == nil || after.RatingScore == nil || *after.RatingScore != 4 {
		t.Errorf("expected ratingScore 4 after rating, got %s", ownBookingJSON(t, *after))
	}
	if detail := getOwnBookingDetail(t, router, bookingID); detail.RatingScore == nil || *detail.RatingScore != 4 {
		t.Errorf("details must report ratingScore 4 after rating, got %s", ownBookingJSON(t, detail))
	}
}
