package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func getSalesDashboard(router http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/events/self/dashboard",
		nil,
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetSalesDashboardHandler_ListsOnlyOwnEvents(t *testing.T) {
	db := testdb.Open(t)

	orgA := "dashboard-a-" + uuid.NewString()
	orgB := "dashboard-b-" + uuid.NewString()
	foreignOrg := "dashboard-foreign-" + uuid.NewString()

	ownA := seedEventForOrg(t, db, orgA, model.EventStatusPublished)
	ownB := seedEventForOrg(t, db, orgB, model.EventStatusDraft)
	seedEventForOrg(t, db, foreignOrg, model.EventStatusPublished)

	router := newEventRouter(db, principalManaging(orgA, orgB))

	rec := getSalesDashboard(router)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var dashboard []model.SalesDashboardEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dashboard); err != nil {
		t.Fatalf("decode dashboard: %v; body: %s", err, rec.Body.String())
	}
	if len(dashboard) != 2 {
		t.Fatalf("expected 2 own events, got %s", rec.Body.String())
	}

	wantIDs := map[uuid.UUID]bool{
		ownA.eventID: true,
		ownB.eventID: true,
	}

	for _, event := range dashboard {
		if !wantIDs[event.EventID] {
			t.Fatalf("unexpected or duplicate event %s", event.EventID)
		}
		delete(wantIDs, event.EventID)

		if event.Title != "Altes Konzert" ||
			event.Capacity != 100 ||
			event.SoldTickets != 0 ||
			event.AvailableSeats != 100 {
			t.Errorf("unexpected dashboard figures: %+v", event)
		}
	}

	if len(wantIDs) != 0 {
		t.Errorf("missing own events: %v", wantIDs)
	}
}

func TestGetSalesDashboardHandler_ReturnsEmptyArray(t *testing.T) {
	db := testdb.Open(t)

	foreignOrg := "dashboard-foreign-" + uuid.NewString()
	seedEventForOrg(t, db, foreignOrg, model.EventStatusPublished)

	ownOrg := "dashboard-empty-" + uuid.NewString()
	router := newEventRouter(db, principalManaging(ownOrg))

	rec := getSalesDashboard(router)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "[]" {
		t.Fatalf("expected [], got %s", rec.Body.String())
	}
}

func TestGetSalesDashboardHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)

	tests := []struct {
		name      string
		principal *middleware.Principal
		want      int
	}{
		{
			name: "no principal",
			want: http.StatusUnauthorized,
		},
		{
			name:      "no event_manager role",
			principal: &middleware.Principal{Subject: "u"},
			want:      http.StatusForbidden,
		},
		{
			name: "organization member without event_manager role",
			principal: &middleware.Principal{
				Subject: "u",
				Organizations: []*middleware.OrganizationAccess{
					{
						ID:    "dashboard-member",
						Alias: "dashboard-member",
						Roles: map[middleware.OrganizationRole]struct{}{},
					},
				},
			},
			want: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newEventRouter(db, tt.principal)
			rec := getSalesDashboard(router)

			if rec.Code != tt.want {
				t.Fatalf(
					"expected %d, got %d: %s",
					tt.want,
					rec.Code,
					rec.Body.String(),
				)
			}

			var problem model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
				t.Fatalf("decode error response: %v; body: %s", err, rec.Body.String())
			}
			if problem.Status != tt.want || problem.Type != "about:blank" {
				t.Errorf("unexpected error response: %s", rec.Body.String())
			}
		})
	}
}
