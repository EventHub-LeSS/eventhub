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
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 282: Warenkorb — the cart endpoints against a real database. Adding tickets to the cart
// reserves their seats; the checkout turns all items into one payment, which is completed
// through the payment endpoints.

// principalSetter injects the principal of the caller like the auth middleware does.
func principalSetter(principal *middleware.Principal) gin.HandlerFunc {
	return func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}
}

// cartAPI bundles the handlers of the cart flow: cart, payments and cancellation.
type cartAPI struct {
	cart     *CartHandler
	payment  *PaymentHandler
	cancel   gin.HandlerFunc
	userRepo repository.UserRepository
}

func newCartAPI(db *gorm.DB) cartAPI {
	userRepo := repository.NewUserRepository(db)
	tx := repository.NewTransactor(db)
	bookingRepo := repository.NewBookingRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	return cartAPI{
		cart:     NewCartHandler(service.NewCartService(tx, bookingRepo, repository.NewCartRepository(db), paymentRepo, 0), userRepo),
		payment:  NewPaymentHandler(service.NewPaymentService(tx, bookingRepo, paymentRepo), userRepo),
		cancel:   CancelBooking(service.NewBookingService(tx, bookingRepo, paymentRepo), userRepo),
		userRepo: userRepo,
	}
}

// register mounts the cart flow on the router like cmd/api does.
func (api cartAPI) register(r *gin.Engine, setPrincipal gin.HandlerFunc) {
	r.GET("/api/v1/cart", setPrincipal, api.cart.GetCartHandler)
	r.DELETE("/api/v1/cart", setPrincipal, api.cart.ClearCartHandler)
	r.POST("/api/v1/cart/items", setPrincipal, api.cart.AddCartItemHandler)
	r.PATCH("/api/v1/cart/items/:bookingId", setPrincipal, api.cart.UpdateCartItemHandler)
	r.DELETE("/api/v1/cart/items/:bookingId", setPrincipal, api.cart.RemoveCartItemHandler)
	r.POST("/api/v1/cart/checkout", setPrincipal, api.cart.CheckoutHandler)
	r.GET("/api/v1/payments/:paymentId", setPrincipal, api.payment.GetPaymentHandler)
	r.POST("/api/v1/payments/:paymentId/simulate", setPrincipal, api.payment.SimulatePaymentHandler)
	r.POST("/api/v1/bookings/:bookingId/cancel", setPrincipal, api.cancel)
}

func newCartRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	newCartAPI(db).register(r, principalSetter(principal))
	return r
}

func decodeCart(t *testing.T, rec *httptest.ResponseRecorder) *model.CartResponse {
	t.Helper()
	var cart model.CartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &cart); err != nil {
		t.Fatalf("decode cart: %v: %s", err, rec.Body.String())
	}
	return &cart
}

// cartItemOf finds the cart item of the event, or nil.
func cartItemOf(cart *model.CartResponse, eventID uuid.UUID) *model.CartItemResponse {
	for i := range cart.Items {
		if item := cart.Items[i]; item.Event != nil && item.Event.EventID == eventID {
			return &cart.Items[i]
		}
	}
	return nil
}

// addToCartViaAPI puts tickets of the event in the cart of the caller and returns the booking ID
// of the item.
func addToCartViaAPI(t *testing.T, router http.Handler, eventID uuid.UUID, tickets int) uuid.UUID {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/items", map[string]any{"eventId": eventID, "quantity": tickets})
	if rec.Code != http.StatusCreated {
		t.Fatalf("add %d tickets of %s to the cart: %d %s", tickets, eventID, rec.Code, rec.Body.String())
	}
	item := cartItemOf(decodeCart(t, rec), eventID)
	if item == nil {
		t.Fatalf("no cart item for event %s", eventID)
	}
	return item.BookingID
}

func getCartViaAPI(t *testing.T, router http.Handler) *model.CartResponse {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodGet, "/api/v1/cart", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get cart: %d %s", rec.Code, rec.Body.String())
	}
	return decodeCart(t, rec)
}

// checkoutViaAPI checks the cart of the caller out and returns the payment, newly created or
// already open.
func checkoutViaAPI(t *testing.T, router http.Handler) *model.PaymentResponse {
	t.Helper()
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", nil)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("checkout: %d %s", rec.Code, rec.Body.String())
	}
	var payment model.PaymentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payment); err != nil {
		t.Fatalf("decode payment: %v: %s", err, rec.Body.String())
	}
	return &payment
}

func bookingCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.BookingModel{}).Count(&count).Error; err != nil {
		t.Fatalf("count bookings: %v", err)
	}
	return count
}

