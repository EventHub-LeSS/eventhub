package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/testdb"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// The payment tests run against a real Postgres like the booking tests: the guarantees depend on
// transactions, row locks and the database clock.

type paymentFixture struct {
	db       *gorm.DB
	payments *PaymentService
	bookings *BookingService
	eventID  uuid.UUID
	userID   uuid.UUID
}

func newPaymentFixture(t *testing.T, capacity int, price string) paymentFixture {
	t.Helper()
	db := testdb.Open(t)
	eventID, userID := seedEvent(t, db, capacity, model.EventStatusPublished)
	setEventPrice(t, db, eventID, price)
	return paymentFixture{
		db:       db,
		payments: NewPaymentService(repository.NewTransactor(db), repository.NewBookingRepository(db), repository.NewPaymentRepository(db)),
		bookings: NewBookingService(repository.NewBookingRepository(db), 0),
		eventID:  eventID,
		userID:   userID,
	}
}

func setEventPrice(t *testing.T, db *gorm.DB, eventID uuid.UUID, price string) {
	t.Helper()
	if err := db.Exec("UPDATE events SET price = ? WHERE event_id = ?", price, eventID).Error; err != nil {
		t.Fatalf("set price: %v", err)
	}
}

func (f paymentFixture) reserve(t *testing.T, tickets int) uuid.UUID {
	t.Helper()
	booking, err := f.bookings.ReserveTickets(context.Background(), f.eventID, f.userID, tickets)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	return booking.BookingID
}

func (f paymentFixture) start(t *testing.T, bookingID uuid.UUID) *model.PaymentResponse {
	t.Helper()
	resp, _, err := f.payments.StartPayment(context.Background(), bookingID, f.userID)
	if err != nil {
		t.Fatalf("start payment: %v", err)
	}
	return resp
}

func (f paymentFixture) seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	err := f.db.Exec("INSERT INTO users (user_id, keycloak_user_id, first_name, last_name, email) VALUES (?, ?, ?, ?, ?)",
		userID, "kc-"+userID.String(), "Erika", "Muster", userID.String()+"@test.de").Error
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

type storedPayment struct {
	BookingStatus model.BookingStatus
	PaymentID     *uuid.UUID
	Amount        *decimal.Decimal
	PaymentStatus *model.PaymentStatus
	FailureReason *model.PaymentFailureReason
}

// stored reads booking and payment straight from the database, bypassing the service.
func stored(t *testing.T, db *gorm.DB, bookingID uuid.UUID) storedPayment {
	t.Helper()
	var s storedPayment
	err := db.Raw(`SELECT b.status AS booking_status, b.payment_id, p.amount, p.status AS payment_status, p.failure_reason
	               FROM bookings b LEFT JOIN payments p ON p.payment_id = b.payment_id
	               WHERE b.booking_id = ?`, bookingID).Scan(&s).Error
	if err != nil {
		t.Fatalf("load booking: %v", err)
	}
	return s
}

func mustExec(t *testing.T, db *gorm.DB, sql string, args ...any) {
	t.Helper()
	if err := db.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func paymentCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&model.PaymentModel{}).Count(&n).Error; err != nil {
		t.Fatalf("count payments: %v", err)
	}
	return n
}

func confirmedTickets(t *testing.T, db *gorm.DB, eventID uuid.UUID) int64 {
	t.Helper()
	n, err := repository.NewEventRepository(db).GetConfirmedTicketCount(eventID)
	if err != nil {
		t.Fatalf("confirmed tickets: %v", err)
	}
	return n
}

func expireReservation(t *testing.T, db *gorm.DB, bookingID uuid.UUID) {
	t.Helper()
	if err := db.Exec("UPDATE bookings SET expires_at = NOW() - interval '1 minute' WHERE booking_id = ?", bookingID).Error; err != nil {
		t.Fatalf("expire reservation: %v", err)
	}
}

