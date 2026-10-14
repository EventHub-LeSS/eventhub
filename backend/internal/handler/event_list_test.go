package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type listOrganizationRepo struct {
	repository.OrganizationRepository
	org       *model.OrganizationModel
	err       error
	requested uuid.UUID
	calls     int
	lookups   []string
}

func (r *listOrganizationRepo) GetByID(id uuid.UUID) (*model.OrganizationModel, error) {
	r.requested = id
	r.calls++
	return r.org, r.err
}

func (r *listOrganizationRepo) ListByKeycloakOrgIDsOrAliases(refs []string) ([]*model.OrganizationModel, error) {
	r.lookups = append(r.lookups, refs...)
	if r.err != nil || r.org == nil {
		return nil, r.err
	}
	if slices.Contains(refs, r.org.KeycloakOrgID) || slices.Contains(refs, r.org.Alias) {
		return []*model.OrganizationModel{r.org}, nil
	}
	return nil, nil
}

type listEventRepo struct {
	repository.EventRepository
	events    []*model.EventModel
	err       error
	requested uuid.UUID
	calls     int
}

func (r *listEventRepo) ListByOrganization(id uuid.UUID) ([]*model.EventModel, error) {
	r.requested = id
	r.calls++
	return r.events, r.err
}

func TestListOrganizationEventsHandler_Membership(t *testing.T) {
	orgID := uuid.New()
	org := &model.OrganizationModel{OrganizationID: orgID, KeycloakOrgID: "kc-org", Alias: "org-alias"}
	event := &model.EventModel{EventID: uuid.New(), OrganizerID: &orgID}
	tests := []struct {
		name       string
		id         string
		principal  *middleware.Principal
		org        *model.OrganizationModel
		orgErr     error
		eventErr   error
		empty      bool
		want       int
		orgCalls   int
		eventCalls int
	}{
		{name: "unauthenticated", id: orgID.String(), want: http.StatusUnauthorized},
		{name: "unknown alias", id: "invalid", principal: principalManaging("kc-org"), org: org, want: http.StatusNotFound},
		{name: "by alias", id: "org-alias", principal: principalManaging("kc-org"), org: org, want: http.StatusOK, orgCalls: 1, eventCalls: 1},
		{name: "by alias without membership", id: "org-alias", principal: principalManaging("other-org"), org: org, want: http.StatusNotFound, orgCalls: 1},
		{name: "alias lookup fails", id: "org-alias", principal: principalManaging("kc-org"), orgErr: errors.New("lookup failed"), want: http.StatusInternalServerError},
		{name: "no membership", id: orgID.String(), principal: &middleware.Principal{Subject: "user"}, org: org, want: http.StatusNotFound, orgCalls: 1},
		{name: "role in another org", id: orgID.String(), principal: principalManaging("other-org"), org: org, want: http.StatusNotFound, orgCalls: 1},
		{name: "unknown organization", id: orgID.String(), principal: principalManaging("kc-org"), want: http.StatusNotFound, orgCalls: 1},
		{name: "organization lookup fails", id: orgID.String(), principal: principalManaging("kc-org"), orgErr: errors.New("lookup failed"), want: http.StatusInternalServerError, orgCalls: 1},
		{name: "events lookup fails", id: orgID.String(), principal: principalManaging("kc-org"), org: org, eventErr: errors.New("query failed"), want: http.StatusInternalServerError, orgCalls: 1, eventCalls: 1},
		{name: "member without roles", id: orgID.String(), principal: &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{nil, {ID: "kc-org", Alias: "org-alias"}}}, org: org, want: http.StatusOK, orgCalls: 1, eventCalls: 1},
		{name: "finance member", id: orgID.String(), principal: &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{{ID: "kc-org", Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleFinanceViewer: {}}}}}, org: org, want: http.StatusOK, orgCalls: 1, eventCalls: 1},
		{name: "alias only token", id: orgID.String(), principal: &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{{ID: "org-alias", Alias: "org-alias"}}}, org: org, want: http.StatusOK, orgCalls: 1, eventCalls: 1},
		{name: "empty events", id: orgID.String(), principal: principalManaging("kc-org"), org: org, empty: true, want: http.StatusOK, orgCalls: 1, eventCalls: 1},
		{name: "empty alias does not grant membership", id: orgID.String(), principal: &middleware.Principal{Subject: "user", Organizations: []*middleware.OrganizationAccess{{ID: "foreign-org"}}}, org: &model.OrganizationModel{OrganizationID: orgID, KeycloakOrgID: "kc-org"}, want: http.StatusNotFound, orgCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgRepo := &listOrganizationRepo{org: tt.org, err: tt.orgErr}
			eventRepo := &listEventRepo{err: tt.eventErr}
			if !tt.empty {
				eventRepo.events = []*model.EventModel{event}
			}
			h := NewEventHandler(service.NewEventService(eventRepo, orgRepo, nil))
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.GET("/api/v1/events/org/:id", func(c *gin.Context) {
				if tt.principal != nil {
					middleware.SetPrincipalForRequest(c, tt.principal)
				}
				h.ListOrganizationEventsHandler(c)
			})
			rec := getOrganizationEvents(router, tt.id)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
			if orgRepo.calls != tt.orgCalls || eventRepo.calls != tt.eventCalls {
				t.Fatalf("repository calls: org=%d events=%d; expected %d/%d", orgRepo.calls, eventRepo.calls, tt.orgCalls, tt.eventCalls)
			}
			if orgRepo.calls > 0 && orgRepo.requested != orgID {
				t.Errorf("wrong organization requested: %s", orgRepo.requested)
			}
			if eventRepo.calls > 0 && eventRepo.requested != orgID {
				t.Errorf("wrong organization events requested: %s", eventRepo.requested)
			}
			if tt.want != http.StatusOK {
				assertOrganizationEventsProblem(t, rec, tt.want)
				return
			}
			if tt.empty {
				if rec.Body.String() != "[]" {
					t.Fatalf("expected [], got %s", rec.Body.String())
				}
				return
			}
			var events []model.EventModel
			if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].EventID != event.EventID {
				t.Fatalf("unexpected events: %s", rec.Body.String())
			}
		})
	}
}