func TestAddCartItem_ReservesTicketsForTokenUser(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	userID := seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	// Client supplied userId and status are ignored: the reservation belongs to the token user.
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/items", map[string]any{
		"eventId":  event.eventID,
		"quantity": 2,
		"userId":   uuid.New(),
		"status":   "confirmed",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	cart := decodeCart(t, rec)
	if len(cart.Items) != 1 || cart.TotalTickets != 2 || !cart.TotalAmount.Equal(decimal.NewFromInt(40)) || !cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	item := cart.Items[0]
	if item.Event == nil || item.Event.EventID != event.eventID || !item.UnitPrice.Equal(decimal.NewFromInt(20)) ||
		!item.LineTotal.Equal(decimal.NewFromInt(40)) || item.AvailableSeats != 100 || !item.Bookable {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.ExpiresAt.Before(time.Now()) {
		t.Errorf("the reservation must be live, got expiry %v", item.ExpiresAt)
	}
	stored, err := repository.NewBookingRepository(db).GetBookingByID(item.BookingID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.UserID == nil || *stored.UserID != userID || stored.Status != model.BookingStatusReserved ||
		stored.NumberOfTickets != 2 || stored.PaymentID != nil || stored.ExpiresAt == nil {
		t.Fatalf("unexpected reservation: %+v", stored)
	}

	// Adding to the item again raises its quantity instead of creating a second reservation.
	rec = doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/items", map[string]any{"eventId": event.eventID, "quantity": 3})
	if rec.Code != http.StatusCreated {
		t.Fatalf("second add: %d %s", rec.Code, rec.Body.String())
	}
	cart = decodeCart(t, rec)
	if len(cart.Items) != 1 || cart.Items[0].BookingID != item.BookingID || cart.TotalTickets != 5 {
		t.Fatalf("the item must stay one reservation: %+v", cart)
	}
	if bookingCount(t, db) != 1 {
		t.Fatalf("expected one reservation, found %d", bookingCount(t, db))
	}
}

func TestAddCartItem_Rejections(t *testing.T) {
	db := testdb.Open(t)
	published := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	draft := seedEventForOrg(t, db, "org-a", model.EventStatusDraft)
	soldOut := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET capacity = 1 WHERE event_id = ?", soldOut.eventID).Error; err != nil {
		t.Fatal(err)
	}
	seedBookingUser(t, db, bookingKeycloakUserID)
	knownUser := &middleware.Principal{Subject: bookingKeycloakUserID}

	cases := []struct {
		name      string
		principal *middleware.Principal
		body      any
		want      int
	}{
		{"missing principal", nil, map[string]any{"eventId": published.eventID, "quantity": 1}, http.StatusUnauthorized},
		{"user not synced", &middleware.Principal{Subject: "kc-unknown"}, map[string]any{"eventId": published.eventID, "quantity": 1}, http.StatusForbidden},
		{"missing event id", knownUser, map[string]any{"quantity": 1}, http.StatusBadRequest},
		{"invalid event id", knownUser, map[string]any{"eventId": "not-a-uuid", "quantity": 1}, http.StatusBadRequest},
		{"missing quantity", knownUser, map[string]any{"eventId": published.eventID}, http.StatusBadRequest},
		{"zero quantity", knownUser, map[string]any{"eventId": published.eventID, "quantity": 0}, http.StatusBadRequest},
		{"negative quantity", knownUser, map[string]any{"eventId": published.eventID, "quantity": -3}, http.StatusBadRequest},
		{"quantity above the limit", knownUser, map[string]any{"eventId": published.eventID, "quantity": 11}, http.StatusUnprocessableEntity},
		{"unknown event", knownUser, map[string]any{"eventId": uuid.New(), "quantity": 1}, http.StatusNotFound},
		{"draft event", knownUser, map[string]any{"eventId": draft.eventID, "quantity": 1}, http.StatusConflict},
		{"capacity exceeded", knownUser, map[string]any{"eventId": soldOut.eventID, "quantity": 2}, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantProblem(t, doPaymentRequest(t, newCartRouter(db, tc.principal), http.MethodPost, "/api/v1/cart/items", tc.body), tc.want)
		})
	}
	if n := bookingCount(t, db); n != 0 {
		t.Errorf("rejected requests must not reserve tickets, found %d bookings", n)
	}
}