func TestStartPayment_AmountIsTicketsTimesPrice(t *testing.T) {
	f := newPaymentFixture(t, 10, "19.99")
	bookingID := f.reserve(t, 3)

	resp, created, err := f.payments.StartPayment(context.Background(), bookingID, f.userID)
	if err != nil {
		t.Fatalf("start payment: %v", err)
	}
	want := decimal.RequireFromString("59.97")
	if !created {
		t.Error("expected a new payment")
	}
	if !resp.Amount.Equal(want) || resp.Status != model.PaymentStatusPending || resp.BookingStatus != model.BookingStatusReserved {
		t.Errorf("expected pending payment of %s for reserved booking, got %s %s/%s", want, resp.Amount, resp.Status, resp.BookingStatus)
	}
	if resp.NumberOfTickets != 3 || resp.BookingID != bookingID || resp.EventID == nil || *resp.EventID != f.eventID {
		t.Errorf("response does not describe the booking: %+v", resp)
	}
	if resp.TicketsValid || resp.ExpiresAt == nil || resp.Message == "" {
		t.Errorf("pending payment must show its deadline and no valid tickets: %+v", resp)
	}

	s := stored(t, f.db, bookingID)
	if s.PaymentID == nil || *s.PaymentID != resp.PaymentID {
		t.Fatalf("booking must reference the payment %s, got %v", resp.PaymentID, s.PaymentID)
	}
	if !s.Amount.Equal(want) || *s.PaymentStatus != model.PaymentStatusPending {
		t.Errorf("expected stored pending payment of %s, got %s %s", want, s.Amount, *s.PaymentStatus)
	}
}

func TestStartPayment_FreeEventCostsNothing(t *testing.T) {
	f := newPaymentFixture(t, 10, "0")
	resp := f.start(t, f.reserve(t, 4))
	if !resp.Amount.IsZero() {
		t.Errorf("expected amount 0, got %s", resp.Amount)
	}
}

func TestStartPayment_AgainReturnsThePendingPayment(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 2)
	first := f.start(t, bookingID)

	// A later price change does not alter the amount the user has already been shown.
	setEventPrice(t, f.db, f.eventID, "25.00")
	again, created, err := f.payments.StartPayment(context.Background(), bookingID, f.userID)
	if err != nil {
		t.Fatalf("start again: %v", err)
	}
	if created || again.PaymentID != first.PaymentID || !again.Amount.Equal(decimal.NewFromInt(40)) {
		t.Errorf("expected the pending payment %s of 40 again, got created=%v %s of %s", first.PaymentID, created, again.PaymentID, again.Amount)
	}
	if n := paymentCount(t, f.db); n != 1 {
		t.Errorf("expected one payment, found %d", n)
	}
}

func TestStartPayment_ConcurrentStartsCreateOnePayment(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 1)

	const attempts = 20
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := f.payments.StartPayment(context.Background(), bookingID, f.userID)
			if err != nil {
				t.Errorf("start payment: %v", err)
				return
			}
			ids <- resp.PaymentID
		}()
	}
	wg.Wait()
	close(ids)

	distinct := map[uuid.UUID]bool{}
	for id := range ids {
		distinct[id] = true
	}
	if len(distinct) != 1 {
		t.Errorf("expected all starts to return one payment, got %d", len(distinct))
	}
	if n := paymentCount(t, f.db); n != 1 {
		t.Errorf("expected one payment, found %d", n)
	}
}

func TestStartPayment_Rejections(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown booking", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		if _, _, err := f.payments.StartPayment(ctx, uuid.New(), f.userID); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got %v", err)
		}
	})
	t.Run("booking of another user", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.seedUser(t)); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got %v", err)
		}
		if s := stored(t, f.db, bookingID); s.PaymentID != nil {
			t.Error("no payment must be started for another user")
		}
	})
	t.Run("expired reservation", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		expireReservation(t, f.db, bookingID)
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrReservationExpired) {
			t.Errorf("expected ErrReservationExpired, got %v", err)
		}
	})
	t.Run("withdrawn event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrEventUnavailable) {
			t.Errorf("expected ErrEventUnavailable, got %v", err)
		}
	})
	t.Run("booking not reserved", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		mustExec(t, f.db, "UPDATE bookings SET status = 'cancelled' WHERE booking_id = ?", bookingID)
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrBookingNotPayable) {
			t.Errorf("expected ErrBookingNotPayable, got %v", err)
		}
	})
	t.Run("amount does not fit the payment column", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "9999999999.99")
		bookingID := f.reserve(t, 2)
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrInvalidPaymentAmount) {
			t.Errorf("expected ErrInvalidPaymentAmount, got %v", err)
		}
		if n := paymentCount(t, f.db); n != 0 {
			t.Errorf("no payment must be stored, found %d", n)
		}
	})
	t.Run("payment already completed", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		mustExec(t, f.db, "UPDATE payments SET status = 'failed'")
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrPaymentAlreadyProcessed) {
			t.Errorf("expected ErrPaymentAlreadyProcessed, got %v", err)
		}
	})
}

