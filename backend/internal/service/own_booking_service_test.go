package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// EVENTHUB-236: the own booking views are built from repository.OwnBooking; these tests need no
// database.

type fakeOwnBookingRepo struct {
	bookings []*repository.OwnBooking
	err      error
}

func (r *fakeOwnBookingRepo) ListByUserNewestFirst(uuid.UUID) ([]*repository.OwnBooking, error) {
	return r.bookings, r.err
}

func (r *fakeOwnBookingRepo) ListByUserByEventStart(uuid.UUID) ([]*repository.OwnBooking, error) {
	return r.bookings, r.err
}

func (r *fakeOwnBookingRepo) GetByUser(bookingID, _ uuid.UUID) (*repository.OwnBooking, error) {
	for _, b := range r.bookings {
		if b.Booking.BookingID == bookingID {
			return b, r.err
		}
	}
	return nil, r.err
}

// ownBookingWith builds an own booking of a published event; payment and event status may be nil.
func ownBookingWith(booking model.BookingStatus, expiresAt *time.Time, payment *model.PaymentModel, event *model.EventStatus, now time.Time) *repository.OwnBooking {
	eventID := uuid.New()
	own := &repository.OwnBooking{BookingPayment: repository.BookingPayment{
		Booking:     &model.BookingModel{BookingID: uuid.New(), EventID: &eventID, NumberOfTickets: 2, Status: booking, ExpiresAt: expiresAt, CreatedAt: now.Add(-time.Hour)},
		Payment:     payment,
		EventStatus: event,
		Now:         now,
	}}
	if event != nil {
		location := "Halle, Weg 1, 53111 Bonn"
		own.Event = &model.EventModel{EventID: eventID, Title: "Konzert", StartTime: now.Add(24 * time.Hour), EndTime: now.Add(26 * time.Hour), Status: *event, Location: &location}
	} else {
		own.Booking.EventID = nil
	}
	return own
}

func paymentWith(status model.PaymentStatus, reason *model.PaymentFailureReason) *model.PaymentModel {
	return &model.PaymentModel{PaymentID: uuid.New(), Amount: decimal.NewFromInt(40), Status: status, FailureReason: reason}
}

// C5: for every booking with a payment the own booking view says what the payment result says.
func TestOwnBookingResponse_BookingsWithPayment_MatchPaymentResponse(t *testing.T) {
	declined, expired := model.PaymentFailureDeclined, model.PaymentFailureReservationExpired
	cancelledReason := model.PaymentFailureBookingCancelled
	published, completed, cancelled := model.EventStatusPublished, model.EventStatusCompleted, model.EventStatusCancelled
	now := time.Now()
	live, past := now.Add(time.Minute), now.Add(-time.Minute)

	cases := []struct {
		name string
		own  *repository.OwnBooking
	}{
		{"paid for published event", ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &published, now)},
		{"paid for completed event", ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &completed, now)},
		{"paid for withdrawn event", ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &cancelled, now)},
		{"paid for deleted event", ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), nil, now)},
		{"pending and live", ownBookingWith(model.BookingStatusReserved, &live, paymentWith(model.PaymentStatusPending, nil), &published, now)},
		{"pending and expired", ownBookingWith(model.BookingStatusReserved, &past, paymentWith(model.PaymentStatusPending, nil), &published, now)},
		{"declined", ownBookingWith(model.BookingStatusFailed, nil, paymentWith(model.PaymentStatusFailed, &declined), &published, now)},
		{"reservation expired", ownBookingWith(model.BookingStatusExpired, nil, paymentWith(model.PaymentStatusFailed, &expired), &published, now)},
		{"cancelled while pending", ownBookingWith(model.BookingStatusCancelled, nil, paymentWith(model.PaymentStatusFailed, &cancelledReason), &published, now)},
		{"cancelled after payment", ownBookingWith(model.BookingStatusCancelled, nil, paymentWith(model.PaymentStatusRefunded, nil), &published, now)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ownBookingResponse(tc.own)
			want := paymentResponse(&tc.own.BookingPayment)
			if got.TicketsValid != want.TicketsValid || got.Message != want.Message {
				t.Errorf("expected ticketsValid=%v message=%q like the payment result, got ticketsValid=%v message=%q",
					want.TicketsValid, want.Message, got.TicketsValid, got.Message)
			}
			if got.PaymentStatus == nil || *got.PaymentStatus != want.Status || got.Amount == nil || !got.Amount.Equal(want.Amount) {
				t.Errorf("expected payment %s of %s, got %v of %v", want.Status, want.Amount, got.PaymentStatus, got.Amount)
			}
			if (got.FailureReason == nil) != (want.FailureReason == nil) || (got.FailureReason != nil && *got.FailureReason != *want.FailureReason) {
				t.Errorf("expected failureReason %v, got %v", want.FailureReason, got.FailureReason)
			}
		})
	}
}

