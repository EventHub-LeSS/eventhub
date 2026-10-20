package service

import (
	"backend/internal/model"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestGetEventStatistics_Access(t *testing.T) {
	tests := []struct {
		name        string
		event       *model.EventModel
		managedOrgs []string
		globalAdmin bool
		wantErr     error
	}{
		{
			name:        "own organization",
			event:       eventOwnedBy(orgB, model.EventStatusPublished),
			managedOrgs: []string{"kc-org-b"},
		},
		{
			name:        "event belongs to another organization",
			event:       eventOwnedBy(orgB, model.EventStatusPublished),
			managedOrgs: []string{"kc-org-a"},
			wantErr:     ErrForbidden,
		},
		{
			name:    "no managed organizations",
			event:   eventOwnedBy(orgB, model.EventStatusPublished),
			wantErr: ErrForbidden,
		},
		{
			name:        "event without organizer",
			event:       eventOwnedBy(nil, model.EventStatusPublished),
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrForbidden,
		},
		{
			name:        "admin sees event of another organization",
			event:       eventOwnedBy(orgB, model.EventStatusPublished),
			globalAdmin: true,
		},
		{
			name:        "admin sees event without organizer",
			event:       eventOwnedBy(nil, model.EventStatusPublished),
			globalAdmin: true,
		},
		{
			name:        "unknown event",
			managedOrgs: []string{"kc-org-b"},
			wantErr:     ErrEventNotFound,
		},
		{
			name:        "admin requests unknown event",
			globalAdmin: true,
			wantErr:     ErrEventNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService(tt.event)

			statistics, err := svc.GetEventStatistics(uuid.New(), tt.managedOrgs, tt.globalAdmin)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && statistics == nil {
				t.Fatal("expected statistics, got nil")
			}
			if tt.wantErr != nil && statistics != nil {
				t.Fatal("rejected request must not return statistics")
			}
		})
	}
}

// EVENTHUB-242: utilization is based on confirmed tickets only and must match the
// remaining seats reported alongside it.
func TestGetEventStatistics_Utilization(t *testing.T) {
	tests := []struct {
		name          string
		capacity      int
		confirmed     int64
		wantPercent   float64
		wantAvailable int64
	}{
		{name: "partially sold", capacity: 200, confirmed: 150, wantPercent: 75, wantAvailable: 50},
		{name: "rounds down to two decimals", capacity: 3, confirmed: 1, wantPercent: 33.33, wantAvailable: 2},
		{name: "rounds up to two decimals", capacity: 3, confirmed: 2, wantPercent: 66.67, wantAvailable: 1},
		{name: "sold out", capacity: 100, confirmed: 100, wantPercent: 100, wantAvailable: 0},
		{name: "no bookings", capacity: 100, confirmed: 0, wantPercent: 0, wantAvailable: 100},
		{name: "zero capacity", capacity: 0, confirmed: 0, wantPercent: 0, wantAvailable: 0},
		{name: "more tickets than capacity is capped", capacity: 100, confirmed: 150, wantPercent: 100, wantAvailable: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := eventOwnedBy(orgB, model.EventStatusPublished)
			event.Capacity = tt.capacity
			svc, repo := newTestService(event)
			repo.confirmedTickets = tt.confirmed

			statistics, err := svc.GetEventStatistics(uuid.New(), []string{"kc-org-b"}, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if statistics.UtilizationPercentage != tt.wantPercent {
				t.Errorf("utilization: expected %v, got %v", tt.wantPercent, statistics.UtilizationPercentage)
			}
			if statistics.AvailableSeats != tt.wantAvailable {
				t.Errorf("available seats: expected %v, got %v", tt.wantAvailable, statistics.AvailableSeats)
			}
			if statistics.SoldTickets != tt.confirmed {
				t.Errorf("sold tickets: expected %v, got %v", tt.confirmed, statistics.SoldTickets)
			}
		})
	}
}