func TestGetCart_ShowsItemsPricesAndSkipsExpired(t *testing.T) {
	db := testdb.Open(t)
	music := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	food := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET price = 5 WHERE event_id = ?", food.eventID).Error; err != nil {
		t.Fatal(err)
	}
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	musicItem := addToCartViaAPI(t, router, music.eventID, 2)
	foodItem := addToCartViaAPI(t, router, food.eventID, 1)

	cart := getCartViaAPI(t, router)
	if len(cart.Items) != 2 || cart.TotalTickets != 3 || !cart.TotalAmount.Equal(decimal.NewFromInt(45)) || !cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	if item := cartItemOf(cart, music.eventID); item == nil || item.BookingID != musicItem ||
		!item.UnitPrice.Equal(decimal.NewFromInt(20)) || !item.LineTotal.Equal(decimal.NewFromInt(40)) || !item.Bookable {
		t.Fatalf("unexpected music item: %+v", item)
	}
	if item := cartItemOf(cart, food.eventID); item == nil || item.BookingID != foodItem ||
		!item.LineTotal.Equal(decimal.NewFromInt(5)) || item.AvailableSeats != 100 {
		t.Fatalf("unexpected food item: %+v", item)
	}

	// An expired reservation holds no seats and is not part of the cart.
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", musicItem).Error; err != nil {
		t.Fatal(err)
	}
	cart = getCartViaAPI(t, router)
	if len(cart.Items) != 1 || cartItemOf(cart, music.eventID) != nil || cart.TotalTickets != 1 || !cart.TotalAmount.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("unexpected cart after expiry: %+v", cart)
	}

	// A checked out item belongs to the payment, not to the cart.
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() + interval '1 minute' WHERE booking_id = ?", musicItem).Error; err != nil {
		t.Fatal(err)
	}
	checkoutViaAPI(t, router)
	cart = getCartViaAPI(t, router)
	if len(cart.Items) != 0 || !cart.TotalAmount.IsZero() || cart.CheckoutReady {
		t.Fatalf("the cart must be empty after the checkout, got %+v", cart)
	}
}

func TestUpdateCartItem_Quantity(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	other := newCartRouter(db, &middleware.Principal{Subject: otherBookingKeycloakUserID})
	item := addToCartViaAPI(t, router, event.eventID, 2)
	path := "/api/v1/cart/items/" + item.String()

	// Growing and shrinking the item.
	rec := doPaymentRequest(t, router, http.MethodPatch, path, map[string]any{"quantity": 3})
	if rec.Code != http.StatusOK {
		t.Fatalf("grow: %d %s", rec.Code, rec.Body.String())
	}
	if cart := decodeCart(t, rec); cart.TotalTickets != 3 || !cart.TotalAmount.Equal(decimal.NewFromInt(60)) {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	if rec := doPaymentRequest(t, router, http.MethodPatch, path, map[string]any{"quantity": 1}); rec.Code != http.StatusOK {
		t.Fatalf("shrink: %d %s", rec.Code, rec.Body.String())
	}

	// The item keeps its own seats, but more tickets need free ones.
	if err := db.Exec("UPDATE events SET capacity = 1 WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	wantProblem(t, doPaymentRequest(t, router, http.MethodPatch, path, map[string]any{"quantity": 2}), http.StatusConflict)

	cases := []struct {
		name   string
		router http.Handler
		path   string
		body   any
		want   int
	}{
		{"missing principal", newCartRouter(db, nil), path, map[string]any{"quantity": 2}, http.StatusUnauthorized},
		{"user not synced", newCartRouter(db, &middleware.Principal{Subject: "kc-unknown"}), path, map[string]any{"quantity": 2}, http.StatusForbidden},
		{"invalid booking id", router, "/api/v1/cart/items/not-a-uuid", map[string]any{"quantity": 2}, http.StatusBadRequest},
		{"foreign item", other, path, map[string]any{"quantity": 2}, http.StatusNotFound},
		{"unknown item", router, "/api/v1/cart/items/" + uuid.NewString(), map[string]any{"quantity": 2}, http.StatusNotFound},
		{"zero quantity", router, path, map[string]any{"quantity": 0}, http.StatusBadRequest},
		{"quantity above the limit", router, path, map[string]any{"quantity": 11}, http.StatusUnprocessableEntity},
		{"missing body", router, path, nil, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantProblem(t, doPaymentRequest(t, tc.router, http.MethodPatch, tc.path, tc.body), tc.want)
		})
	}

	// An expired reservation cannot be changed, only replaced by adding again.
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", item).Error; err != nil {
		t.Fatal(err)
	}
	wantProblem(t, doPaymentRequest(t, router, http.MethodPatch, path, map[string]any{"quantity": 2}), http.StatusConflict)
	// A checked out item is no cart item anymore.
	second := addToCartViaAPI(t, router, event.eventID, 1)
	checkoutViaAPI(t, router)
	wantProblem(t, doPaymentRequest(t, router, http.MethodPatch, "/api/v1/cart/items/"+second.String(), map[string]any{"quantity": 2}), http.StatusNotFound)
}