// F4: a reservation past its expiry by the database clock is shown as expired, whether or not a
// payment was started; every other status is shown as stored.
func TestOwnBookingResponse_ShownStatus_ExpiredReservationsShownExpired(t *testing.T) {
	published := model.EventStatusPublished
	now := time.Now()
	live, past := now.Add(time.Minute), now.Add(-time.Minute)

	cases := []struct {
		name          string
		own           *repository.OwnBooking
		want          model.BookingStatus
		wantExpiresAt bool
	}{
		{"live reservation without payment", ownBookingWith(model.BookingStatusReserved, &live, nil, &published, now), model.BookingStatusReserved, true},
		{"live reservation with pending payment", ownBookingWith(model.BookingStatusReserved, &live, paymentWith(model.PaymentStatusPending, nil), &published, now), model.BookingStatusReserved, true},
		{"expired reservation without payment", ownBookingWith(model.BookingStatusReserved, &past, nil, &published, now), model.BookingStatusExpired, false},
		{"expired reservation with pending payment", ownBookingWith(model.BookingStatusReserved, &past, paymentWith(model.PaymentStatusPending, nil), &published, now), model.BookingStatusExpired, false},
		{"reservation expiring right now", ownBookingWith(model.BookingStatusReserved, &now, nil, &published, now), model.BookingStatusExpired, false},
		{"stored expired", ownBookingWith(model.BookingStatusExpired, nil, nil, &published, now), model.BookingStatusExpired, false},
		{"confirmed", ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &published, now), model.BookingStatusConfirmed, false},
		{"cancelled", ownBookingWith(model.BookingStatusCancelled, nil, nil, &published, now), model.BookingStatusCancelled, false},
		{"failed", ownBookingWith(model.BookingStatusFailed, nil, nil, &published, now), model.BookingStatusFailed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ownBookingResponse(tc.own)
			if got.BookingStatus != tc.want {
				t.Errorf("expected bookingStatus %s, got %s", tc.want, got.BookingStatus)
			}
			if (got.ExpiresAt != nil) != tc.wantExpiresAt {
				t.Errorf("expected expiresAt shown=%v, got %v", tc.wantExpiresAt, got.ExpiresAt)
			}
			if got.TicketsValid != (tc.want == model.BookingStatusConfirmed) {
				t.Errorf("unexpected ticketsValid=%v for %s", got.TicketsValid, tc.want)
			}
		})
	}
}

func TestOwnBookingResponse_NoPaymentNoEvent_ReturnsNullFields(t *testing.T) {
	published := model.EventStatusPublished
	now := time.Now()
	live := now.Add(time.Minute)

	withoutPayment := ownBookingResponse(ownBookingWith(model.BookingStatusReserved, &live, nil, &published, now))
	if withoutPayment.PaymentStatus != nil || withoutPayment.Amount != nil || withoutPayment.RefundAmount != nil ||
		withoutPayment.FailureReason != nil || withoutPayment.Message != "" {
		t.Errorf("a booking without payment must show no payment data, got %+v", withoutPayment)
	}

	withoutEvent := ownBookingResponse(ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), nil, now))
	if withoutEvent.Event != nil || withoutEvent.TicketsValid {
		t.Errorf("a booking whose event is gone must show event null and no valid tickets, got %+v", withoutEvent)
	}

	own := ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &published, now)
	own.Event.Location = nil
	withoutLocation := ownBookingResponse(own)
	if withoutLocation.Event == nil || withoutLocation.Event.Location != nil || !withoutLocation.TicketsValid {
		t.Errorf("a missing location must show location null and keep the ticket valid, got %+v", withoutLocation.Event)
	}
}

