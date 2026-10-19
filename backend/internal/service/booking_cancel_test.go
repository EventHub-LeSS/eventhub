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

type cancelBookingRepo struct {
	repository.BookingRepository
	repository.BookingTx
	booking        *model.BookingModel
	saved          bool
	payment        *model.PaymentModel
	paymentUpdates int
	paymentErr     error
}

func (r *cancelBookingRepo) InTransaction(ctx context.Context, fn func(repository.BookingTx) error) error {
	return fn(r)
}
func (r *cancelBookingRepo) LockBooking(uuid.UUID) (*model.BookingModel, error) {
	return r.booking, nil
}
func (r *cancelBookingRepo) UpdateBooking(*model.BookingModel) error       { r.saved = true; return nil }
func (r *cancelBookingRepo) CountOccupiedTickets(uuid.UUID) (int64, error) { return 0, nil }

func (r *cancelBookingRepo) GetPayment(uuid.UUID) (*model.PaymentModel, error) { return r.payment, nil }
func (r *cancelBookingRepo) UpdatePayment(*model.PaymentModel) error {
	r.paymentUpdates++
	return r.paymentErr
}

func TestCancelBooking_FullRefund(t *testing.T) {
	owner, paymentID := uuid.New(), uuid.New()
	for _, amount := range []string{"0", "24.99", "9999999999.99"} {
		t.Run(amount, func(t *testing.T) {
			repo := &cancelBookingRepo{
				booking: &model.BookingModel{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed},
				payment: &model.PaymentModel{PaymentID: paymentID, Amount: decimal.RequireFromString(amount), Status: model.PaymentStatusPaid},
			}
			svc := NewBookingService(repo, 0)
			for i := 0; i < 2; i++ {
				if _, err := svc.CancelBooking(context.Background(), repo.booking.BookingID, owner); err != nil {
					t.Fatal(err)
				}
			}
			if repo.payment.Status != model.PaymentStatusRefunded || !repo.payment.RefundAmount.Equal(repo.payment.Amount) || repo.paymentUpdates != 1 {
				t.Fatalf("payment=%+v updates=%d", repo.payment, repo.paymentUpdates)
			}
		})
	}
}

func TestCancelBooking_RefundFailureDoesNotSaveCancellation(t *testing.T) {
	owner, paymentID := uuid.New(), uuid.New()
	want := errors.New("refund update failed")
	repo := &cancelBookingRepo{
		booking:    &model.BookingModel{BookingID: uuid.New(), UserID: &owner, PaymentID: &paymentID, Status: model.BookingStatusConfirmed},
		payment:    &model.PaymentModel{PaymentID: paymentID, Amount: decimal.RequireFromString("24.99"), Status: model.PaymentStatusPaid},
		paymentErr: want,
	}
	if _, err := NewBookingService(repo, 0).CancelBooking(context.Background(), repo.booking.BookingID, owner); !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
	if repo.saved || repo.booking.Status != model.BookingStatusConfirmed {
		t.Fatal("booking cancelled despite failed refund")
	}
}