func TestRemoveCartItem_ReleasesSeats(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET capacity = 2 WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})
	other := newCartRouter(db, &middleware.Principal{Subject: otherBookingKeycloakUserID})
	item := addToCartViaAPI(t, router, event.eventID, 2)
	path := "/api/v1/cart/items/" + item.String()

	// While the item holds its seats nobody else can reserve them.
	wantProblem(t, doPaymentRequest(t, other, http.MethodPost, "/api/v1/cart/items", map[string]any{"eventId": event.eventID, "quantity": 1}), http.StatusConflict)
	wantProblem(t, doPaymentRequest(t, other, http.MethodDelete, path, nil), http.StatusNotFound)

	rec := doPaymentRequest(t, router, http.MethodDelete, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body.String())
	}
	if cart := decodeCart(t, rec); len(cart.Items) != 0 || cart.TotalTickets != 0 || !cart.TotalAmount.IsZero() || cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}
	wantProblem(t, doPaymentRequest(t, router, http.MethodDelete, path, nil), http.StatusNotFound)

	// The seats are free again.
	addToCartViaAPI(t, other, event.eventID, 2)
}

func TestClearCart_KeepsOpenCheckout(t *testing.T) {
	db := testdb.Open(t)
	music := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	food := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	userID := seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	musicItem := addToCartViaAPI(t, router, music.eventID, 2)
	foodItem := addToCartViaAPI(t, router, food.eventID, 1)
	// A reservation with a payment belongs to an open checkout, not to the cart. The order of
	// the items follows the booking IDs, so both are looked up, not positional.
	pending := checkoutViaAPI(t, router)
	if len(pending.Items) != 2 || paymentItemOf(pending, musicItem) == nil || paymentItemOf(pending, foodItem) == nil {
		t.Fatalf("unexpected checkout: %+v", pending.Items)
	}
	addToCartViaAPI(t, router, music.eventID, 1)

	rec := doPaymentRequest(t, router, http.MethodDelete, "/api/v1/cart", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body.String())
	}
	if cart := decodeCart(t, rec); len(cart.Items) != 0 || cart.TotalTickets != 0 || !cart.TotalAmount.IsZero() || cart.CheckoutReady {
		t.Fatalf("unexpected cart: %+v", cart)
	}

	// The cart item is gone, the checked out reservations stay with their payment.
	var open int64
	if err := db.Raw("SELECT COUNT(*) FROM bookings WHERE user_id = ? AND payment_id IS NULL AND status = 'reserved'", userID).Scan(&open).Error; err != nil || open != 0 {
		t.Fatalf("the cart items must be gone, got %d (%v)", open, err)
	}
	for _, id := range []uuid.UUID{musicItem, foodItem} {
		stored, err := repository.NewBookingRepository(db).GetBookingByID(id)
		if err != nil || stored == nil || stored.PaymentID == nil || *stored.PaymentID != pending.PaymentID {
			t.Fatalf("the checked out reservation %s must stay: %+v (%v)", id, stored, err)
		}
	}
	if rec := doPaymentRequest(t, router, http.MethodGet, paymentPath(pending.PaymentID), nil); rec.Code != http.StatusOK {
		t.Fatalf("the payment must stay readable: %d %s", rec.Code, rec.Body.String())
	}
}

