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

// The payment tests run against a real Postgres like the cart tests: the guarantees depend on
// transactions, row locks and the database clock.

type paymentFixture struct {
	db       *gorm.DB
	carts    *CartService
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
	tx := repository.NewTransactor(db)
	return paymentFixture{
		db:       db,
		carts:    NewCartService(tx, repository.NewBookingRepository(db), repository.NewCartRepository(db), repository.NewPaymentRepository(db), 0),
		payments: NewPaymentService(tx, repository.NewBookingRepository(db), repository.NewPaymentRepository(db)),
		bookings: NewBookingService(tx, repository.NewBookingRepository(db), repository.NewPaymentRepository(db)),
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

// reserve puts tickets of the fixture event in the cart of the fixture user and returns the
// booking ID of the item.
func (f paymentFixture) reserve(t *testing.T, tickets int) uuid.UUID {
	t.Helper()
	return f.addToCart(t, f.userID, tickets)
}

// checkout turns the live cart of the user into a pending payment.
func (f paymentFixture) checkoutFor(t *testing.T, userID uuid.UUID) *model.PaymentResponse {
	t.Helper()
	resp, _, err := f.carts.Checkout(context.Background(), userID)
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	return resp
}

func (f paymentFixture) checkout(t *testing.T) *model.PaymentResponse {
	t.Helper()
	return f.checkoutFor(t, f.userID)
}

// pay checks the cart of the user out and completes the payment with the outcome.
func (f paymentFixture) pay(t *testing.T, outcome model.SimulatedPaymentOutcome) *model.PaymentResponse {
	t.Helper()
	pending := f.checkout(t)
	resp, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, outcome, pending.Amount)
	if err != nil {
		t.Fatalf("simulate %s: %v", outcome, err)
	}
	return resp
}

func (f paymentFixture) seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return seedUserRow(t, f.db)
}

type storedPayment struct {
	BookingStatus model.BookingStatus
	PaymentID     *uuid.UUID
	LineAmount    *decimal.Decimal
	Amount        *decimal.Decimal
	PaymentStatus *model.PaymentStatus
	FailureReason *model.PaymentFailureReason
}

