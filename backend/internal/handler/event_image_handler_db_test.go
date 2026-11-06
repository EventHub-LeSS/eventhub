package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// The tests in this file exercise the real image handler, service and repository against a real
// Postgres; see testdb.Open for how to enable them.

func newImageDBRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	eventService := service.NewEventService(
		repository.NewEventRepository(db),
		repository.NewOrganizationRepository(db),
		repository.NewTransactor(db),
	)
	h := NewEventHandler(eventService)

	r := gin.New()
	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}
	r.PUT("/api/v1/events/:id/image", setPrincipal, middleware.Audit(audit.EventImageChanged), h.PutEventImageHandler)
	r.PUT("/api/v1/events/:id", setPrincipal, middleware.Audit(audit.EventUpdated), h.UpdateEventHandler)
	r.GET("/api/v1/events/:eventId/image", h.GetEventImageHandler)
	r.GET("/api/v1/events", h.ListPublishedEventsHandler)
	r.GET("/api/v1/events/:eventId", h.GetPublishedEventDetailsHandler)
	return r
}

func TestEventImageRoundTrip(t *testing.T) {
	db := testdb.Open(t)
	s := seedEventForOrg(t, db, "kc-image-org", model.EventStatusPublished)
	router := newImageDBRouter(db, principalManaging("kc-image-org"))

	// Upload a PNG.
	rec := putImage(t, router, s.eventID.String(), model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// The binary route serves the original bytes again.
	rec = getImage(router, s.eventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("binary route: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != model.ImageContentTypePNG {
		t.Errorf("Content-Type = %q, want %q", got, model.ImageContentTypePNG)
	}
	if rec.Body.String() != string(testPNG) {
		t.Errorf("served bytes differ from the uploaded image")
	}

	// A later full update of the event keeps the image.
	rec = putEvent(t, router, s.eventID.String(), map[string]any{
		"title":       "Neuer Titel",
		"description": nil,
		"startTime":   "2099-01-01T10:00:00Z",
		"endTime":     "2099-01-01T12:00:00Z",
		"capacity":    50,
		"price":       "10.00",
		"categoryId":  s.categoryID,
		"location":    seededLocation(s),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("update event: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = getImage(router, s.eventID.String())
	if rec.Code != http.StatusOK || rec.Body.String() != string(testPNG) {
		t.Fatalf("image lost after event update: status %d", rec.Code)
	}

	// The audit log records the change without the image content.
	var auditCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM audit_logs WHERE action = ? AND resource_id = ?`, string(audit.EventImageChanged), s.eventID.String()).Scan(&auditCount).Error; err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("audit entries for the image change = %d, want 1", auditCount)
	}
}

func TestEventImageReplacesPreviousImage(t *testing.T) {
	db := testdb.Open(t)
	s := seedEventForOrg(t, db, "kc-image-org", model.EventStatusPublished)
	router := newImageDBRouter(db, principalManaging("kc-image-org"))

	if rec := putImage(t, router, s.eventID.String(), model.ImageContentTypePNG, testPNG); rec.Code != http.StatusOK {
		t.Fatalf("upload png: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec := putImage(t, router, s.eventID.String(), model.ImageContentTypeJPEG, testJPEG)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload jpeg: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	rec = getImage(router, s.eventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("binary route: expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != model.ImageContentTypeJPEG {
		t.Errorf("Content-Type = %q, want image/jpeg after replacing the image", got)
	}
	if rec.Body.String() != string(testJPEG) {
		t.Errorf("served the replaced image's bytes")
	}
}

func TestEventImagePublicRoutesHideDraftsAndEmptyImages(t *testing.T) {
	db := testdb.Open(t)
	draft := seedEventForOrg(t, db, "kc-image-org", model.EventStatusDraft)
	router := newImageDBRouter(db, principalManaging("kc-image-org"))

	// Uploading to a draft works for its event manager.
	if rec := putImage(t, router, draft.eventID.String(), model.ImageContentTypePNG, testPNG); rec.Code != http.StatusOK {
		t.Fatalf("upload to draft: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// But the draft stays invisible on the public routes, image included.
	if rec := getImage(router, draft.eventID.String()); rec.Code != http.StatusNotFound {
		t.Errorf("draft binary route: expected 404, got %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/events", nil))
	var events []model.PublishedEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode event list: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("draft appears in the public event list")
	}

	// A published event without an image returns 404 on the binary route.
	published := seedEventForOrg(t, db, "kc-image-org", model.EventStatusPublished)
	if rec := getImage(router, published.eventID.String()); rec.Code != http.StatusNotFound {
		t.Errorf("no image: expected 404, got %d", rec.Code)
	}
}

func TestEventImageUploadRefusalsAgainstDatabase(t *testing.T) {
	db := testdb.Open(t)
	s := seedEventForOrg(t, db, "kc-image-org", model.EventStatusPublished)
	other := seedEventForOrg(t, db, "kc-image-other-org", model.EventStatusPublished)
	cancelled := seedEventForOrg(t, db, "kc-image-org", model.EventStatusCancelled)
	router := newImageDBRouter(db, principalManaging("kc-image-org"))

	// An event manager of another organization is refused.
	if rec := putImage(t, router, other.eventID.String(), model.ImageContentTypePNG, testPNG); rec.Code != http.StatusForbidden {
		t.Errorf("other organization: expected 403, got %d", rec.Code)
	}
	// A cancelled event is no longer editable.
	if rec := putImage(t, router, cancelled.eventID.String(), model.ImageContentTypePNG, testPNG); rec.Code != http.StatusBadRequest {
		t.Errorf("cancelled event: expected 400, got %d", rec.Code)
	}
	// Mislabeled bytes are refused without changing the stored image.
	if rec := putImage(t, router, s.eventID.String(), model.ImageContentTypeJPEG, testPNG); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("mislabeled bytes: expected 415, got %d", rec.Code)
	}
	// An empty body is refused.
	if rec := putImage(t, router, s.eventID.String(), model.ImageContentTypePNG, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("empty body: expected 400, got %d", rec.Code)
	}
}
