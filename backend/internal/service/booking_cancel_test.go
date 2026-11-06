package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// cancelRepo fakes the repositories CancelBooking uses. The embedded interfaces provide the
// methods the service never calls on these paths; calling them would panic, like a nil map.
type cancelRepo struct {
	repository.BookingRepository
	repository.PaymentRepository
	repository.EventRepository
	bookings       []*model.BookingModel
	events         []*model.EventModel
	updated        []uuid.UUID
	view           *repository.PaymentView
	payment        *model.PaymentModel
	paymentUpdates int
	paymentErr     error
}

func (r *cancelRepo) find(bookingID uuid.UUID) *model.BookingModel {
	for _, booking := range r.bookings {
		if booking.BookingID == bookingID {
			return booking
		}
	}
	return nil
}

func (r *cancelRepo) GetBookingByID(bookingID uuid.UUID) (*model.BookingModel, error) {
	return r.find(bookingID), nil
}
func (r *cancelRepo) LockBooking(bookingID uuid.UUID) (*model.BookingModel, error) {
	return r.find(bookingID), nil
}
func (r *cancelRepo) LockBookings(_ []uuid.UUID) ([]*model.BookingModel, error) {
	return r.bookings, nil
}
func (r *cancelRepo) UpdateBooking(booking *model.BookingModel) error {
	r.updated = append(r.updated, booking.BookingID)
	return nil
}
func (r *cancelRepo) GetPaymentView(uuid.UUID) (*repository.PaymentView, error) {
	return r.view, nil
}
func (r *cancelRepo) LockPayment(uuid.UUID) (*model.PaymentModel, error) {
	return r.payment, nil
}
func (r *cancelRepo) UpdatePayment(*model.PaymentModel) error {
	r.paymentUpdates++
	return r.paymentErr
}
func (r *cancelRepo) LockEvents([]uuid.UUID) ([]*model.EventModel, error) {
	return r.events, nil
}

// cancelTransactor runs the transaction on the fake repos without a database.
type cancelTransactor struct{ repo *cancelRepo }

func (t *cancelTransactor) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	return fn(repository.Tx{Events: t.repo, Bookings: t.repo, Payments: t.repo})
}

func newCancelService(repo *cancelRepo) *BookingService {
	return NewBookingService(&cancelTransactor{repo: repo}, repo, repo)
}

// A reservation without a payment is a cart item and is cancelled on its own.
func TestCancelBooking_CartItemStatusAndOwnership(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		name   string
		status model.BookingStatus
		user   *uuid.UUID
		want   error
		save   bool
	}{
		{"reserved", model.BookingStatusReserved, &owner, nil, true},
		{"cancelled", model.BookingStatusCancelled, &owner, nil, false},
		{"confirmed", model.BookingStatusConfirmed, &owner, ErrBookingNotCancellable, false},
		{"failed", model.BookingStatusFailed, &owner, ErrBookingNotCancellable, false},
		{"expired", model.BookingStatusExpired, &owner, ErrBookingNotCancellable, false},
		{"other owner", model.BookingStatusReserved, func() *uuid.UUID { id := uuid.New(); return &id }(), ErrBookingNotFound, false},
		{"deleted owner", model.BookingStatusReserved, nil, ErrBookingNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cancelRepo{bookings: []*model.BookingModel{{BookingID: uuid.New(), UserID: tc.user, Status: tc.status}}}
			result, err := newCancelService(repo).CancelBooking(context.Background(), repo.bookings[0].BookingID, owner)
			if !errors.Is(err, tc.want) || len(repo.updated) != b2i(tc.save) {
				t.Fatalf("err=%v updated=%v", err, repo.updated)
			}
			if err == nil && (result.Status != model.BookingStatusCancelled || result.ExpiresAt != nil) {
				t.Fatalf("unexpected booking: %+v", result)
			}
		})
	}
	repo := &cancelRepo{}
	if _, err := newCancelService(repo).CancelBooking(context.Background(), uuid.New(), owner); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("missing booking: %v", err)
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Cancelling one booking of a pending payment cancels the whole checkout, because the payment
// amount covers all of its bookings.
func TestCancelBooking_PendingCancelsTheWholeCheckout(t *testing.T) {
	owner := uuid.New()
	target, other := uuid.New(), uuid.New()
	paymentID := uuid.New()
	repo := &cancelRepo{
		bookings: []*model.BookingModel{
			{BookingID: target, UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusReserved},
			{BookingID: other, UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusReserved},
		},
		payment: &model.PaymentModel{PaymentID: paymentID, Amount: decimal.NewFromInt(40), Status: model.PaymentStatusPending},
	}
	repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{
		{Booking: repo.bookings[0]}, {Booking: repo.bookings[1]},
	}}

	if _, err := newCancelService(repo).CancelBooking(context.Background(), target, owner); err != nil {
		t.Fatal(err)
	}
	for _, booking := range repo.bookings {
		if booking.Status != model.BookingStatusCancelled || booking.ExpiresAt != nil {
			t.Fatalf("every booking must be cancelled: %+v", booking)
		}
	}
	if repo.payment.Status != model.PaymentStatusFailed || repo.payment.FailureReason == nil ||
		*repo.payment.FailureReason != model.PaymentFailureBookingCancelled || !repo.payment.RefundAmount.IsZero() {
		t.Fatalf("unexpected payment: %+v", repo.payment)
	}
	if len(repo.updated) != 2 || repo.paymentUpdates != 1 {
		t.Fatalf("updated=%v paymentUpdates=%d", repo.updated, repo.paymentUpdates)
	}
}

