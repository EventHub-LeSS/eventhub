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

func getSalesDashboard(
	router http.Handler,
	organizationRef string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/organizations/"+organizationRef+"/dashboard",
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
	seedEventForOrg(t, db, orgB, model.EventStatusDraft)
	seedEventForOrg(t, db, foreignOrg, model.EventStatusPublished)

	// The caller can view both A and B, but requests only A.
	router := newEventRouter(
		db,
		principalViewingFinance(orgA, orgB),
	)

	rec := getSalesDashboard(router, ownA.ownerOrgID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}

	var dashboard []model.SalesDashboardEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dashboard); err != nil {
		t.Fatalf("decode dashboard: %v; body: %s", err, rec.Body.String())
	}

	if len(dashboard) != 1 {
		t.Fatalf("expected only one event from organization A, got %s", rec.Body.String())
	}

	want := model.SalesDashboardEventResponse{
		EventID:        ownA.eventID,
		Title:          "Altes Konzert",
		Capacity:       100,
		SoldTickets:    0,
		AvailableSeats: 100,
	}

	if dashboard[0] != want {
		t.Errorf("got %+v, want %+v", dashboard[0], want)
	}
}

func TestGetSalesDashboardHandler_ReturnsEmptyArray(t *testing.T) {
	db := testdb.Open(t)

	foreignOrg := "dashboard-foreign-" + uuid.NewString()
	seedEventForOrg(t, db, foreignOrg, model.EventStatusPublished)

	ownOrg := "dashboard-empty-" + uuid.NewString()
	ownOrgID := uuid.New()

	err := db.Exec(
		`INSERT INTO organizations (organization_id, keycloak_org_id, name)
		 VALUES (?, ?, ?)`,
		ownOrgID,
		ownOrg,
		"Empty dashboard organization",
	).Error
	if err != nil {
		t.Fatalf("seed organization: %v", err)
	}

	router := newEventRouter(
		db,
		principalViewingFinance(ownOrg),
	)

	rec := getSalesDashboard(router, ownOrgID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf(
			"expected 200, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
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
			name:      "no finance_viewer role",
			principal: &middleware.Principal{Subject: "u"},
			want:      http.StatusForbidden,
		},
		{
			name: "organization member without finance_viewer role",
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
		{
			name:      "event_manager without finance_viewer role",
			principal: principalManaging("dashboard-member"),
			want:      http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newEventRouter(db, tt.principal)
			rec := getSalesDashboard(router, "dashboard-member")

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
				t.Fatalf(
					"decode error response: %v; body: %s",
					err,
					rec.Body.String(),
				)
			}

			if problem.Status != tt.want || problem.Type != "about:blank" {
				t.Errorf(
					"unexpected error response: %s",
					rec.Body.String(),
				)
			}
		})
	}
}

func TestGetSalesDashboardHandler_RejectsForeignOrganization(t *testing.T) {
	db := testdb.Open(t)

	ownOrg := "dashboard-own-" + uuid.NewString()
	foreignOrg := "dashboard-foreign-" + uuid.NewString()

	seedEventForOrg(t, db, ownOrg, model.EventStatusPublished)
	foreign := seedEventForOrg(t, db, foreignOrg, model.EventStatusPublished)

	router := newEventRouter(
		db,
		principalViewingFinance(ownOrg),
	)

	rec := getSalesDashboard(router, foreign.ownerOrgID.String())
	if rec.Code != http.StatusForbidden {
		t.Fatalf(
			"expected 403, got %d: %s",
			rec.Code,
			rec.Body.String(),
		)
	}
}