func TestSimulatePayment_SuccessConfirmsBooking(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 2)
	f.start(t, bookingID)

	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString("40.00"))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusPaid || resp.BookingStatus != model.BookingStatusConfirmed || !resp.TicketsValid {
		t.Errorf("expected paid payment and confirmed booking with valid tickets, got %+v", resp)
	}
	if resp.FailureReason != nil || resp.ExpiresAt != nil || resp.Message == "" {
		t.Errorf("completed payment must carry a message but no failure or deadline: %+v", resp)
	}

	s := stored(t, f.db, bookingID)
	if *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("expected stored paid/confirmed, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 2 {
		t.Errorf("expected 2 sold tickets, got %d", n)
	}
}

func TestSimulatePayment_FailureLeavesNoTicketClaim(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	bookingID := f.reserve(t, 2)
	f.start(t, bookingID)

	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentFailure, decimal.NewFromInt(40))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.BookingStatus != model.BookingStatusFailed || resp.TicketsValid {
		t.Errorf("expected failed payment and booking without valid tickets, got %+v", resp)
	}
	if resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureDeclined || resp.Message == "" {
		t.Errorf("expected declined failure with a message, got %+v", resp)
	}

	s := stored(t, f.db, bookingID)
	if *s.PaymentStatus != model.PaymentStatusFailed || *s.FailureReason != model.PaymentFailureDeclined || s.BookingStatus != model.BookingStatusFailed {
		t.Errorf("expected stored failed/declined/failed, got %+v", s)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 0 {
		t.Errorf("a failed payment must not sell tickets, got %d", n)
	}
	// The tickets of the failed booking are free again.
	if _, err := f.bookings.ReserveTickets(context.Background(), f.eventID, f.userID, 2); err != nil {
		t.Errorf("expected the released tickets to be bookable, got %v", err)
	}
}

func TestSimulatePayment_RejectsManipulatedAmount(t *testing.T) {
	f := newPaymentFixture(t, 10, "19.99")
	bookingID := f.reserve(t, 3)
	f.start(t, bookingID)

	// 1e2000000000 also checks that the comparison does not stall on a huge exponent.
	for _, amount := range []string{"0", "0.01", "59.96", "59.98", "59.971", "-59.97", "5997", "100000000000", "1e2000000000", "5997e-2000000000"} {
		_, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString(amount))
		if !errors.Is(err, ErrPaymentAmountMismatch) {
			t.Errorf("amount %s: expected ErrPaymentAmountMismatch, got %v", amount, err)
		}
	}
	s := stored(t, f.db, bookingID)
	if *s.PaymentStatus != model.PaymentStatusPending || s.BookingStatus != model.BookingStatusReserved {
		t.Fatalf("rejected amounts must not change anything, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}

	// The same value with another scale is the stored amount.
	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString("59.970"))
	if err != nil || resp.Status != model.PaymentStatusPaid {
		t.Errorf("expected the stored amount to be accepted, got %v %+v", err, resp)
	}
}

func TestSimulatePayment_ExpiredReservationFails(t *testing.T) {
	f := newPaymentFixture(t, 1, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)
	expireReservation(t, f.db, bookingID)

	// The expired reservation released its ticket, which someone else has reserved meanwhile.
	other := f.seedUser(t)
	if _, err := f.bookings.ReserveTickets(context.Background(), f.eventID, other, 1); err != nil {
		t.Fatalf("reserve released ticket: %v", err)
	}

	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureReservationExpired {
		t.Errorf("expected failed payment with reservation_expired, got %+v", resp)
	}
	if resp.BookingStatus != model.BookingStatusExpired || resp.TicketsValid {
		t.Errorf("expected expired booking without valid tickets, got %+v", resp)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 0 {
		t.Errorf("the expired reservation must not be confirmed, got %d sold tickets", n)
	}
}

func TestSimulatePayment_WithdrawnEventFails(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)
	mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureEventUnavailable {
		t.Errorf("expected failed payment with event_unavailable, got %+v", resp)
	}
	if resp.BookingStatus != model.BookingStatusFailed || resp.TicketsValid {
		t.Errorf("expected failed booking without valid tickets, got %+v", resp)
	}
}

