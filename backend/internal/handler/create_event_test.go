package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type createEventRepo struct {
	repository.EventRepository
	created *model.EventModel
	err     error
	calls   int
}

func (r *createEventRepo) CreateEvent(event *model.EventModel) error {
	r.calls++
	r.created = event
	return r.err
}

func validCreateEventBody(orgID uuid.UUID) map[string]any {
	start := time.Now().UTC().Add(48 * time.Hour)
	return map[string]any{
		"title": "Summer concert", "description": "Live music",
		"startTime": start, "endTime": start.Add(2 * time.Hour),
		"capacity": 100, "price": "19.50", "organizerId": orgID,
		"categoryId": uuid.New(), "locationId": uuid.New(),
	}
}

func postCreateEvent(t *testing.T, principal *middleware.Principal, orgRepo *listOrganizationRepo, eventRepo *createEventRepo, body any) *httptest.ResponseRecorder {
	t.Helper()
	var data []byte
	if raw, ok := body.(string); ok {
		data = []byte(raw)
	} else {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewEventHandler(service.NewEventService(eventRepo, orgRepo, nil))
	router.POST("/api/v1/events", func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		h.CreateEventHandler(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateEventHandler_AuthorizationAndErrors(t *testing.T) {
	orgID := uuid.New()
	org := &model.OrganizationModel{OrganizationID: orgID, KeycloakOrgID: "kc-org", Alias: "org-alias"}
	tests := []struct {
		name                       string
		principal                  *middleware.Principal
		org                        *model.OrganizationModel
		orgErr, eventErr           error
		want, orgCalls, eventCalls int
	}{
		{name: "unauthenticated", want: http.StatusUnauthorized},
		{name: "no manager role", principal: &middleware.Principal{Subject: "user"}, want: http.StatusForbidden},
		{name: "manager of another organization", principal: principalManaging("other-org"), org: org, want: http.StatusForbidden, orgCalls: 1},
		{name: "unknown organization", principal: principalManaging("kc-org"), want: http.StatusNotFound, orgCalls: 1},
		{name: "organization lookup failure", principal: principalManaging("kc-org"), orgErr: errors.New("private database details"), want: http.StatusInternalServerError, orgCalls: 1},
		{name: "creation failure", principal: principalManaging("kc-org"), org: org, eventErr: errors.New("private database details"), want: http.StatusInternalServerError, orgCalls: 1, eventCalls: 1},
		{name: "unknown category or location", principal: principalManaging("kc-org"), org: org, eventErr: repository.ErrUnknownReference, want: http.StatusUnprocessableEntity, orgCalls: 1, eventCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orgRepo := &listOrganizationRepo{org: tt.org, err: tt.orgErr}
			eventRepo := &createEventRepo{err: tt.eventErr}
			rec := postCreateEvent(t, tt.principal, orgRepo, eventRepo, validCreateEventBody(orgID))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			if orgRepo.calls != tt.orgCalls || eventRepo.calls != tt.eventCalls {
				t.Errorf("repository calls = %d/%d, want %d/%d", orgRepo.calls, eventRepo.calls, tt.orgCalls, tt.eventCalls)
			}
			if orgRepo.calls > 0 && orgRepo.requested != orgID {
				t.Error("wrong organization requested")
			}
			if tt.want == http.StatusInternalServerError && strings.Contains(rec.Body.String(), "private database details") {
				t.Error("response exposes internal error")
			}
			if rec.Code == tt.want {
				assertOrganizationEventsProblem(t, rec, tt.want)
			}
		})
	}
}

func TestCreateEventHandler_Success(t *testing.T) {
	for _, ref := range []string{"kc-org", "org-alias"} {
		t.Run(ref, func(t *testing.T) {
			orgID := uuid.New()
			orgRepo := &listOrganizationRepo{org: &model.OrganizationModel{OrganizationID: orgID, KeycloakOrgID: "kc-org", Alias: "org-alias"}}
			eventRepo := &createEventRepo{}
			body := validCreateEventBody(orgID)
			before := time.Now()
			rec := postCreateEvent(t, principalManaging("other-org", ref), orgRepo, eventRepo, body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
			}
			id, err := uuid.Parse(rec.Body.String())
			if err != nil || id == uuid.Nil {
				t.Fatalf("invalid response UUID: %q", rec.Body.String())
			}
			event := eventRepo.created
			if eventRepo.calls != 1 || event == nil {
				t.Fatal("event was not created exactly once")
			}
			if event.EventID != id || event.Status != model.EventStatusDraft {
				t.Error("wrong event ID or status")
			}
			if event.Title != body["title"] || event.Description == nil || *event.Description != body["description"] || event.Capacity != 100 || event.Price.String() != "19.5" {
				t.Errorf("incorrect event fields: %+v", event)
			}
			if !event.StartTime.Equal(body["startTime"].(time.Time)) || !event.EndTime.Equal(body["endTime"].(time.Time)) {
				t.Error("incorrect event times")
			}
			if event.OrganizerID == nil || *event.OrganizerID != orgID || event.CategoryID == nil || *event.CategoryID != body["categoryId"].(uuid.UUID) || event.LocationID == nil || *event.LocationID != body["locationId"].(uuid.UUID) {
				t.Error("incorrect organization, category or location")
			}
			if event.CreatedAt.Before(before) || event.UpdatedAt.Before(before) || event.CreatedAt.After(time.Now()) || event.UpdatedAt.After(time.Now()) {
				t.Error("incorrect timestamps")
			}
		})
	}
}

func TestCreateEventHandler_InvalidRequest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing title", func(b map[string]any) { delete(b, "title") }},
		{"short title", func(b map[string]any) { b["title"] = "ab" }},
		{"long title", func(b map[string]any) { b["title"] = strings.Repeat("a", 201) }},
		{"long description", func(b map[string]any) { b["description"] = strings.Repeat("a", 5001) }},
		{"missing start", func(b map[string]any) { delete(b, "startTime") }},
		{"past start", func(b map[string]any) { b["startTime"] = time.Now().Add(-time.Hour) }},
		{"invalid time", func(b map[string]any) { b["startTime"] = "invalid" }},
		{"missing end", func(b map[string]any) { delete(b, "endTime") }},
		{"end before start", func(b map[string]any) { b["endTime"] = b["startTime"].(time.Time).Add(-time.Hour) }},
		{"equal times", func(b map[string]any) { b["endTime"] = b["startTime"] }},
		{"zero capacity", func(b map[string]any) { b["capacity"] = 0 }},
		{"negative capacity", func(b map[string]any) { b["capacity"] = -1 }},
		{"negative price", func(b map[string]any) { b["price"] = "-0.01" }},
		{"invalid price", func(b map[string]any) { b["price"] = "invalid" }},
		{"missing category", func(b map[string]any) { delete(b, "categoryId") }},
		{"missing location", func(b map[string]any) { delete(b, "locationId") }},
		{"missing organizer", func(b map[string]any) { delete(b, "organizerId") }},
		{"invalid organizer", func(b map[string]any) { b["organizerId"] = "invalid" }},
		{"zero organizer UUID", func(b map[string]any) { b["organizerId"] = uuid.Nil }},
		{"malformed JSON", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body any = validCreateEventBody(uuid.New())
			if tt.mutate != nil {
				tt.mutate(body.(map[string]any))
			} else {
				body = "{"
			}
			orgRepo, eventRepo := &listOrganizationRepo{}, &createEventRepo{}
			rec := postCreateEvent(t, principalManaging("kc-org"), orgRepo, eventRepo, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
			assertOrganizationEventsProblem(t, rec, http.StatusBadRequest)
			if orgRepo.calls != 0 || eventRepo.calls != 0 {
				t.Error("invalid request reached repositories")
			}
		})
	}
}