// A paid payment refunds the line amount of the cancelled booking; it counts as refunded once
// every booking of it is cancelled. Repeated cancellation never refunds twice.
func TestCancelBooking_PaidRefundsTheLineAmount(t *testing.T) {
	owner := uuid.New()
	for _, amount := range []string{"0", "24.99", "9999999999.99"} {
		t.Run(amount, func(t *testing.T) {
			line := decimal.RequireFromString(amount)
			paymentID := uuid.New()
			repo := &cancelRepo{
				bookings: []*model.BookingModel{{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID,
					Status: model.BookingStatusConfirmed, LineAmount: &line}},
				payment: &model.PaymentModel{PaymentID: paymentID, Amount: line, Status: model.PaymentStatusPaid},
			}
			repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{{Booking: repo.bookings[0]}}}
			svc := newCancelService(repo)
			for i := 0; i < 2; i++ {
				if _, err := svc.CancelBooking(context.Background(), repo.bookings[0].BookingID, owner); err != nil {
					t.Fatal(err)
				}
			}
			if repo.payment.Status != model.PaymentStatusRefunded || !repo.payment.RefundAmount.Equal(line) ||
				repo.paymentUpdates != 1 || len(repo.updated) != 1 {
				t.Fatalf("payment=%+v updates=%d updated=%v", repo.payment, repo.paymentUpdates, repo.updated)
			}
		})
	}

	// One payment, two bookings: cancelling one refunds its line only.
	t.Run("partial refund", func(t *testing.T) {
		paymentID := uuid.New()
		first, second := decimal.NewFromInt(30), decimal.NewFromInt(20)
		repo := &cancelRepo{
			bookings: []*model.BookingModel{
				{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed, LineAmount: &first},
				{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed, LineAmount: &second},
			},
			payment: &model.PaymentModel{PaymentID: paymentID, Amount: decimal.NewFromInt(50), Status: model.PaymentStatusPaid},
		}
		repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{
			{Booking: repo.bookings[0]}, {Booking: repo.bookings[1]},
		}}
		svc := newCancelService(repo)

		if _, err := svc.CancelBooking(context.Background(), repo.bookings[0].BookingID, owner); err != nil {
			t.Fatal(err)
		}
		if repo.payment.Status != model.PaymentStatusPaid || !repo.payment.RefundAmount.Equal(first) {
			t.Fatalf("the payment must stay paid with a partial refund: %+v", repo.payment)
		}
		if _, err := svc.CancelBooking(context.Background(), repo.bookings[1].BookingID, owner); err != nil {
			t.Fatal(err)
		}
		if repo.payment.Status != model.PaymentStatusRefunded || !repo.payment.RefundAmount.Equal(repo.payment.Amount) {
			t.Fatalf("the payment must count as refunded now: %+v", repo.payment)
		}
		if repo.paymentUpdates != 2 {
			t.Fatalf("each cancellation updates the payment once, got %d", repo.paymentUpdates)
		}
	})

	// A free booking adds nothing to the refund: cancelling the paid booking next to it refunds
	// the whole amount, yet the payment stays paid, so the free tickets stay valid.
	t.Run("free booking stays valid", func(t *testing.T) {
		paymentID := uuid.New()
		free, paid := decimal.Zero, decimal.NewFromInt(20)
		repo := &cancelRepo{
			bookings: []*model.BookingModel{
				{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed, LineAmount: &free},
				{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed, LineAmount: &paid},
			},
			payment: &model.PaymentModel{PaymentID: paymentID, Amount: paid, Status: model.PaymentStatusPaid},
		}
		repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{
			{Booking: repo.bookings[0]}, {Booking: repo.bookings[1]},
		}}
		svc := newCancelService(repo)

		if _, err := svc.CancelBooking(context.Background(), repo.bookings[1].BookingID, owner); err != nil {
			t.Fatal(err)
		}
		if repo.payment.Status != model.PaymentStatusPaid || !repo.payment.RefundAmount.Equal(paid) ||
			repo.bookings[0].Status != model.BookingStatusConfirmed {
			t.Fatalf("the free booking must stay valid: payment=%+v free=%+v", repo.payment, repo.bookings[0])
		}
		if _, err := svc.CancelBooking(context.Background(), repo.bookings[0].BookingID, owner); err != nil {
			t.Fatal(err)
		}
		if repo.payment.Status != model.PaymentStatusRefunded || !repo.payment.RefundAmount.Equal(paid) {
			t.Fatalf("the payment must count as refunded now: %+v", repo.payment)
		}
	})
}

