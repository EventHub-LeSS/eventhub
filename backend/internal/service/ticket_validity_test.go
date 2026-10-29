package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"testing"
	"time"

	"github.com/google/uuid"
)

// EVENTHUB-236: the payment result and the ticket views of the visitor share hasValidTickets. Every
// combination of booking, payment and event status is checked against the two valid ones.
func TestHasValidTickets_AllStatusCombinations_OnlyPaidConfirmedHeldEventsValid(t *testing.T) {
	bookingStatuses := []model.BookingStatus{
		model.BookingStatusReserved, model.BookingStatusConfirmed, model.BookingStatusCancelled,
		model.BookingStatusFailed, model.BookingStatusExpired,
	}
	// nil stands for a booking without payment and for a booking whose event no longer exists.
	paymentStatuses := []*model.PaymentStatus{nil}
	for _, s := range []model.PaymentStatus{model.PaymentStatusPending, model.PaymentStatusPaid, model.PaymentStatusFailed, model.PaymentStatusRefunded} {
		paymentStatuses = append(paymentStatuses, &s)
	}
	eventStatuses := []*model.EventStatus{nil}
	for _, s := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		eventStatuses = append(eventStatuses, &s)
	}

	valid := map[string]bool{
		"confirmed/paid/published": true,
		"confirmed/paid/completed": true,
	}

	combinations, validCount := 0, 0
	for _, booking := range bookingStatuses {
		for _, payment := range paymentStatuses {
			for _, event := range eventStatuses {
				view := &repository.BookingPayment{
					Booking:     &model.BookingModel{BookingID: uuid.New(), Status: booking},
					EventStatus: event,
					Now:         time.Now(),
				}
				paymentName, eventName := "none", "missing"
				if payment != nil {
					view.Payment = &model.PaymentModel{PaymentID: uuid.New(), Status: *payment}
					paymentName = string(*payment)
				}
				if event != nil {
					eventName = string(*event)
				}
				key := string(booking) + "/" + paymentName + "/" + eventName
				got := hasValidTickets(view)
				if got != valid[key] {
					t.Errorf("%s: expected ticketsValid=%v, got %v", key, valid[key], got)
				}
				if got {
					validCount++
				}
				combinations++
			}
		}
	}
	if combinations != 125 || validCount != len(valid) {
		t.Errorf("expected 125 combinations with %d valid ones, got %d with %d valid", len(valid), combinations, validCount)
	}
}
