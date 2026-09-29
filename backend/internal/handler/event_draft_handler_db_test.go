package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/testdb"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EVENTHUB-77: Veranstaltung als Entwurf speichern

func postDraft(t *testing.T, router http.Handler, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events/draft", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func getOwnEvents(router http.Handler, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/self"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// draftBody references the category and location of s, which therefore exist.
func draftBody(s seededEvent) map[string]any {
	return map[string]any{
		"title":      "Sommerfest 2026",
		"startTime":  time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		"endTime":    time.Now().Add(52 * time.Hour).UTC().Format(time.RFC3339),
		"capacity":   100,
		"price":      15,
		"categoryId": s.categoryID.String(),
		"locationId": s.locationID.String(),
	}
}

func with(body map[string]any, key string, value any) map[string]any {
	copied := make(map[string]any, len(body)+1)
	for k, v := range body {
		copied[k] = v
	}
	copied[key] = value
	return copied
}

func eventCount(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&model.EventModel{}).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	return count
}

func decodeEvents(t *testing.T, rec *httptest.ResponseRecorder) []model.EventModel {
	t.Helper()
	var events []model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode events: %v; body: %s", err, rec.Body.String())
	}
	return events
}

// AK1: Eine Veranstaltung kann im Status Entwurf gespeichert werden.
func TestSaveEventAsDraftHandler_CreatesDraft(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := postDraft(t, router, draftBody(seeded))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Status != model.EventStatusDraft {
		t.Errorf("response status = %s, want draft", created.Status)
	}

	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", created.EventID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Status != model.EventStatusDraft {
		t.Errorf("stored status = %s, want draft", stored.Status)
	}
	if stored.OrganizerID == nil || *stored.OrganizerID != seeded.ownerOrgID {
		t.Errorf("organizer = %v, want %v", stored.OrganizerID, seeded.ownerOrgID)
	}
}

func TestSaveEventAsDraftHandler_PicksOrganization(t *testing.T) {
	db := testdb.Open(t)
	seedEventForOrg(t, db, "kc-org-a", model.EventStatusPublished)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-a", "kc-org-b"))

	rec := postDraft(t, router, with(draftBody(seeded), "organizationId", "kc-org-b"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.OrganizerID == nil || *created.OrganizerID != seeded.ownerOrgID {
		t.Errorf("organizer = %v, want %v", created.OrganizerID, seeded.ownerOrgID)
	}
}

func TestSaveEventAsDraftHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	seedEventForOrg(t, db, "kc-org-a", model.EventStatusPublished)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	before := eventCount(t, db)

	tests := []struct {
		name       string
		principal  *middleware.Principal
		body       map[string]any
		want       int
		wantDetail string
	}{
		{name: "no principal", body: draftBody(seeded), want: http.StatusUnauthorized},
		{name: "no event_manager role", principal: &middleware.Principal{Subject: "u"}, body: draftBody(seeded), want: http.StatusForbidden},
		{name: "invalid body", principal: principalManaging("kc-org-b"), body: map[string]any{"title": "x"}, want: http.StatusBadRequest},
		{name: "several organizations without organizationId", principal: principalManaging("kc-org-a", "kc-org-b"), body: draftBody(seeded), want: http.StatusBadRequest, wantDetail: "organizationId"},
		{name: "foreign organizationId", principal: principalManaging("kc-org-a"), body: with(draftBody(seeded), "organizationId", "kc-org-b"), want: http.StatusForbidden},
		{name: "unknown category", principal: principalManaging("kc-org-b"), body: with(draftBody(seeded), "categoryId", uuid.NewString()), want: http.StatusUnprocessableEntity, wantDetail: "categoryId"},
		{name: "unknown location", principal: principalManaging("kc-org-b"), body: with(draftBody(seeded), "locationId", uuid.NewString()), want: http.StatusUnprocessableEntity, wantDetail: "locationId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postDraft(t, newEventRouter(db, tt.principal), tt.body)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
			var problem model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Status != tt.want || problem.Type != "about:blank" {
				t.Errorf("expected RFC 9457 body, got %s", rec.Body.String())
			}
			if !strings.Contains(problem.Detail, tt.wantDetail) {
				t.Errorf("detail %q does not mention %q", problem.Detail, tt.wantDetail)
			}
		})
	}

	if after := eventCount(t, db); after != before {
		t.Errorf("rejected requests created %d events", after-before)
	}
}