// The organizer settles the refund of a withdrawn event, so the visitor cannot cancel a paid
// booking of it anymore; its tickets keep showing as cancelled by the organizer. Repeating the
// cancellation of a booking cancelled before the withdrawal still succeeds, and a pending
// checkout of a withdrawn event can still be cancelled, as nothing was charged.
func TestCancelBooking_WithdrawnEvent(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		name          string
		paymentStatus model.PaymentStatus
		bookingStatus model.BookingStatus
		want          error
		updates       int
	}{
		{"paid", model.PaymentStatusPaid, model.BookingStatusConfirmed, ErrBookingEventCancelled, 0},
		{"cancelled before the withdrawal", model.PaymentStatusRefunded, model.BookingStatusCancelled, nil, 0},
		{"pending checkout", model.PaymentStatusPending, model.BookingStatusReserved, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eventID, paymentID := uuid.New(), uuid.New()
			line := decimal.NewFromInt(20)
			repo := &cancelRepo{
				bookings: []*model.BookingModel{{BookingID: uuid.New(), UserID: &owner, EventID: &eventID,
					PaymentID: &paymentID, Status: tc.bookingStatus, LineAmount: &line}},
				events:  []*model.EventModel{{EventID: eventID, Status: model.EventStatusCancelled}},
				payment: &model.PaymentModel{PaymentID: paymentID, Amount: line, Status: tc.paymentStatus},
			}
			repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{{Booking: repo.bookings[0]}}}

			_, err := newCancelService(repo).CancelBooking(context.Background(), repo.bookings[0].BookingID, owner)
			if !errors.Is(err, tc.want) || repo.paymentUpdates != tc.updates {
				t.Fatalf("err=%v paymentUpdates=%d", err, repo.paymentUpdates)
			}
			if tc.want != nil && (!errors.Is(err, ErrBookingNotCancellable) || len(repo.updated) != 0 ||
				repo.bookings[0].Status != model.BookingStatusConfirmed || repo.payment.Status != model.PaymentStatusPaid) {
				t.Fatalf("a rejected cancellation must change nothing: err=%v updated=%v payment=%+v", err, repo.updated, repo.payment)
			}
		})
	}
}

