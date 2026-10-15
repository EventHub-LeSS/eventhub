package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type dashboardEventRepo struct {
	repository.EventRepository

	events       []*model.EventModel
	ticketCounts map[uuid.UUID]int64
	ticketErr    error
	listOrgIDs   []uuid.UUID
}

func (f *dashboardEventRepo) ListByOrganizers(
	organizationIDs []uuid.UUID,
	status model.EventStatus,
) ([]*model.EventModel, error) {
	f.listOrgIDs = organizationIDs

	events := make([]*model.EventModel, 0)
	for _, event := range f.events {
		if event.OrganizerID == nil {
			continue
		}
		if status != "" && event.Status != status {
			continue
		}

		for _, orgID := range organizationIDs {
			if *event.OrganizerID == orgID {
				events = append(events, event)
				break
			}
		}
	}
	return events, nil
}

func (f *dashboardEventRepo) GetConfirmedTicketCount(
	eventID uuid.UUID,
) (int64, error) {
	if f.ticketErr != nil {
		return 0, f.ticketErr
	}
	return f.ticketCounts[eventID], nil
}

type dashboardOrgRepo struct {
	repository.OrganizationRepository

	orgs []*model.OrganizationModel
}

func (f *dashboardOrgRepo) ListByKeycloakOrgIDsOrAliases(
	refs []string,
) ([]*model.OrganizationModel, error) {
	orgs := make([]*model.OrganizationModel, 0)
	for _, org := range f.orgs {
		for _, ref := range refs {
			if ref == org.KeycloakOrgID || ref == org.Alias {
				orgs = append(orgs, org)
				break
			}
		}
	}
	return orgs, nil
}

func newDashboardUnitRouter(
	principal *middleware.Principal,
	eventRepo *dashboardEventRepo,
	orgRepo *dashboardOrgRepo,
) http.Handler {
	gin.SetMode(gin.TestMode)

	eventService := service.NewEventService(eventRepo, orgRepo, nil)
	h := NewEventHandler(eventService)

	r := gin.New()
	r.GET(
		"/api/v1/events/self/dashboard",
		func(c *gin.Context) {
			if principal != nil {
				middleware.SetPrincipalForRequest(c, principal)
			}
			c.Next()
		},
		h.GetSalesDashboardHandler,
	)
	return r
}

func TestGetSalesDashboardHandlerUnit_ReturnsOwnSalesFigures(t *testing.T) {
	ownOrgID := uuid.New()
	foreignOrgID := uuid.New()
	ownEventID := uuid.New()
	foreignEventID := uuid.New()

	eventRepo := &dashboardEventRepo{
		events: []*model.EventModel{
			{
				EventID:     ownEventID,
				Title:       "Eigenes Konzert",
				Capacity:    100,
				Status:      model.EventStatusPublished,
				OrganizerID: &ownOrgID,
			},
			{
				EventID:     foreignEventID,
				Title:       "Fremdes Konzert",
				Capacity:    50,
				Status:      model.EventStatusPublished,
				OrganizerID: &foreignOrgID,
			},
		},
		ticketCounts: map[uuid.UUID]int64{
			ownEventID:     30,
			foreignEventID: 20,
		},
	}
	orgRepo := &dashboardOrgRepo{
		orgs: []*model.OrganizationModel{
			{
				OrganizationID: ownOrgID,
				KeycloakOrgID:  "kc-own",
				Alias:          "own",
			},
			{
				OrganizationID: foreignOrgID,
				KeycloakOrgID:  "kc-foreign",
				Alias:          "foreign",
			},
		},
	}

	router := newDashboardUnitRouter(
		principalManaging("kc-own"),
		eventRepo,
		orgRepo,
	)
	rec := getSalesDashboard(router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var dashboard []model.SalesDashboardEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dashboard); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if len(dashboard) != 1 {
		t.Fatalf("expected 1 own event, got %s", rec.Body.String())
	}

	want := model.SalesDashboardEventResponse{
		EventID:        ownEventID,
		Title:          "Eigenes Konzert",
		Capacity:       100,
		SoldTickets:    30,
		AvailableSeats: 70,
	}
	if dashboard[0] != want {
		t.Errorf("got %+v, want %+v", dashboard[0], want)
	}

	if len(eventRepo.listOrgIDs) != 1 ||
		eventRepo.listOrgIDs[0] != ownOrgID {
		t.Errorf("unexpected organization filter: %v", eventRepo.listOrgIDs)
	}
}

func TestGetSalesDashboardHandlerUnit_ReturnsEmptyArray(t *testing.T) {
	orgRepo := &dashboardOrgRepo{
		orgs: []*model.OrganizationModel{
			{
				OrganizationID: uuid.New(),
				KeycloakOrgID:  "kc-own",
				Alias:          "own",
			},
		},
	}

	router := newDashboardUnitRouter(
		principalManaging("kc-own"),
		&dashboardEventRepo{},
		orgRepo,
	)
	rec := getSalesDashboard(router)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "[]" {
		t.Fatalf("expected [], got %s", rec.Body.String())
	}
}

func TestGetSalesDashboardHandlerUnit_StatusCodes(t *testing.T) {
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
			name:      "no organization role",
			principal: &middleware.Principal{Subject: "u"},
			want:      http.StatusForbidden,
		},
		{
			name: "organization member without event_manager role",
			principal: &middleware.Principal{
				Subject: "u",
				Organizations: []*middleware.OrganizationAccess{
					{
						ID:    "kc-own",
						Alias: "own",
						Roles: map[middleware.OrganizationRole]struct{}{},
					},
				},
			},
			want: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newDashboardUnitRouter(
				tt.principal,
				&dashboardEventRepo{},
				&dashboardOrgRepo{},
			)
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
				t.Fatalf("decode error response: %v", err)
			}
			if problem.Status != tt.want || problem.Type != "about:blank" {
				t.Errorf("unexpected error response: %s", rec.Body.String())
			}
		})
	}
}

func TestGetSalesDashboardHandlerUnit_ReturnsInternalServerError(t *testing.T) {
	orgID := uuid.New()

	eventRepo := &dashboardEventRepo{
		events: []*model.EventModel{
			{
				EventID:     uuid.New(),
				Title:       "Eigenes Konzert",
				Capacity:    100,
				Status:      model.EventStatusPublished,
				OrganizerID: &orgID,
			},
		},
		ticketErr: errors.New("internal database detail"),
	}
	orgRepo := &dashboardOrgRepo{
		orgs: []*model.OrganizationModel{
			{
				OrganizationID: orgID,
				KeycloakOrgID:  "kc-own",
				Alias:          "own",
			},
		},
	}

	router := newDashboardUnitRouter(
		principalManaging("kc-own"),
		eventRepo,
		orgRepo,
	)
	rec := getSalesDashboard(router)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}

	var problem model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if problem.Status != http.StatusInternalServerError ||
		problem.Type != "about:blank" ||
		problem.Detail != "internal error" {
		t.Errorf("unexpected error response: %s", rec.Body.String())
	}
}