func TestSimulatePayment_CompletesOnlyOnce(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)
	ctx := context.Background()

	if _, err := f.payments.SimulatePayment(ctx, bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
		t.Fatalf("simulate: %v", err)
	}
	for _, outcome := range []model.SimulatedPaymentOutcome{model.SimulatedPaymentFailure, model.SimulatedPaymentSuccess} {
		if _, err := f.payments.SimulatePayment(ctx, bookingID, f.userID, outcome, decimal.NewFromInt(20)); !errors.Is(err, ErrPaymentAlreadyProcessed) {
			t.Errorf("%s after success: expected ErrPaymentAlreadyProcessed, got %v", outcome, err)
		}
	}
	if s := stored(t, f.db, bookingID); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("a completed payment must stay paid, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
}

func TestSimulatePayment_ConcurrentCompletionsSettleOnce(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)

	const attempts = 20
	var wg sync.WaitGroup
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		outcome := model.SimulatedPaymentSuccess
		if i%2 == 1 {
			outcome = model.SimulatedPaymentFailure
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, outcome, decimal.NewFromInt(20))
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	completed := 0
	for err := range results {
		switch {
		case err == nil:
			completed++
		case !errors.Is(err, ErrPaymentAlreadyProcessed):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if completed != 1 {
		t.Errorf("expected exactly one completion, got %d", completed)
	}
	s := stored(t, f.db, bookingID)
	paid := *s.PaymentStatus == model.PaymentStatusPaid && s.BookingStatus == model.BookingStatusConfirmed
	failed := *s.PaymentStatus == model.PaymentStatusFailed && s.BookingStatus == model.BookingStatusFailed
	if !paid && !failed {
		t.Errorf("payment and booking disagree: %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
}

// Starting, completing and reserving lock the same rows; mixing them must neither deadlock nor
// overbook.
func TestPayment_ConcurrentStartSimulateAndReserve(t *testing.T) {
	f := newPaymentFixture(t, 3, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)

	const rounds = 10
	var wg sync.WaitGroup
	completed := make(chan struct{}, rounds)
	for i := 0; i < rounds; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _, err := f.payments.StartPayment(context.Background(), bookingID, f.userID)
			if err != nil && !errors.Is(err, ErrBookingNotPayable) {
				t.Errorf("start: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
			switch {
			case err == nil:
				completed <- struct{}{}
			case !errors.Is(err, ErrPaymentAlreadyProcessed):
				t.Errorf("simulate: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			var capErr *CapacityExceededError
			if _, err := f.bookings.ReserveTickets(context.Background(), f.eventID, f.userID, 1); err != nil && !errors.As(err, &capErr) {
				t.Errorf("reserve: %v", err)
			}
		}()
	}
	wg.Wait()
	close(completed)

	if len(completed) != 1 {
		t.Errorf("expected exactly one completion, got %d", len(completed))
	}
	if s := stored(t, f.db, bookingID); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("expected paid/confirmed, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
	var occupied int64
	if err := f.db.Raw("SELECT COALESCE(SUM(number_of_tickets), 0) FROM bookings WHERE event_id = ? AND status IN ('reserved', 'confirmed')", f.eventID).Scan(&occupied).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if occupied != 3 {
		t.Errorf("expected the capacity of 3 to be used up exactly, got %d tickets", occupied)
	}
}

func TestSimulatePayment_Rejections(t *testing.T) {
	ctx := context.Background()
	amount := decimal.NewFromInt(20)

	t.Run("payment not started", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		if _, err := f.payments.SimulatePayment(ctx, bookingID, f.userID, model.SimulatedPaymentSuccess, amount); !errors.Is(err, ErrPaymentNotStarted) {
			t.Errorf("expected ErrPaymentNotStarted, got %v", err)
		}
		if s := stored(t, f.db, bookingID); s.BookingStatus != model.BookingStatusReserved {
			t.Errorf("booking must stay reserved, got %s", s.BookingStatus)
		}
	})
	t.Run("booking of another user", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		if _, err := f.payments.SimulatePayment(ctx, bookingID, f.seedUser(t), model.SimulatedPaymentSuccess, amount); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got %v", err)
		}
		if s := stored(t, f.db, bookingID); *s.PaymentStatus != model.PaymentStatusPending {
			t.Errorf("payment must stay pending, got %s", *s.PaymentStatus)
		}
	})
	t.Run("unknown booking", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		if _, err := f.payments.SimulatePayment(ctx, uuid.New(), f.userID, model.SimulatedPaymentSuccess, amount); !errors.Is(err, ErrBookingNotFound) {
			t.Errorf("expected ErrBookingNotFound, got %v", err)
		}
	})
	t.Run("unknown outcome", func(t *testing.T) {
		svc := NewPaymentService(nil, nil, nil)
		if _, err := svc.SimulatePayment(ctx, uuid.New(), uuid.New(), "maybe", amount); !errors.Is(err, ErrInvalidPaymentOutcome) {
			t.Errorf("expected ErrInvalidPaymentOutcome, got %v", err)
		}
	})
}

func TestGetPayment_ReturnsStoredState(t *testing.T) {
	f := newPaymentFixture(t, 10, "20")
	bookingID := f.reserve(t, 1)
	ctx := context.Background()

	if _, err := f.payments.GetPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrPaymentNotStarted) {
		t.Errorf("expected ErrPaymentNotStarted before the start, got %v", err)
	}
	started := f.start(t, bookingID)

	// The result reflects the stored status, also when it was changed outside the service.
	mustExec(t, f.db, "UPDATE payments SET status = 'refunded' WHERE payment_id = ?", started.PaymentID)
	mustExec(t, f.db, "UPDATE bookings SET status = 'cancelled' WHERE booking_id = ?", bookingID)
	resp, err := f.payments.GetPayment(ctx, bookingID, f.userID)
	if err != nil {
		t.Fatalf("get payment: %v", err)
	}
	if resp.Status != model.PaymentStatusRefunded || resp.BookingStatus != model.BookingStatusCancelled || resp.TicketsValid {
		t.Errorf("expected stored refunded/cancelled without valid tickets, got %+v", resp)
	}
	if _, err := f.payments.GetPayment(ctx, bookingID, f.seedUser(t)); !errors.Is(err, ErrBookingNotFound) {
		t.Errorf("expected ErrBookingNotFound for another user, got %v", err)
	}
}

func TestPaymentAmount(t *testing.T) {
	cases := []struct {
		price   string
		tickets int
		want    string
		err     error
	}{
		{"20", 2, "40", nil},
		{"19.99", 3, "59.97", nil},
		{"0", 5, "0", nil},
		{"0.01", 1, "0.01", nil},
		{"4999999999.99", 2, "9999999999.98", nil},
		{"5000000000", 2, "", ErrInvalidPaymentAmount},
		{"-1", 1, "", ErrInvalidPaymentAmount},
		{"0.005", 1, "", ErrInvalidPaymentAmount},
		{"20", 0, "", ErrInvalidTicketCount},
		{"20", -1, "", ErrInvalidTicketCount},
	}
	for _, tc := range cases {
		got, err := paymentAmount(decimal.RequireFromString(tc.price), tc.tickets)
		if !errors.Is(err, tc.err) {
			t.Errorf("%s x %d: expected error %v, got %v", tc.price, tc.tickets, tc.err, err)
			continue
		}
		if tc.err == nil && !got.Equal(decimal.RequireFromString(tc.want)) {
			t.Errorf("%s x %d: expected %s, got %s", tc.price, tc.tickets, tc.want, got)
		}
	}
}

func TestPaymentResponse_OnlyPaidConfirmedTicketsOfHeldEventsAreValid(t *testing.T) {
	declined := model.PaymentFailureDeclined
	expired := model.PaymentFailureReservationExpired
	unavailable := model.PaymentFailureEventUnavailable
	published, completed, cancelled := model.EventStatusPublished, model.EventStatusCompleted, model.EventStatusCancelled
	now := time.Now()
	live, past := now.Add(time.Minute), now.Add(-time.Minute)
	view := func(payment model.PaymentStatus, reason *model.PaymentFailureReason, booking model.BookingStatus, expires *time.Time, event *model.EventStatus) *repository.BookingPayment {
		return &repository.BookingPayment{
			Booking:     &model.BookingModel{BookingID: uuid.New(), Status: booking, ExpiresAt: expires},
			Payment:     &model.PaymentModel{PaymentID: uuid.New(), Status: payment, FailureReason: reason},
			EventStatus: event,
			Now:         now,
		}
	}
	cases := []struct {
		name        string
		view        *repository.BookingPayment
		valid       bool
		completable bool
	}{
		{"paid for published event", view(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &published), true, false},
		{"paid for completed event", view(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &completed), true, false},
		{"paid for cancelled event", view(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &cancelled), false, false},
		{"paid for deleted event", view(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, nil), false, false},
		{"paid for cancelled booking", view(model.PaymentStatusPaid, nil, model.BookingStatusCancelled, nil, &published), false, false},
		{"pending and live", view(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, &published), false, true},
		{"pending and expired", view(model.PaymentStatusPending, nil, model.BookingStatusReserved, &past, &published), false, false},
		{"pending for withdrawn event", view(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, &cancelled), false, false},
		{"pending for deleted event", view(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, nil), false, false},
		{"pending but booking not reserved", view(model.PaymentStatusPending, nil, model.BookingStatusConfirmed, &live, &published), false, false},
		{"declined", view(model.PaymentStatusFailed, &declined, model.BookingStatusFailed, nil, &published), false, false},
		{"declined but confirmed", view(model.PaymentStatusFailed, &declined, model.BookingStatusConfirmed, nil, &published), false, false},
		{"reservation expired", view(model.PaymentStatusFailed, &expired, model.BookingStatusExpired, nil, &published), false, false},
		{"event unavailable", view(model.PaymentStatusFailed, &unavailable, model.BookingStatusFailed, nil, &cancelled), false, false},
		{"failed without reason", view(model.PaymentStatusFailed, nil, model.BookingStatusFailed, nil, &published), false, false},
		{"refunded", view(model.PaymentStatusRefunded, nil, model.BookingStatusConfirmed, nil, &published), false, false},
	}
	success := paymentResponse(cases[0].view).Message
	completable := paymentResponse(cases[5].view).Message

	messages := map[string]bool{}
	for _, tc := range cases {
		resp := paymentResponse(tc.view)
		if resp.TicketsValid != tc.valid {
			t.Errorf("%s: expected ticketsValid=%v", tc.name, tc.valid)
		}
		if resp.Message == "" {
			t.Errorf("%s: expected a message", tc.name)
		}
		// The message never contradicts the result.
		if !tc.valid && resp.Message == success {
			t.Errorf("%s: invalid tickets must not be reported as a confirmed booking", tc.name)
		}
		if (resp.Message == completable) != tc.completable {
			t.Errorf("%s: only a payable booking may be asked to complete its payment, got %q", tc.name, resp.Message)
		}
		messages[resp.Message] = true
	}
	// pending (payable, reservation invalid, event gone), paid (valid, event gone, booking
	// inactive), refunded, generic failure and one message per failure reason
	if len(messages) != 11 {
		t.Errorf("expected 11 distinct messages, got %d: %v", len(messages), messages)
	}
}

func TestStartPayment_FailsPendingPaymentItCannotComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("expired reservation", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		expireReservation(t, f.db, bookingID)

		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrReservationExpired) {
			t.Fatalf("expected ErrReservationExpired, got %v", err)
		}
		s := stored(t, f.db, bookingID)
		if *s.PaymentStatus != model.PaymentStatusFailed || s.FailureReason == nil || *s.FailureReason != model.PaymentFailureReservationExpired || s.BookingStatus != model.BookingStatusExpired {
			t.Errorf("expected failed/reservation_expired/expired to be stored, got %+v", s)
		}
		// Asking again still says why.
		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrReservationExpired) {
			t.Errorf("start again: expected ErrReservationExpired, got %v", err)
		}
	})
	t.Run("withdrawn event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

		if _, _, err := f.payments.StartPayment(ctx, bookingID, f.userID); !errors.Is(err, ErrEventUnavailable) {
			t.Fatalf("expected ErrEventUnavailable, got %v", err)
		}
		s := stored(t, f.db, bookingID)
		if *s.PaymentStatus != model.PaymentStatusFailed || s.FailureReason == nil || *s.FailureReason != model.PaymentFailureEventUnavailable || s.BookingStatus != model.BookingStatusFailed {
			t.Errorf("expected failed/event_unavailable/failed to be stored, got %+v", s)
		}
	})
}