// The checkout turns all items into one payment: one amount, one line per item, one expiry.
func TestCheckout_TurnsTheCartIntoOnePayment(t *testing.T) {
	db := testdb.Open(t)
	music := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	food := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET price = 5 WHERE event_id = ?", food.eventID).Error; err != nil {
		t.Fatal(err)
	}
	seedBookingUser(t, db, bookingKeycloakUserID)
	router := newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID})

	musicItem := addToCartViaAPI(t, router, music.eventID, 2)
	foodItem := addToCartViaAPI(t, router, food.eventID, 3)

	// A client supplied amount is ignored: the server computes the lines and their sum.
	rec := doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", map[string]any{"amount": "0.01"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("checkout: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	payment := decodePayment(t, rec)
	if payment.Status != model.PaymentStatusPending || payment.TicketsValid || !payment.Amount.Equal(decimal.NewFromInt(55)) {
		t.Fatalf("expected a pending payment of 55 without valid tickets, got %+v", payment)
	}
	if payment.ExpiresAt == nil || payment.Message == "" {
		t.Errorf("a pending payment must show its deadline and a message: %+v", payment)
	}
	if item := paymentItemOf(&payment, musicItem); item == nil || item.NumberOfTickets != 2 ||
		!item.LineAmount.Equal(decimal.NewFromInt(40)) || item.BookingStatus != model.BookingStatusReserved || item.TicketsValid {
		t.Fatalf("unexpected music item: %+v", item)
	}
	if item := paymentItemOf(&payment, foodItem); item == nil || item.NumberOfTickets != 3 || !item.LineAmount.Equal(decimal.NewFromInt(15)) {
		t.Fatalf("unexpected food item: %+v", item)
	}

	// A checkout that is already open is returned unchanged.
	rec = doPaymentRequest(t, router, http.MethodPost, "/api/v1/cart/checkout", nil)
	if rec.Code != http.StatusOK || decodePayment(t, rec).PaymentID != payment.PaymentID {
		t.Fatalf("checkout again: expected 200 with the same payment, got %d: %s", rec.Code, rec.Body.String())
	}

	// Completing the payment confirms every item together.
	rec = doPaymentRequest(t, router, http.MethodPost, paymentPath(payment.PaymentID)+"/simulate", map[string]any{"outcome": "success", "amount": "55"})
	if rec.Code != http.StatusOK {
		t.Fatalf("simulate: %d %s", rec.Code, rec.Body.String())
	}
	paid := decodePayment(t, rec)
	if paid.Status != model.PaymentStatusPaid || !paid.TicketsValid || paid.ExpiresAt != nil {
		t.Fatalf("expected a paid payment with valid tickets, got %+v", paid)
	}
	for _, id := range []uuid.UUID{musicItem, foodItem} {
		if item := paymentItemOf(&paid, id); item == nil || item.BookingStatus != model.BookingStatusConfirmed || !item.TicketsValid {
			t.Fatalf("expected the confirmed item %s, got %+v", id, item)
		}
	}
	for event, want := range map[uuid.UUID]int64{music.eventID: 2, food.eventID: 3} {
		sold, err := repository.NewEventRepository(db).GetConfirmedTicketCount(event)
		if err != nil {
			t.Fatal(err)
		}
		if sold != want {
			t.Errorf("event %s: expected %d sold tickets, got %d", event, want, sold)
		}
	}
}

func TestCheckout_Rejections(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	withdrawn := seedEventForOrg(t, db, "org-a", model.EventStatusPublished)
	seedBookingUser(t, db, bookingKeycloakUserID)
	seedBookingUser(t, db, otherBookingKeycloakUserID)
	seedBookingUser(t, db, expiredBookingKeycloakUserID)

	cases := []struct {
		name      string
		principal *middleware.Principal
		prepare   func(t *testing.T)
		want      int
	}{
		{"missing principal", nil, nil, http.StatusUnauthorized},
		{"user not synced", &middleware.Principal{Subject: "kc-unknown"}, nil, http.StatusForbidden},
		{"empty cart", &middleware.Principal{Subject: otherBookingKeycloakUserID}, nil, http.StatusUnprocessableEntity},
		{"only expired items", &middleware.Principal{Subject: expiredBookingKeycloakUserID}, func(t *testing.T) {
			item := addToCartViaAPI(t, newCartRouter(db, &middleware.Principal{Subject: expiredBookingKeycloakUserID}), event.eventID, 1)
			if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", item).Error; err != nil {
				t.Fatal(err)
			}
		}, http.StatusConflict},
		{"withdrawn event", &middleware.Principal{Subject: bookingKeycloakUserID}, func(t *testing.T) {
			addToCartViaAPI(t, newCartRouter(db, &middleware.Principal{Subject: bookingKeycloakUserID}), withdrawn.eventID, 1)
			if err := db.Exec("UPDATE events SET status = 'cancelled' WHERE event_id = ?", withdrawn.eventID).Error; err != nil {
				t.Fatal(err)
			}
		}, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.prepare != nil {
				tc.prepare(t)
			}
			wantProblem(t, doPaymentRequest(t, newCartRouter(db, tc.principal), http.MethodPost, "/api/v1/cart/checkout", nil), tc.want)
		})
	}

	// No rejected checkout created a payment.
	var payments int64
	if err := db.Model(&model.PaymentModel{}).Count(&payments).Error; err != nil {
		t.Fatalf("count payments: %v", err)
	}
	if payments != 0 {
		t.Errorf("rejected checkouts must not create payments, found %d", payments)
	}
}