// stored reads booking and payment straight from the database, bypassing the service.
func stored(t *testing.T, db *gorm.DB, bookingID uuid.UUID) storedPayment {
	t.Helper()
	var s storedPayment
	err := db.Raw(`SELECT b.status AS booking_status, b.payment_id, b.line_amount, p.amount, p.status AS payment_status, p.failure_reason
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

// paymentItemFor finds the item of the booking in the payment response, or nil.
func paymentItemFor(resp *model.PaymentResponse, bookingID uuid.UUID) *model.PaymentItemResponse {
	for i := range resp.Items {
		if resp.Items[i].BookingID == bookingID {
			return &resp.Items[i]
		}
	}
	return nil
}

func TestCheckout_SumsTheLinesOfAllItems(t *testing.T) {
	f := newPaymentFixture(t, 10, "19.99")
	music := f.eventID
	food := seedEventFor(t, f.db, 10, model.EventStatusPublished, "5", f.userID)
	ctx := context.Background()

	musicItem := f.reserve(t, 3)
	foodItem := addToCartEvent(t, f.carts, f.userID, food, 2)
	if musicItem == foodItem {
		t.Fatal("items of different events must be different reservations")
	}

	resp := f.checkout(t)
	want := decimal.RequireFromString("69.97")
	if resp.Status != model.PaymentStatusPending || resp.TicketsValid || !resp.Amount.Equal(want) {
		t.Fatalf("expected a pending payment of %s without valid tickets, got %+v", want, resp)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected both items in the payment, got %+v", resp.Items)
	}
	if item := paymentItemFor(resp, musicItem); item == nil || item.NumberOfTickets != 3 || !item.LineAmount.Equal(decimal.RequireFromString("59.97")) ||
		item.BookingStatus != model.BookingStatusReserved || item.Event == nil || item.Event.EventID != music {
		t.Fatalf("unexpected music item: %+v", item)
	} else {
		wantSeededLocation(t, item.Event.Location)
	}
	if item := paymentItemFor(resp, foodItem); item == nil || !item.LineAmount.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("unexpected food item: %+v", item)
	}
	if resp.ExpiresAt == nil || resp.Message == "" {
		t.Errorf("a pending payment must show its deadline and a message: %+v", resp)
	}

	// The lines and the sum are stored, and all bookings share the expiry of the payment.
	for _, id := range []uuid.UUID{musicItem, foodItem} {
		s := stored(t, f.db, id)
		if s.PaymentID == nil || *s.PaymentID != resp.PaymentID {
			t.Fatalf("booking %s must reference the payment %s, got %v", id, resp.PaymentID, s.PaymentID)
		}
		if s.LineAmount == nil || !s.LineAmount.Equal(paymentItemFor(resp, id).LineAmount) {
			t.Fatalf("booking %s must store its line amount, got %v", id, s.LineAmount)
		}
	}
	var expiries int64
	if err := f.db.Raw(`SELECT COUNT(DISTINCT expires_at) FROM bookings WHERE payment_id = ?`, resp.PaymentID).Scan(&expiries).Error; err != nil || expiries != 1 {
		t.Fatalf("all bookings must share one expiry, got %d (%v)", expiries, err)
	}

	// The cart is empty after the checkout: its items belong to the payment now.
	cart, err := f.carts.GetCart(ctx, f.userID)
	if err != nil || len(cart.Items) != 0 {
		t.Fatalf("the cart must be empty after the checkout, got %+v (%v)", cart, err)
	}
}

func TestCheckout_FreeEventCostsNothing(t *testing.T) {
	f := newPaymentFixture(t, 10, "0")
	f.reserve(t, 4)
	resp := f.checkout(t)
	if !resp.Amount.IsZero() {
		t.Errorf("expected amount 0, got %s", resp.Amount)
	}
}

func TestCheckout_AgainReturnsThePendingPayment(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	f.reserve(t, 2)
	first := f.checkout(t)

	// A later price change does not alter the amount the user has already been shown.
	setEventPrice(t, f.db, f.eventID, "25.00")
	again, created, err := f.carts.Checkout(context.Background(), f.userID)
	if err != nil {
		t.Fatalf("checkout again: %v", err)
	}
	if created || again.PaymentID != first.PaymentID || !again.Amount.Equal(decimal.NewFromInt(40)) {
		t.Errorf("expected the pending payment %s of 40 again, got created=%v %s of %s", first.PaymentID, created, again.PaymentID, again.Amount)
	}
	if n := paymentCount(t, f.db); n != 1 {
		t.Errorf("expected one payment, found %d", n)
	}
}

func TestCheckout_ConcurrentCheckoutsCreateOnePayment(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	f.reserve(t, 1)

	const attempts = 20
	var wg sync.WaitGroup
	ids := make(chan uuid.UUID, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := f.carts.Checkout(context.Background(), f.userID)
			if err != nil {
				t.Errorf("checkout: %v", err)
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
		t.Errorf("expected all checkouts to return one payment, got %d", len(distinct))
	}
	if n := paymentCount(t, f.db); n != 1 {
		t.Errorf("expected one payment, found %d", n)
	}
}

func TestCheckout_Rejections(t *testing.T) {
	ctx := context.Background()

	t.Run("empty cart", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		if _, _, err := f.carts.Checkout(ctx, f.userID); !errors.Is(err, ErrCartEmpty) {
			t.Errorf("expected ErrCartEmpty, got %v", err)
		}
	})
	t.Run("only expired items", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		expireReservation(t, f.db, bookingID)
		if _, _, err := f.carts.Checkout(ctx, f.userID); !errors.Is(err, ErrReservationExpired) {
			t.Errorf("expected ErrReservationExpired, got %v", err)
		}
		if n := paymentCount(t, f.db); n != 0 {
			t.Errorf("expired items must not create a payment, found %d", n)
		}
	})
	t.Run("expired items are skipped beside live ones", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		expired := f.reserve(t, 1)
		expireReservation(t, f.db, expired)
		// Reserving the same event again would replace the expired reservation in its row, so
		// the live item comes from a second event.
		liveEvent := seedEventFor(t, f.db, 10, model.EventStatusPublished, "20", f.userID)
		live := addToCartEvent(t, f.carts, f.userID, liveEvent, 2)

		resp := f.checkout(t)
		if len(resp.Items) != 1 || resp.Items[0].BookingID != live || !resp.Amount.Equal(decimal.NewFromInt(40)) {
			t.Fatalf("expected only the live item of 40, got %+v", resp)
		}
		if s := stored(t, f.db, expired); s.PaymentID != nil {
			t.Errorf("the expired item must keep no payment, got %v", s.PaymentID)
		}
	})
	t.Run("withdrawn event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		f.reserve(t, 1)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)
		if _, _, err := f.carts.Checkout(ctx, f.userID); !errors.Is(err, ErrEventUnavailable) {
			t.Errorf("expected ErrEventUnavailable, got %v", err)
		}
		if n := paymentCount(t, f.db); n != 0 {
			t.Errorf("no payment must be stored, found %d", n)
		}
	})
	t.Run("amount does not fit the payment column", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "9999999999.99")
		f.reserve(t, 2)
		if _, _, err := f.carts.Checkout(ctx, f.userID); !errors.Is(err, ErrInvalidPaymentAmount) {
			t.Errorf("expected ErrInvalidPaymentAmount, got %v", err)
		}
		if n := paymentCount(t, f.db); n != 0 {
			t.Errorf("no payment must be stored, found %d", n)
		}
	})
	t.Run("cancelled item is not a cart item", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		bookingID := f.reserve(t, 1)
		mustExec(t, f.db, "UPDATE bookings SET status = 'cancelled' WHERE booking_id = ?", bookingID)
		if _, _, err := f.carts.Checkout(ctx, f.userID); !errors.Is(err, ErrCartEmpty) {
			t.Errorf("expected ErrCartEmpty, got %v", err)
		}
	})
}

func TestSimulatePayment_SuccessConfirmsEveryItem(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	other := seedEventFor(t, f.db, 10, model.EventStatusPublished, "5", f.userID)
	first := f.reserve(t, 2)
	second := addToCartEvent(t, f.carts, f.userID, other, 1)

	resp, err := f.payments.SimulatePayment(context.Background(), f.checkout(t).PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString("45.00"))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusPaid || !resp.TicketsValid || resp.FailureReason != nil || resp.ExpiresAt != nil || resp.Message == "" {
		t.Fatalf("expected a paid payment with valid tickets, got %+v", resp)
	}
	for _, id := range []uuid.UUID{first, second} {
		if item := paymentItemFor(resp, id); item == nil || item.BookingStatus != model.BookingStatusConfirmed || !item.TicketsValid {
			t.Fatalf("expected the confirmed item %s, got %+v", id, item)
		}
		s := stored(t, f.db, id)
		if *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
			t.Errorf("expected stored paid/confirmed, got %s/%s", *s.PaymentStatus, s.BookingStatus)
		}
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 2 {
		t.Errorf("expected 2 sold tickets, got %d", n)
	}
}

func TestSimulatePayment_FailureLeavesNoTicketClaim(t *testing.T) {
	f := newPaymentFixture(t, 2, "20.00")
	bookingID := f.reserve(t, 2)
	pending := f.checkout(t)

	resp, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentFailure, decimal.NewFromInt(40))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.TicketsValid {
		t.Errorf("expected a failed payment without valid tickets, got %+v", resp)
	}
	if resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureDeclined || resp.Message == "" {
		t.Errorf("expected declined failure with a message, got %+v", resp)
	}
	if item := paymentItemFor(resp, bookingID); item == nil || item.BookingStatus != model.BookingStatusFailed || item.TicketsValid {
		t.Errorf("expected the failed item, got %+v", item)
	}

	s := stored(t, f.db, bookingID)
	if *s.PaymentStatus != model.PaymentStatusFailed || *s.FailureReason != model.PaymentFailureDeclined || s.BookingStatus != model.BookingStatusFailed {
		t.Errorf("expected stored failed/declined/failed, got %+v", s)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 0 {
		t.Errorf("a failed payment must not sell tickets, got %d", n)
	}
	// The tickets of the failed booking are free again.
	if _, err := f.carts.AddToCart(context.Background(), f.userID, f.eventID, 2); err != nil {
		t.Errorf("expected the released tickets to be bookable, got %v", err)
	}
}

func TestSimulatePayment_RejectsManipulatedAmount(t *testing.T) {
	f := newPaymentFixture(t, 10, "19.99")
	f.reserve(t, 3)
	pending := f.checkout(t)

	// 1e2000000000 also checks that the comparison does not stall on a huge exponent.
	for _, amount := range []string{"0", "0.01", "59.96", "59.98", "59.971", "-59.97", "5997", "100000000000", "1e2000000000", "5997e-2000000000"} {
		_, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString(amount))
		if !errors.Is(err, ErrPaymentAmountMismatch) {
			t.Errorf("amount %s: expected ErrPaymentAmountMismatch, got %v", amount, err)
		}
	}
	s := stored(t, f.db, pending.Items[0].BookingID)
	if *s.PaymentStatus != model.PaymentStatusPending || s.BookingStatus != model.BookingStatusReserved {
		t.Fatalf("rejected amounts must not change anything, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}

	// The same value with another scale is the stored amount.
	resp, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.RequireFromString("59.970"))
	if err != nil || resp.Status != model.PaymentStatusPaid {
		t.Errorf("expected the stored amount to be accepted, got %v %+v", err, resp)
	}
}

func TestSimulatePayment_ExpiredReservationFails(t *testing.T) {
	f := newPaymentFixture(t, 1, "20.00")
	bookingID := f.reserve(t, 1)
	pending := f.checkout(t)
	expireReservation(t, f.db, bookingID)

	// The expired reservation released its ticket, which someone else has reserved meanwhile.
	other := f.seedUser(t)
	if _, err := f.carts.AddToCart(context.Background(), other, f.eventID, 1); err != nil {
		t.Fatalf("reserve released ticket: %v", err)
	}

	resp, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureReservationExpired {
		t.Errorf("expected failed payment with reservation_expired, got %+v", resp)
	}
	if item := paymentItemFor(resp, bookingID); item == nil || item.BookingStatus != model.BookingStatusExpired || item.TicketsValid {
		t.Errorf("expected the expired item without valid tickets, got %+v", item)
	}
	if n := confirmedTickets(t, f.db, f.eventID); n != 0 {
		t.Errorf("the expired reservation must not be confirmed, got %d sold tickets", n)
	}
}

func TestSimulatePayment_WithdrawnEventFailsTheWholeCheckout(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	music := f.eventID
	food := seedEventFor(t, f.db, 10, model.EventStatusPublished, "5", f.userID)
	musicItem := f.reserve(t, 1)
	foodItem := addToCartEvent(t, f.carts, f.userID, food, 1)
	pending := f.checkout(t)
	mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", music)

	resp, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(25))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if resp.Status != model.PaymentStatusFailed || resp.FailureReason == nil || *resp.FailureReason != model.PaymentFailureEventUnavailable {
		t.Errorf("expected failed payment with event_unavailable, got %+v", resp)
	}
	// The payment completes all items together or none: the item of the intact event fails too.
	for _, id := range []uuid.UUID{musicItem, foodItem} {
		if item := paymentItemFor(resp, id); item == nil || item.BookingStatus != model.BookingStatusFailed || item.TicketsValid {
			t.Errorf("expected the failed item %s, got %+v", id, item)
		}
	}
}

func TestSimulatePayment_CompletesOnlyOnce(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)
	ctx := context.Background()

	if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
		t.Fatalf("simulate: %v", err)
	}
	for _, outcome := range []model.SimulatedPaymentOutcome{model.SimulatedPaymentFailure, model.SimulatedPaymentSuccess} {
		if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, outcome, decimal.NewFromInt(20)); !errors.Is(err, ErrPaymentAlreadyProcessed) {
			t.Errorf("%s after success: expected ErrPaymentAlreadyProcessed, got %v", outcome, err)
		}
	}
	if s := stored(t, f.db, pending.Items[0].BookingID); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("a completed payment must stay paid, got %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
}

func TestSimulatePayment_ConcurrentCompletionsSettleOnce(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)

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
			_, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, outcome, decimal.NewFromInt(20))
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
	s := stored(t, f.db, pending.Items[0].BookingID)
	paid := *s.PaymentStatus == model.PaymentStatusPaid && s.BookingStatus == model.BookingStatusConfirmed
	failed := *s.PaymentStatus == model.PaymentStatusFailed && s.BookingStatus == model.BookingStatusFailed
	if !paid && !failed {
		t.Errorf("payment and booking disagree: %s/%s", *s.PaymentStatus, s.BookingStatus)
	}
}

// Checking out, completing and reserving lock the same rows; mixing them must neither deadlock
// nor overbook.
func TestPayment_ConcurrentCheckoutSimulateAndAddToCart(t *testing.T) {
	f := newPaymentFixture(t, 3, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)

	const rounds = 10
	var wg sync.WaitGroup
	completed := make(chan struct{}, rounds)
	for i := 0; i < rounds; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			// A repeated checkout returns the open payment, or nothing left to pay.
			_, _, err := f.carts.Checkout(context.Background(), f.userID)
			if err != nil && !errors.Is(err, ErrCartEmpty) {
				t.Errorf("checkout: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := f.payments.SimulatePayment(context.Background(), pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
			switch {
			case err == nil:
				completed <- struct{}{}
			case !errors.Is(err, ErrPaymentAlreadyProcessed):
				t.Errorf("simulate: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			userID := f.seedUser(t)
			var capErr *CapacityExceededError
			if _, err := f.carts.AddToCart(context.Background(), userID, f.eventID, 1); err != nil && !errors.As(err, &capErr) {
				t.Errorf("add to cart: %v", err)
			}
		}()
	}
	wg.Wait()
	close(completed)

	if len(completed) != 1 {
		t.Errorf("expected exactly one completion, got %d", len(completed))
	}
	if s := stored(t, f.db, pending.Items[0].BookingID); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
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

	t.Run("unknown payment", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		if _, err := f.payments.SimulatePayment(ctx, uuid.New(), f.userID, model.SimulatedPaymentSuccess, amount); !errors.Is(err, ErrPaymentNotFound) {
			t.Errorf("expected ErrPaymentNotFound, got %v", err)
		}
	})
	t.Run("payment of another user", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		f.reserve(t, 1)
		pending := f.checkout(t)
		if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.seedUser(t), model.SimulatedPaymentSuccess, amount); !errors.Is(err, ErrPaymentNotFound) {
			t.Errorf("expected ErrPaymentNotFound, got %v", err)
		}
		if s := stored(t, f.db, pending.Items[0].BookingID); *s.PaymentStatus != model.PaymentStatusPending {
			t.Errorf("payment must stay pending, got %s", *s.PaymentStatus)
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
	f.reserve(t, 1)
	ctx := context.Background()

	if _, err := f.payments.GetPayment(ctx, uuid.New(), f.userID); !errors.Is(err, ErrPaymentNotFound) {
		t.Errorf("expected ErrPaymentNotFound for an unknown payment, got %v", err)
	}
	pending := f.checkout(t)
	bookingID := pending.Items[0].BookingID

	// The result reflects the stored status, also when it was changed outside the service.
	mustExec(t, f.db, "UPDATE payments SET status = 'refunded' WHERE payment_id = ?", pending.PaymentID)
	mustExec(t, f.db, "UPDATE bookings SET status = 'cancelled' WHERE booking_id = ?", bookingID)
	resp, err := f.payments.GetPayment(ctx, pending.PaymentID, f.userID)
	if err != nil {
		t.Fatalf("get payment: %v", err)
	}
	if resp.Status != model.PaymentStatusRefunded || resp.TicketsValid {
		t.Errorf("expected stored refunded without valid tickets, got %+v", resp)
	}
	if item := paymentItemFor(resp, bookingID); item == nil || item.BookingStatus != model.BookingStatusCancelled {
		t.Errorf("expected the cancelled item, got %+v", item)
	}
	if _, err := f.payments.GetPayment(ctx, pending.PaymentID, f.seedUser(t)); !errors.Is(err, ErrPaymentNotFound) {
		t.Errorf("expected ErrPaymentNotFound for another user, got %v", err)
	}
}

func TestGetPayment_ReflectsEventState(t *testing.T) {
	ctx := context.Background()

	t.Run("pending payment of a withdrawn event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		f.reserve(t, 1)
		pending := f.checkout(t)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, pending.PaymentID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if resp.Status != model.PaymentStatusPending || !strings.Contains(resp.Message, "event is no longer available") {
			t.Errorf("expected the stored pending status with a message about the event, got %+v", resp)
		}
	})
	t.Run("paid tickets of a cancelled event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		f.reserve(t, 1)
		paid := f.pay(t, model.SimulatedPaymentSuccess)
		mustExec(t, f.db, "UPDATE events SET status = 'cancelled' WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, paid.PaymentID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if resp.Status != model.PaymentStatusPaid || resp.TicketsValid {
			t.Errorf("tickets of a cancelled event must not be valid, got %+v", resp)
		}
	})
	t.Run("paid tickets of a deleted event", func(t *testing.T) {
		f := newPaymentFixture(t, 10, "20")
		f.reserve(t, 1)
		paid := f.pay(t, model.SimulatedPaymentSuccess)
		mustExec(t, f.db, "DELETE FROM events WHERE event_id = ?", f.eventID)

		resp, err := f.payments.GetPayment(ctx, paid.PaymentID, f.userID)
		if err != nil {
			t.Fatalf("get payment: %v", err)
		}
		if item := paymentItemFor(resp, paid.Items[0].BookingID); item == nil || item.Event != nil || item.TicketsValid {
			t.Errorf("tickets of a deleted event must not be valid, got %+v", item)
		}
	})
}

// GET must never combine the booking state from before a completion with the payment after it.
func TestGetPayment_ConsistentWhileCompleting(t *testing.T) {
	f := newPaymentFixture(t, 10, "20.00")
	f.reserve(t, 1)
	pending := f.checkout(t)
	paymentID, bookingID := pending.PaymentID, pending.Items[0].BookingID

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
				resp, err := f.payments.GetPayment(context.Background(), paymentID, f.userID)
				if err != nil {
					t.Errorf("get payment: %v", err)
					return
				}
				pending := resp.Status == model.PaymentStatusPending && len(resp.Items) == 1 &&
					resp.Items[0].BookingID == bookingID && resp.Items[0].BookingStatus == model.BookingStatusReserved
				paid := resp.Status == model.PaymentStatusPaid && resp.TicketsValid &&
					resp.Items[0].BookingStatus == model.BookingStatusConfirmed
				if !pending && !paid {
					t.Errorf("inconsistent result: %s/%s ticketsValid=%v", resp.Status, resp.Items[0].BookingStatus, resp.TicketsValid)
					return
				}
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := f.payments.SimulatePayment(context.Background(), paymentID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(20)); err != nil {
		t.Errorf("simulate: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
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

// paymentViewWith builds a payment view of one item; the event may be nil for a deleted event.
func paymentViewWith(payment model.PaymentStatus, reason *model.PaymentFailureReason, booking model.BookingStatus, expires *time.Time, event *model.EventStatus) *repository.PaymentView {
	line := decimal.NewFromInt(40)
	return &repository.PaymentView{
		Payment: &model.PaymentModel{PaymentID: uuid.New(), Amount: line, Status: payment, FailureReason: reason},
		Items: []*repository.PaymentItem{{
			Booking:     &model.BookingModel{BookingID: uuid.New(), Status: booking, ExpiresAt: expires, LineAmount: &line},
			EventStatus: event,
		}},
		Now: time.Now(),
	}
}

// The payment result reports only paid and confirmed tickets of events that take place, and its
// message never contradicts the result: only valid tickets are confirmed, only a payable payment
// asks to be completed.
func TestPaymentResponse_OnlyPaidConfirmedTicketsOfHeldEventsAreValid(t *testing.T) {
	declined := model.PaymentFailureDeclined
	expired := model.PaymentFailureReservationExpired
	unavailable := model.PaymentFailureEventUnavailable
	cancelledReason := model.PaymentFailureBookingCancelled
	published, completed, cancelled := model.EventStatusPublished, model.EventStatusCompleted, model.EventStatusCancelled
	now := time.Now()
	live, past := now.Add(time.Minute), now.Add(-time.Minute)

	cases := []struct {
		name        string
		view        *repository.PaymentView
		valid       bool
		completable bool
	}{
		{"paid for published event", paymentViewWith(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &published), true, false},
		{"paid for completed event", paymentViewWith(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &completed), true, false},
		{"paid for cancelled event", paymentViewWith(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, &cancelled), false, false},
		{"paid for deleted event", paymentViewWith(model.PaymentStatusPaid, nil, model.BookingStatusConfirmed, nil, nil), false, false},
		{"paid for cancelled booking", paymentViewWith(model.PaymentStatusPaid, nil, model.BookingStatusCancelled, nil, &published), false, false},
		{"pending and live", paymentViewWith(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, &published), false, true},
		{"pending and expired", paymentViewWith(model.PaymentStatusPending, nil, model.BookingStatusReserved, &past, &published), false, false},
		{"pending for withdrawn event", paymentViewWith(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, &cancelled), false, false},
		{"pending for deleted event", paymentViewWith(model.PaymentStatusPending, nil, model.BookingStatusReserved, &live, nil), false, false},
		{"pending but booking not reserved", paymentViewWith(model.PaymentStatusPending, nil, model.BookingStatusConfirmed, &live, &published), false, false},
		{"refunded", paymentViewWith(model.PaymentStatusRefunded, nil, model.BookingStatusConfirmed, nil, &published), false, false},
		{"failed: booking cancelled", paymentViewWith(model.PaymentStatusFailed, &cancelledReason, model.BookingStatusCancelled, nil, &published), false, false},
		{"failed: declined", paymentViewWith(model.PaymentStatusFailed, &declined, model.BookingStatusFailed, nil, &published), false, false},
		{"failed: reservation expired", paymentViewWith(model.PaymentStatusFailed, &expired, model.BookingStatusExpired, nil, &published), false, false},
		{"failed: event unavailable", paymentViewWith(model.PaymentStatusFailed, &unavailable, model.BookingStatusFailed, nil, &cancelled), false, false},
		{"failed without reason", paymentViewWith(model.PaymentStatusFailed, nil, model.BookingStatusFailed, nil, &published), false, false},
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
		if !tc.valid && resp.Message == success {
			t.Errorf("%s: invalid tickets must not be reported as confirmed tickets", tc.name)
		}
		if (resp.Message == completable) != tc.completable {
			t.Errorf("%s: only a payable checkout may be asked to complete its payment, got %q", tc.name, resp.Message)
		}
		messages[resp.Message] = true
	}
	// pending (payable, reservation invalid, event gone), paid (valid, event gone, booking
	// inactive), refunded, generic failure and one message per failure reason
	if len(messages) != 12 {
		t.Errorf("expected 12 distinct messages, got %d: %v", len(messages), messages)
	}
}

func TestExpireStalePayments(t *testing.T) {
	f := newPaymentFixture(t, 20, "20")
	ctx := context.Background()

	// A paid payment must stay untouched by the cleanup.
	paidItem := f.reserve(t, 1)
	paid := f.pay(t, model.SimulatedPaymentSuccess)

	// A checkout gives its reservations one expiry; once it lapses the cleanup fails the payment.
	stale := f.reserve(t, 1)
	stalePayment := f.checkout(t)
	expireReservation(t, f.db, stale)

	// A reservation without a payment is no payment process; it stays as stored. It is added
	// after the user's last checkout, or that checkout would sweep it in.
	withoutPayment := f.reserve(t, 1)
	expireReservation(t, f.db, withoutPayment)

	// A user holds at most one pending payment, so the live one belongs to another user.
	liveUser := f.seedUser(t)
	live := addToCartEvent(t, f.carts, liveUser, f.eventID, 1)
	livePayment := f.checkoutFor(t, liveUser)

	// A stale payment of a withdrawn event fails for that reason; its booking is expired,
	// because its reservation lapsed.
	otherEvent, otherUser := seedEvent(t, f.db, 5, model.EventStatusPublished)
	other := addToCartEvent(t, f.carts, otherUser, otherEvent, 1)
	otherPayment := f.checkoutFor(t, otherUser)
	expireReservation(t, f.db, other)
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
		stale: {model.PaymentFailureReservationExpired, model.BookingStatusExpired},
		other: {model.PaymentFailureEventUnavailable, model.BookingStatusExpired},
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
	if s := stored(t, f.db, paid.Items[0].BookingID); *s.PaymentStatus != model.PaymentStatusPaid || s.BookingStatus != model.BookingStatusConfirmed {
		t.Errorf("a paid booking must stay confirmed, got %+v", s)
	}
	if paid.Items[0].BookingID != paidItem {
		t.Fatalf("the paid fixture must cover its own item")
	}
	if stalePayment.Status != model.PaymentStatusPending || livePayment.Status != model.PaymentStatusPending || otherPayment.Status != model.PaymentStatusPending {
		t.Fatal("the fixtures must start pending")
	}

	if n, err := f.payments.ExpireStalePayments(ctx); err != nil || n != 0 {
		t.Errorf("a second run must find nothing, got %d %v", n, err)
	}
}

// One run processes one batch of payments; the rest waits for the next run.
func TestExpireStalePayments_ProcessesOneBatchPerRun(t *testing.T) {
	previous := stalePaymentBatch
	stalePaymentBatch = 2
	t.Cleanup(func() { stalePaymentBatch = previous })

	f := newPaymentFixture(t, 10, "20")
	// A user holds at most one pending payment, so every stale payment needs its own user.
	for i := 0; i < 5; i++ {
		user := f.seedUser(t)
		bookingID := addToCartEvent(t, f.carts, user, f.eventID, 1)
		f.checkoutFor(t, user)
		expireReservation(t, f.db, bookingID)
	}
	for run, want := range []int{2, 2, 1, 0} {
		if n, err := f.payments.ExpireStalePayments(context.Background()); err != nil || n != want {
			t.Errorf("run %d: expected %d failed payments, got %d %v", run, want, n, err)
		}
	}
}

// The cleanup must neither wait for nor interfere with a booking that is being completed.
func TestExpireStalePayments_SkipsLockedBookings(t *testing.T) {
	f := newPaymentFixture(t, 10, "20")
	ctx := context.Background()
	bookingID := f.reserve(t, 1)
	f.checkout(t)
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
	payments := make([]*model.PaymentResponse, bookings)
	// A user holds at most one pending payment, so every racing payment needs its own user.
	users := make([]uuid.UUID, bookings)
	for i := range payments {
		users[i] = f.seedUser(t)
		bookingID := addToCartEvent(t, f.carts, users[i], f.eventID, 1)
		payments[i] = f.checkoutFor(t, users[i])
		expireReservation(t, f.db, bookingID)
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
	for i, pending := range payments {
		paymentID, owner := pending.PaymentID, users[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.payments.SimulatePayment(ctx, paymentID, owner, model.SimulatedPaymentSuccess, decimal.NewFromInt(20))
			if err != nil && !errors.Is(err, ErrPaymentAlreadyProcessed) {
				t.Errorf("simulate: %v", err)
			}
		}()
	}
	wg.Wait()

	for _, pending := range payments {
		s := stored(t, f.db, pending.Items[0].BookingID)
		if *s.PaymentStatus != model.PaymentStatusFailed || s.BookingStatus != model.BookingStatusExpired {
			t.Errorf("booking %s: expected failed/expired, got %s/%s", pending.Items[0].BookingID, *s.PaymentStatus, s.BookingStatus)
		}
	}
}