func TestExpireStalePayments(t *testing.T) {
	f := newPaymentFixture(t, 20, "20")
	ctx := context.Background()

	stale := f.reserve(t, 1)
	f.start(t, stale)
	expireReservation(t, f.db, stale)

	live := f.reserve(t, 1)
	f.start(t, live)

	withoutPayment := f.reserve(t, 1)
	expireReservation(t, f.db, withoutPayment)

	paid := f.reserve(t, 1)
	f.start(t, paid)
	if _, err := f.payments.SimulatePayment(ctx, paid, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
		t.Fatalf("simulate: %v", err)
	}

	// A stale payment of a withdrawn event fails for that reason.
	otherEvent, otherUser := seedEvent(t, f.db, 5, model.EventStatusPublished)
	other, err := f.bookings.ReserveTickets(ctx, otherEvent, otherUser, 1)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if _, _, err := f.payments.StartPayment(ctx, other.BookingID, otherUser); err != nil {
		t.Fatalf("start: %v", err)
	}
	expireReservation(t, f.db, other.BookingID)
	mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", otherEvent)

	n, err := f.payments.ExpireStalePayments(ctx)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 failed payments, got %d", n)
	}
	for id, want := range map[uuid.UUID]struct {
		reason  model.PaymentFailureReason
		booking model.BookingStatus
	}{
		stale:           {model.PaymentFailureReservationExpired, model.BookingStatusExpired},
		other.BookingID: {model.PaymentFailureEventUnavailable, model.BookingStatusFailed},
	} {
		s := stored(t, f.db, id)
		if *s.PaymentStatus != model.PaymentStatusFailed || s.FailureReason == nil || *s.FailureReason != want.reason || s.BookingStatus != want.booking {
			t.Errorf("booking %s: expected failed/%s/%s, got %+v", id, want.reason, want.booking, s)
		}
	}
	if s := stored(t, f.db, live); *s.PaymentStatus != model.PaymentStatusPending || s.BookingStatus != model.BookingStatusReserved {
		t.Errorf("a live payment must stay pending, got %+v", s)
	}
	if s := stored(t, f.db, withoutPayment); s.PaymentID != nil || s.BookingStatus != model.BookingStatusReserved {
		t.Errorf("a booking without payment must stay untouched, got %+v", s)
	}
	if s := stored(t, f.db, paid); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("a paid booking must stay confirmed, got %+v", s)
	}

	if n, err := f.payments.ExpireStalePayments(ctx); err != nil || n != 0 {
		t.Errorf("a second run must find nothing, got %d %v", n, err)
	}
}