func TestCancelBooking_RefundPersistedOnce(t *testing.T) {
	f := newPaymentFixture(t, 2, "12.49")
	id := f.reserve(t, 2)
	started := f.start(t, id)
	ctx := context.Background()
	if _, err := f.payments.SimulatePayment(ctx, id, f.userID, model.SimulatedPaymentSuccess, started.Amount); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.bookings.CancelBooking(ctx, id, f.userID); err != nil {
			t.Fatal(err)
		}
	}
	payment, err := repository.NewPaymentRepository(f.db).GetPayment(started.PaymentID)
	if err != nil {
		t.Fatal(err)
	}
	if payment.Status != model.PaymentStatusRefunded || !payment.RefundAmount.Equal(decimal.RequireFromString("24.98")) {
		t.Fatalf("unexpected payment: %+v", payment)
	}
	result, err := f.payments.GetPayment(ctx, id, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if result.BookingStatus != model.BookingStatusCancelled || result.TicketsValid || result.Status != model.PaymentStatusRefunded {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := f.bookings.ReserveTickets(ctx, f.eventID, f.userID, 2); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestCancelBooking_StatusAndOwnership(t *testing.T) {
	owner := uuid.New()
	for _, tc := range []struct {
		name   string
		status model.BookingStatus
		user   *uuid.UUID
		want   error
		save   bool
	}{
		{"reserved", model.BookingStatusReserved, &owner, nil, true},
		{"confirmed", model.BookingStatusConfirmed, &owner, nil, true},
		{"cancelled", model.BookingStatusCancelled, &owner, nil, false},
		{"failed", model.BookingStatusFailed, &owner, ErrBookingNotCancellable, false},
		{"expired", model.BookingStatusExpired, &owner, ErrBookingNotCancellable, false},
		{"other owner", model.BookingStatusConfirmed, func() *uuid.UUID { id := uuid.New(); return &id }(), ErrBookingNotFound, false},
		{"deleted owner", model.BookingStatusConfirmed, nil, ErrBookingNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &cancelBookingRepo{booking: &model.BookingModel{BookingID: uuid.New(), UserID: tc.user, Status: tc.status}}
			result, err := NewBookingService(repo, 0).CancelBooking(context.Background(), repo.booking.BookingID, owner)
			if !errors.Is(err, tc.want) || repo.saved != tc.save {
				t.Fatalf("err=%v saved=%v", err, repo.saved)
			}
			if err == nil && (result.Status != model.BookingStatusCancelled || result.ExpiresAt != nil) {
				t.Fatalf("unexpected booking: %+v", result)
			}
		})
	}
	repo := &cancelBookingRepo{}
	if _, err := NewBookingService(repo, 0).CancelBooking(context.Background(), uuid.New(), owner); !errors.Is(err, ErrBookingNotFound) {
		t.Fatalf("missing booking: %v", err)
	}
}

func TestCancelBooking_PendingPaymentAndCapacity(t *testing.T) {
	f := newPaymentFixture(t, 1, "10")
	id := f.reserve(t, 1)
	payment := f.start(t, id)
	ctx := context.Background()
	if _, err := f.bookings.CancelBooking(ctx, id, f.userID); err != nil {
		t.Fatal(err)
	}
	result, err := f.payments.GetPayment(ctx, id, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != model.PaymentStatusFailed || result.FailureReason == nil || *result.FailureReason != model.PaymentFailureBookingCancelled || result.TicketsValid {
		t.Fatalf("unexpected payment: %+v", result)
	}
	if _, err := f.payments.SimulatePayment(ctx, id, f.userID, model.SimulatedPaymentSuccess, payment.Amount); !errors.Is(err, ErrPaymentAlreadyProcessed) {
		t.Fatalf("payment after cancel: %v", err)
	}
	if _, err := f.bookings.ReserveTickets(ctx, f.eventID, f.userID, 1); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestCancelBooking_ConcurrentPayment(t *testing.T) {
	f := newPaymentFixture(t, 1, "10")
	id := f.reserve(t, 1)
	payment := f.start(t, id)
	ctx := context.Background()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var cancelErr, paymentErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; _, cancelErr = f.bookings.CancelBooking(ctx, id, f.userID) }()
	go func() {
		defer wg.Done()
		<-start
		_, paymentErr = f.payments.SimulatePayment(ctx, id, f.userID, model.SimulatedPaymentSuccess, payment.Amount)
	}()
	close(start)
	wg.Wait()
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	if paymentErr != nil && !errors.Is(paymentErr, ErrPaymentAlreadyProcessed) {
		t.Fatal(paymentErr)
	}
	result, err := f.payments.GetPayment(ctx, id, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if result.BookingStatus != model.BookingStatusCancelled || result.TicketsValid || (result.Status != model.PaymentStatusRefunded && result.Status != model.PaymentStatusFailed) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := f.bookings.ReserveTickets(ctx, f.eventID, f.userID, 1); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}
