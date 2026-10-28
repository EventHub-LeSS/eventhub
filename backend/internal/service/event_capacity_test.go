package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// The capacity of an event must never drop below the tickets that are sold or reserved for it,
// otherwise completing a payment confirms more tickets than the event has (review of PR #63).

type capacityFixture struct {
	paymentFixture
	events     *EventService
	orgID      string
	categoryID uuid.UUID
	location   string
}

func newCapacityFixture(t *testing.T, capacity int) capacityFixture {
	t.Helper()
	f := capacityFixture{paymentFixture: newPaymentFixture(t, capacity, "20")}
	var row struct {
		KeycloakOrgID string
		CategoryID    uuid.UUID
		Location      string
	}
	err := f.db.Raw(`SELECT o.keycloak_org_id, e.category_id, e.location
	                 FROM events e JOIN organizations o ON o.organization_id = e.organizer_id
	                 WHERE e.event_id = ?`, f.eventID).Scan(&row).Error
	if err != nil {
		t.Fatalf("load event: %v", err)
	}
	f.orgID, f.categoryID, f.location = row.KeycloakOrgID, row.CategoryID, row.Location
	f.events = NewEventService(repository.NewEventRepository(f.db), repository.NewOrganizationRepository(f.db), repository.NewTransactor(f.db))
	return f
}

// setCapacity updates the event as its organizer, changing nothing but the capacity.
func (f capacityFixture) setCapacity(capacity int) error {
	_, err := f.events.UpdateEvent(auditCtx(audit.EventUpdated), f.eventID, []string{f.orgID}, model.UpdateEventRequest{
		Title:      "Konzert",
		StartTime:  time.Now().Add(24 * time.Hour),
		EndTime:    time.Now().Add(48 * time.Hour),
		Capacity:   capacity,
		Price:      decimal.NewFromInt(20),
		CategoryID: f.categoryID,
		Location:   f.location,
	})
	return err
}

func (f capacityFixture) capacity(t *testing.T) int {
	t.Helper()
	var capacity int
	if err := f.db.Raw("SELECT capacity FROM events WHERE event_id = ?", f.eventID).Scan(&capacity).Error; err != nil {
		t.Fatalf("load capacity: %v", err)
	}
	return capacity
}

func (f capacityFixture) occupied(t *testing.T) int64 {
	t.Helper()
	var occupied int64
	err := f.db.Raw(`SELECT COALESCE(SUM(number_of_tickets), 0) FROM bookings
	                 WHERE event_id = ? AND (status = 'confirmed' OR (status = 'reserved' AND expires_at > NOW()))`, f.eventID).Scan(&occupied).Error
	if err != nil {
		t.Fatalf("count occupied: %v", err)
	}
	return occupied
}

// Reproduces the review finding: capacity 2, two tickets reserved and the payment started, then
// the organizer lowers the capacity to 1 and the payment succeeds.
func TestCheckout_CapacityCannotDropBelowReservedTickets(t *testing.T) {
	f := newCapacityFixture(t, 2)
	bookingID := f.reserve(t, 2)
	f.start(t, bookingID)

	err := f.setCapacity(1)
	var capErr *CapacityBelowOccupiedError
	if !errors.As(err, &capErr) || capErr.Occupied != 2 || capErr.Requested != 1 {
		t.Fatalf("expected CapacityBelowOccupiedError with occupied=2 requested=1, got %v", err)
	}
	if c := f.capacity(t); c != 2 {
		t.Fatalf("a rejected update must keep the capacity, got %d", c)
	}

	resp, err := f.payments.SimulatePayment(context.Background(), bookingID, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(40))
	if err != nil {
		t.Fatalf("simulate: %v", err)
	}
	if !resp.TicketsValid {
		t.Errorf("expected valid tickets, got %+v", resp)
	}
	if sold := confirmedTickets(t, f.db, f.eventID); sold > int64(f.capacity(t)) {
		t.Errorf("sold %d tickets for capacity %d", sold, f.capacity(t))
	}
}

func TestUpdateEvent_CapacityCountsSoldAndLiveReservedTickets(t *testing.T) {
	f := newCapacityFixture(t, 10)
	ctx := context.Background()

	// 3 sold and 2 reserved tickets occupy the event; expired and failed bookings do not.
	sold := f.reserve(t, 3)
	f.start(t, sold)
	if _, err := f.payments.SimulatePayment(ctx, sold, f.userID, model.SimulatedPaymentSuccess, decimal.NewFromInt(60)); err != nil {
		t.Fatalf("simulate success: %v", err)
	}
	f.reserve(t, 2)
	expired := f.reserve(t, 4)
	expireReservation(t, f.db, expired)
	failed := f.reserve(t, 1)
	f.start(t, failed)
	if _, err := f.payments.SimulatePayment(ctx, failed, f.userID, model.SimulatedPaymentFailure, decimal.NewFromInt(20)); err != nil {
		t.Fatalf("simulate failure: %v", err)
	}

	var capErr *CapacityBelowOccupiedError
	if err := f.setCapacity(4); !errors.As(err, &capErr) || capErr.Occupied != 5 {
		t.Fatalf("lowering to 4: expected CapacityBelowOccupiedError with occupied=5, got %v", err)
	}
	if c := f.capacity(t); c != 10 {
		t.Fatalf("a rejected update must keep the capacity, got %d", c)
	}
	if err := f.setCapacity(5); err != nil {
		t.Fatalf("lowering to exactly the occupied tickets must work, got %v", err)
	}
	if err := f.setCapacity(20); err != nil {
		t.Fatalf("raising the capacity must work, got %v", err)
	}
	if c := f.capacity(t); c != 20 {
		t.Errorf("expected capacity 20, got %d", c)
	}
}

// Updates that do not lower the capacity stay possible even if the event is already overbooked,
// e.g. by legacy data, so organizers can still edit title and time.
func TestUpdateEvent_KeepingTheCapacityNeedsNoRoom(t *testing.T) {
	f := newCapacityFixture(t, 5)
	f.reserve(t, 5)
	mustExec(t, f.db, "UPDATE events SET capacity = 3 WHERE event_id = ?", f.eventID)

	if err := f.setCapacity(3); err != nil {
		t.Errorf("keeping the capacity must work, got %v", err)
	}
}

// Lowering the capacity while visitors reserve must never leave more occupied tickets than seats.
func TestUpdateEvent_CapacityReductionRacesWithReservations(t *testing.T) {
	f := newCapacityFixture(t, 10)
	for i := 0; i < 4; i++ {
		f.reserve(t, 1)
	}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var capErr *CapacityExceededError
			if _, err := f.bookings.ReserveTickets(context.Background(), f.eventID, f.userID, 1); err != nil && !errors.As(err, &capErr) {
				t.Errorf("reserve: %v", err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var capErr *CapacityBelowOccupiedError
		if err := f.setCapacity(6); err != nil && !errors.As(err, &capErr) {
			t.Errorf("set capacity: %v", err)
		}
	}()
	wg.Wait()

	if occupied, capacity := f.occupied(t), f.capacity(t); occupied > int64(capacity) {
		t.Errorf("%d tickets occupied for capacity %d", occupied, capacity)
	}
}