// A failed payment update aborts the transaction, so nothing is persisted.
func TestCancelBooking_PaymentUpdateFailureIsReturned(t *testing.T) {
	owner, paymentID := uuid.New(), uuid.New()
	want := errors.New("refund update failed")
	line := decimal.RequireFromString("24.99")
	repo := &cancelRepo{
		bookings: []*model.BookingModel{{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID,
			Status: model.BookingStatusConfirmed, LineAmount: &line}},
		payment:    &model.PaymentModel{PaymentID: paymentID, Amount: line, Status: model.PaymentStatusPaid},
		paymentErr: want,
	}
	repo.view = &repository.PaymentView{Payment: repo.payment, Items: []*repository.PaymentItem{{Booking: repo.bookings[0]}}}
	if _, err := newCancelService(repo).CancelBooking(context.Background(), repo.bookings[0].BookingID, owner); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestCancelBooking_RefundPersistedOnce(t *testing.T) {
	f := newPaymentFixture(t, 2, "12.49")
	id := f.reserve(t, 2)
	pending := f.checkout(t)
	ctx := context.Background()
	if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.bookings.CancelBooking(ctx, id, f.userID); err != nil {
			t.Fatal(err)
		}
	}
	payment, err := repository.NewPaymentRepository(f.db).GetPayment(pending.PaymentID)
	if err != nil {
		t.Fatal(err)
	}
	if payment.Status != model.PaymentStatusRefunded || !payment.RefundAmount.Equal(decimal.RequireFromString("24.98")) {
		t.Fatalf("unexpected payment: %+v", payment)
	}
	result, err := f.payments.GetPayment(ctx, pending.PaymentID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if item := paymentItemFor(result, id); item == nil || item.BookingStatus != model.BookingStatusCancelled || item.TicketsValid {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Status != model.PaymentStatusRefunded || result.TicketsValid {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := f.carts.AddToCart(ctx, f.userID, f.eventID, 2); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestCancelBooking_PendingPaymentAndCapacity(t *testing.T) {
	f := newPaymentFixture(t, 1, "10")
	other := seedEventFor(t, f.db, 1, model.EventStatusPublished, "10", f.userID)
	first := f.reserve(t, 1)
	second := addToCartEvent(t, f.carts, f.userID, other, 1)
	pending := f.checkout(t)
	ctx := context.Background()

	// Cancelling one booking of the pending checkout cancels all of them.
	if _, err := f.bookings.CancelBooking(ctx, first, f.userID); err != nil {
		t.Fatal(err)
	}
	result, err := f.payments.GetPayment(ctx, pending.PaymentID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.PaymentStatusFailed || result.FailureReason == nil ||
		*result.FailureReason != model.PaymentFailureBookingCancelled || result.TicketsValid {
		t.Fatalf("unexpected payment: %+v", result)
	}
	for _, id := range []uuid.UUID{first, second} {
		if item := paymentItemFor(result, id); item == nil || item.BookingStatus != model.BookingStatusCancelled {
			t.Fatalf("the whole checkout must be cancelled: %+v", result.Items)
		}
	}
	if _, err := f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount); !errors.Is(err, ErrPaymentAlreadyProcessed) {
		t.Fatalf("payment after cancel: %v", err)
	}
	for _, event := range []uuid.UUID{f.eventID, other} {
		if _, err := f.carts.AddToCart(ctx, f.userID, event, 1); err != nil {
			t.Fatalf("capacity of %s not released: %v", event, err)
		}
	}
}

func TestCancelBooking_ConcurrentPayment(t *testing.T) {
	f := newPaymentFixture(t, 1, "10")
	id := f.reserve(t, 1)
	pending := f.checkout(t)
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var cancelErr, paymentErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _, cancelErr = f.bookings.CancelBooking(ctx, id, f.userID) }()
	go func() {
		defer wg.Done()
		<-start
		_, paymentErr = f.payments.SimulatePayment(ctx, pending.PaymentID, f.userID, model.SimulatedPaymentSuccess, pending.Amount)
	}()
	close(start)
	wg.Wait()
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	if paymentErr != nil && !errors.Is(paymentErr, ErrPaymentAlreadyProcessed) {
		t.Fatal(paymentErr)
	}
	result, err := f.payments.GetPayment(ctx, pending.PaymentID, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if item := paymentItemFor(result, id); item == nil || item.BookingStatus != model.BookingStatusCancelled || item.TicketsValid ||
		(result.Status != model.PaymentStatusRefunded && result.Status != model.PaymentStatusFailed) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := f.carts.AddToCart(ctx, f.userID, f.eventID, 1); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}
