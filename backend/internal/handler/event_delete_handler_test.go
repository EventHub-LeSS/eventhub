package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB-256: Veranstaltungsentwurf löschen. The tests in this file need no database: the real
// handler and service run against fake repositories.

type deleteEventRepo struct {
	repository.EventRepository
	event       *model.EventModel
	lockErr     error
	hasBookings bool
	deleted     []uuid.UUID
}

func (r *deleteEventRepo) LockEvent(uuid.UUID) (*model.EventModel, error) {
	return r.event, r.lockErr
}

func (r *deleteEventRepo) HasBookings(uuid.UUID) (bool, error) {
	return r.hasBookings, nil
}

func (r *deleteEventRepo) DeleteEvent(eventID uuid.UUID) (bool, error) {
	r.deleted = append(r.deleted, eventID)
	return true, nil
}

type deleteOrgRepo struct {
	repository.OrganizationRepository
	org *model.OrganizationModel
}

func (r *deleteOrgRepo) GetByID(uuid.UUID) (*model.OrganizationModel, error) {
	return r.org, nil
}

type deleteAuditRepo struct {
	repository.AuditLogRepository
	entries []*model.AuditLogModel
}

func (r *deleteAuditRepo) Append(entry *model.AuditLogModel) error {
	r.entries = append(r.entries, entry)
	return nil
}

type deleteTransactor struct {
	tx    repository.Tx
	calls int
}

func (t *deleteTransactor) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	t.calls++
	return fn(t.tx)
}

// principalWithRole is principalManaging with another organization role.
func principalWithRole(role middleware.OrganizationRole, keycloakOrgIDs ...string) *middleware.Principal {
	p := principalManaging(keycloakOrgIDs...)
	for _, org := range p.Organizations {
		org.Roles = map[middleware.OrganizationRole]struct{}{role: {}}
	}
	return p
}

func deleteEvent(router http.Handler, eventID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/events/"+eventID, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// wantProblemDetail checks a problem response and its detail, which the frontend shows to users.
func wantProblemDetail(t *testing.T, rec *httptest.ResponseRecorder, status int, detail string) {
	t.Helper()
	wantProblem(t, rec, status)
	var problem model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Detail != detail {
		t.Errorf("detail = %q, want %q", problem.Detail, detail)
	}
}

func wantEventDeleted(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response model.EventDeletedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Message != "event deleted" {
		t.Errorf("unexpected response body: %s", rec.Body.String())
	}
}

type deleteFixture struct {
	events *deleteEventRepo
	orgs   *deleteOrgRepo
	audit  *deleteAuditRepo
	tx     *deleteTransactor
	event  *model.EventModel
}

func newDeleteFixture(status model.EventStatus) *deleteFixture {
	org := &model.OrganizationModel{OrganizationID: uuid.New(), KeycloakOrgID: "kc-org-b", Alias: "alias-b"}
	event := &model.EventModel{EventID: uuid.New(), Title: "Altes Konzert", Status: status, OrganizerID: &org.OrganizationID}
	f := &deleteFixture{
		events: &deleteEventRepo{event: event},
		orgs:   &deleteOrgRepo{org: org},
		audit:  &deleteAuditRepo{},
		event:  event,
	}
	f.tx = &deleteTransactor{tx: repository.Tx{Events: f.events, Organizations: f.orgs, Audit: f.audit}}
	return f
}

func (f *deleteFixture) router(principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewEventHandler(service.NewEventService(f.events, f.orgs, f.tx))
	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}
	r := gin.New()
	r.DELETE("/api/v1/events/:id", setPrincipal, middleware.Audit(audit.EventDeleted), h.DeleteEventHandler)
	return r
}