func TestExpireStalePayments_WorksThroughSeveralBatches(t *testing.T) {
	previous := stalePaymentBatch
	stalePaymentBatch = 2
	t.Cleanup(func() { stalePaymentBatch = previous })

	f := newPaymentFixture(t, 10, "20")
	for i := 0; i < 5; i++ {
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		expireReservation(t, f.db, bookingID)
	}
	if n, err := f.payments.ExpireStalePayments(context.Background()); err != nil || n != 5 {
		t.Errorf("expected all 5 stale payments in one call, got %d %v", n, err)
	}
}

// The cleanup must neither wait for nor interfere with a booking that is being completed.
func TestExpireStalePayments_SkipsLockedBookings(t *testing.T) {
	f := newPaymentFixture(t, 10, "20")
	ctx := context.Background()
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)
	expireReservation(t, f.db, bookingID)

	holder := f.db.Begin()
	if err := holder.Exec("SELECT 1 FROM bookings WHERE booking_id = ? FOR UPDATE", bookingID).Error; err != nil {
		t.Fatalf("lock booking: %v", err)
	}
	done := make(chan int, 1)
	go func() {
		n, err := f.payments.ExpireStalePayments(ctx)
		if err != nil {
			t.Errorf("expire: %v", err)
		}
		done <- n
	}()
	select {
	case n := <-done:
		if n != 0 {
			t.Errorf("expected the locked booking to be skipped, got %d", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup waited for a locked booking")
	}
	holder.Rollback()

	if n, err := f.payments.ExpireStalePayments(ctx); err != nil || n != 1 {
		t.Errorf("expected the released booking to be expired, got %d %v", n, err)
	}
}

// Cleanup and completion race for the same expired reservations: neither may deadlock, and an
// expired reservation must never end up paid.
func TestExpireStalePayments_ConcurrentWithCompletion(t *testing.T) {
	f := newPaymentFixture(t, 50, "20")
	ctx := context.Background()
	const bookings = 10
	ids := make([]uuid.UUID, bookings)
	for i := range ids {
		ids[i] = f.reserve(t, 1)
		f.start(t, ids[i])
		expireReservation(t, f.db, ids[i])
	}

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.payments.ExpireStalePayments(ctx); err != nil {
				t.Errorf("expire: %v", err)
			}
		}()
	}
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.payments.SimulatePayment(ctx, id, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
			if err != nil && !errors.Is(err, ErrPaymentAlreadyProcessed) {
				t.Errorf("simulate: %v", err)
			}
		}()
	}
	wg.Wait()

	for _, id := range ids {
		s := stored(t, f.db, id)
		if *s.PaymentStatus != model.PaymentStatusFailed || s.BookingStatus != model.BookingStatusExpired {
			t.Errorf("booking %s: expected failed/expired, got %s/%s", id, *s.PaymentStatus, s.BookingStatus)
		}
	}
}