// AK2 + AK3: Der Veranstalter sieht eigene Entwürfe, andere Organisationen nicht.
func TestListOwnEventsHandler_ListsOwnDrafts(t *testing.T) {
	db := testdb.Open(t)
	ownDraft := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	ownPublished := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	seedEventForOrg(t, db, "kc-org-a", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := getOwnEvents(router, "?status=draft")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	drafts := decodeEvents(t, rec)
	if len(drafts) != 1 || drafts[0].EventID != ownDraft.eventID {
		t.Fatalf("expected only the own draft %s, got %s", ownDraft.eventID, rec.Body.String())
	}

	rec = getOwnEvents(router, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	all := decodeEvents(t, rec)
	if len(all) != 2 {
		t.Fatalf("expected the 2 own events, got %s", rec.Body.String())
	}
	for _, event := range all {
		if event.EventID != ownDraft.eventID && event.EventID != ownPublished.eventID {
			t.Errorf("foreign event %s listed", event.EventID)
		}
	}
}

func TestListOwnEventsHandler_StatusCodes(t *testing.T) {
	db := testdb.Open(t)
	seedEventForOrg(t, db, "kc-org-a", model.EventStatusDraft)

	tests := []struct {
		name      string
		principal *middleware.Principal
		query     string
		want      int
		wantBody  string
	}{
		{name: "no principal", query: "?status=draft", want: http.StatusUnauthorized},
		{name: "visitor without organization role", principal: &middleware.Principal{Subject: "u"}, query: "?status=draft", want: http.StatusForbidden},
		{name: "invalid status", principal: principalManaging("kc-org-b"), query: "?status=deleted", want: http.StatusBadRequest},
		{name: "no own events", principal: principalManaging("kc-org-b"), query: "?status=draft", want: http.StatusOK, wantBody: "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getOwnEvents(newEventRouter(db, tt.principal), tt.query)
			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("expected body %s, got %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}

// AK4 + AK5: Ein Entwurf kann später bearbeitet und veröffentlicht werden.
func TestDraftLifecycle_EditAndPublish(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := postDraft(t, router, draftBody(seeded))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create draft: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var draft model.EventModel
	if err := json.Unmarshal(rec.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode: %v", err)
	}

	rec = putEvent(t, router, draft.EventID.String(), validBody(seeded))
	if rec.Code != http.StatusOK {
		t.Fatalf("edit draft: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var stored model.EventModel
	db.First(&stored, "event_id = ?", draft.EventID)
	if stored.Title != "Neues Konzert" || stored.Status != model.EventStatusDraft {
		t.Fatalf("edit not persisted or status changed: title=%q status=%s", stored.Title, stored.Status)
	}

	rec = postPublish(t, router, draft.EventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("publish draft: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	db.First(&stored, "event_id = ?", draft.EventID)
	if stored.Status != model.EventStatusPublished {
		t.Errorf("status after publish = %s, want published", stored.Status)
	}
	if drafts := decodeEvents(t, getOwnEvents(router, "?status=draft")); len(drafts) != 0 {
		t.Errorf("published event still listed as draft: %v", drafts)
	}
}

func TestUpdateEventHandler_UnknownCategory(t *testing.T) {
	db := testdb.Open(t)
	seeded := seedEventForOrg(t, db, "kc-org-b", model.EventStatusDraft)
	router := newEventRouter(db, principalManaging("kc-org-b"))

	rec := putEvent(t, router, seeded.eventID.String(), with(validBody(seeded), "categoryId", uuid.NewString()))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
}