// EVENTHUB-256 / AK3, T3: event_manager und org_admin der besitzenden Organisation dürfen löschen.
func TestDeleteEventHandler_Roles_OnlyManagersAndAdminsOfOwner(t *testing.T) {
	multiRole := principalWithRole(middleware.RoleOrganizationAdmin, "kc-org-b")
	multiRole.Organizations = append(multiRole.Organizations, principalManaging("kc-org-a").Organizations...)

	tests := []struct {
		name      string
		principal *middleware.Principal
		eventID   string
		want      int
		detail    string
		// serviceCalls says whether the request got past the handler's own checks.
		serviceCalls int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized, detail: "authentication is required"},
		{name: "invalid event id", principal: principalManaging("kc-org-b"), eventID: "not-a-uuid", want: http.StatusBadRequest, detail: "invalid event id"},
		{name: "visitor without organization", principal: &middleware.Principal{Subject: "visitor"}, want: http.StatusForbidden, detail: "missing organization role"},
		{name: "member without role", principal: &middleware.Principal{Subject: "u", Organizations: []*middleware.OrganizationAccess{{ID: "kc-org-b"}}}, want: http.StatusForbidden, detail: "missing organization role"},
		{name: "finance_viewer of the owner", principal: principalWithRole(middleware.RoleFinanceViewer, "kc-org-b"), want: http.StatusForbidden, detail: "missing organization role"},
		{name: "event_manager of another organization", principal: principalManaging("kc-org-a"), want: http.StatusForbidden, detail: "forbidden", serviceCalls: 1},
		{name: "org_admin of another organization", principal: principalWithRole(middleware.RoleOrganizationAdmin, "kc-org-a"), want: http.StatusForbidden, detail: "forbidden", serviceCalls: 1},
		{name: "event_manager of the owner", principal: principalManaging("kc-org-b"), want: http.StatusOK, serviceCalls: 1},
		{name: "event_manager of the owner, token with alias", principal: principalManaging("alias-b"), want: http.StatusOK, serviceCalls: 1},
		{name: "org_admin of the owner", principal: principalWithRole(middleware.RoleOrganizationAdmin, "kc-org-b"), want: http.StatusOK, serviceCalls: 1},
		{name: "org_admin of the owner, event_manager elsewhere", principal: multiRole, want: http.StatusOK, serviceCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeleteFixture(model.EventStatusDraft)
			eventID := tt.eventID
			if eventID == "" {
				eventID = f.event.EventID.String()
			}

			rec := deleteEvent(f.router(tt.principal), eventID)

			if tt.want == http.StatusOK {
				wantEventDeleted(t, rec)
				if len(f.events.deleted) != 1 || f.events.deleted[0] != f.event.EventID {
					t.Errorf("draft was not deleted: %v", f.events.deleted)
				}
			} else {
				wantProblemDetail(t, rec, tt.want, tt.detail)
				if len(f.events.deleted) != 0 {
					t.Errorf("rejected request deleted %v", f.events.deleted)
				}
			}
			if f.tx.calls != tt.serviceCalls {
				t.Errorf("service transactions = %d, want %d", f.tx.calls, tt.serviceCalls)
			}
		})
	}
}

// EVENTHUB-256 / AK2, T2: Fehler des Service werden auf Statuscodes und Texte abgebildet.
func TestDeleteEventHandler_ServiceErrors_MapToStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		status      model.EventStatus
		missing     bool
		hasBookings bool
		lockErr     error
		want        int
		detail      string
	}{
		{name: "unknown event", status: model.EventStatusDraft, missing: true, want: http.StatusNotFound, detail: "event not found"},
		{name: "published", status: model.EventStatusPublished, want: http.StatusBadRequest, detail: "only draft events can be deleted"},
		{name: "cancelled", status: model.EventStatusCancelled, want: http.StatusBadRequest, detail: "only draft events can be deleted"},
		{name: "completed", status: model.EventStatusCompleted, want: http.StatusBadRequest, detail: "only draft events can be deleted"},
		{name: "draft with bookings", status: model.EventStatusDraft, hasBookings: true, want: http.StatusConflict, detail: "events with bookings cannot be deleted"},
		{name: "database failure", status: model.EventStatusDraft, lockErr: errors.New("connection reset"), want: http.StatusInternalServerError, detail: "internal error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newDeleteFixture(tt.status)
			f.events.hasBookings = tt.hasBookings
			f.events.lockErr = tt.lockErr
			if tt.missing {
				f.events.event = nil
			}

			rec := deleteEvent(f.router(principalManaging("kc-org-b")), f.event.EventID.String())

			wantProblemDetail(t, rec, tt.want, tt.detail)
			if len(f.events.deleted) != 0 || len(f.audit.entries) != 0 {
				t.Errorf("rejected request ran: deleted=%v entries=%d", f.events.deleted, len(f.audit.entries))
			}
		})
	}
}