func TestGetPayment_ReflectsEventState(t *testing.T) {
	ctx := context.Background()
	paidBooking := func(t *testing.T, f paymentFixture) uuid.UUID {
		t.Helper()
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		if _, err := f.payments.SimulatePayment(ctx, bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
			t.Fatalf("simulate: %v", err)
		}
		return bookingID
	}

	t.Run("pending payment of a withdrawn event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		f.start(t, bookingID)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, bookingID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if resp.Status != model.PaymentStatusPending || !strings.Contains(resp.Message, "event is no longer available") {
			t.Errorf("expected the stored pending status with a message about the event, got %+v", resp)
		}
	})
	t.Run("paid tickets of a cancelled event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := paidBooking(t, f)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, bookingID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if resp.Status != model.PaymentStatusPaid || resp.BookingStatus != model.BookingStatusConfirmed || resp.TicketsValid {
			t.Errorf("tickets of a cancelled event must not be valid, got %+v", resp)
		}
	})
	t.Run("paid tickets of a deleted event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := paidBooking(t, f)
		mustExec(t, f.db, "DELETE FROM events WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, bookingID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if resp.EventID != nil || resp.TicketsValid {
			t.Errorf("tickets of a deleted event must not be valid, got %+v", resp)
		}
	})
}

// GET must never combine the booking state from before a completion with the payment after it.
func TestGetPayment_ConsistentWhileCompleting(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	bookingID := f.reserve(t, 1)
	f.start(t, bookingID)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, err := f.payments.GetPayment(context.Background(), bookingID, f.userID)
				if err != nil {
					t.Errorf("get payment: %v", err)
					return
				}
				pending := resp.Status == model.PaymentStatusPending && resp.BookingStatus == model.BookingStatusReserved
				paid := resp.Status == model.PaymentStatusPaid && resp.BookingStatus == model.BookingStatusConfirmed && resp.TicketsValid
				if !pending && !paid {
					t.Errorf("inconsistent result: %s/%s ticketsValid=%v", resp.Status, resp.BookingStatus, resp.TicketsValid)
					return
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
		t.Errorf("simulate: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}