func TestListOwnTickets_MixedBookings_ReturnsOnlyValidInRepositoryOrder(t *testing.T) {
	published, cancelled := model.EventStatusPublished, model.EventStatusCancelled
	now := time.Now()
	live := now.Add(time.Minute)
	first := ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &published, now)
	second := ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &published, now)
	repo := &fakeOwnBookingRepo{bookings: []*repository.OwnBooking{
		ownBookingWith(model.BookingStatusReserved, &live, paymentWith(model.PaymentStatusPending, nil), &published, now),
		first,
		ownBookingWith(model.BookingStatusConfirmed, nil, paymentWith(model.PaymentStatusPaid, nil), &cancelled, now),
		ownBookingWith(model.BookingStatusCancelled, nil, paymentWith(model.PaymentStatusRefunded, nil), &published, now),
		second,
	}}

	tickets, err := NewOwnBookingService(repo).ListOwnTickets(uuid.New())
	if err != nil {
		t.Fatalf("list tickets: %v", err)
	}
	if len(tickets) != 2 || tickets[0].BookingID != first.Booking.BookingID || tickets[1].BookingID != second.Booking.BookingID {
		t.Fatalf("expected the two valid tickets in repository order, got %+v", tickets)
	}
}

func TestListOwnBookings_StatusFilter_UsesShownStatus(t *testing.T) {
	published := model.EventStatusPublished
	now := time.Now()
	live, past := now.Add(time.Minute), now.Add(-time.Minute)
	liveReservation := ownBookingWith(model.BookingStatusReserved, &live, nil, &published, now)
	expiredReservation := ownBookingWith(model.BookingStatusReserved, &past, nil, &published, now)
	storedExpired := ownBookingWith(model.BookingStatusExpired, nil, nil, &published, now)
	repo := &fakeOwnBookingRepo{bookings: []*repository.OwnBooking{liveReservation, expiredReservation, storedExpired}}
	svc := NewOwnBookingService(repo)

	all, err := svc.ListOwnBookings(uuid.New(), "")
	if err != nil || len(all) != 3 {
		t.Fatalf("expected all 3 bookings without filter, got %d (%v)", len(all), err)
	}
	expired, _ := svc.ListOwnBookings(uuid.New(), model.BookingStatusExpired)
	if len(expired) != 2 || expired[0].BookingID != expiredReservation.Booking.BookingID || expired[1].BookingID != storedExpired.Booking.BookingID {
		t.Errorf("expected the derived and the stored expired booking, got %+v", expired)
	}
	reserved, _ := svc.ListOwnBookings(uuid.New(), model.BookingStatusReserved)
	if len(reserved) != 1 || reserved[0].BookingID != liveReservation.Booking.BookingID {
		t.Errorf("expected only the live reservation, got %+v", reserved)
	}
	none, _ := svc.ListOwnBookings(uuid.New(), model.BookingStatusConfirmed)
	if none == nil || len(none) != 0 {
		t.Errorf("expected an empty, non-nil list, got %#v", none)
	}
}

func TestGetOwnBooking_UnknownOrForeign_ReturnsErrBookingNotFound(t *testing.T) {
	svc := NewOwnBookingService(&fakeOwnBookingRepo{})
	if _, err := svc.GetOwnBooking(uuid.New(), uuid.New()); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("expected ErrBookingNotFound, got %v", err)
	}
}

func TestOwnBookingService_RepositoryError_IsReturned(t *testing.T) {
	failure := errors.New("database down")
	svc := NewOwnBookingService(&fakeOwnBookingRepo{err: failure})
	if _, err := svc.ListOwnTickets(uuid.New()); !errors.Is(err, failure) {
		t.Errorf("tickets: expected repository error, got %v", err)
	}
	if _, err := svc.ListOwnBookings(uuid.New(), ""); !errors.Is(err, failure) {
		t.Errorf("history: expected repository error, got %v", err)
	}
	if _, err := svc.GetOwnBooking(uuid.New(), uuid.New()); !errors.Is(err, failure) {
		t.Errorf("detail: expected repository error, got %v", err)
	}
}
