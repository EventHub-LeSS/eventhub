package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// The tests in this file need no database: the real handler and service run against fake
// repositories, like the delete handler tests do.

// Minimal PNG and JPEG bodies; the service only checks the magic bytes of the declared type.
var (
	testPNG  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, []byte("rest of the image")...)
	testJPEG = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("rest of the image")...)
)

type imageEventRepo struct {
	repository.EventRepository
	event            *model.EventModel
	imageBase64      *string
	imageContentType *string
	updateCalls      int
}

func (r *imageEventRepo) LockEvent(uuid.UUID) (*model.EventModel, error) {
	return r.event, nil
}

func (r *imageEventRepo) GetEventByID(uuid.UUID) (*model.EventModel, error) {
	return r.event, nil
}

func (r *imageEventRepo) UpdateEvent(event *model.EventModel) error {
	r.imageBase64 = event.ImageBase64
	r.imageContentType = event.ImageContentType
	r.updateCalls++
	return nil
}

type imageOrgRepo struct {
	repository.OrganizationRepository
	org *model.OrganizationModel
}

func (r *imageOrgRepo) GetByID(uuid.UUID) (*model.OrganizationModel, error) {
	return r.org, nil
}

type imageAuditRepo struct {
	repository.AuditLogRepository
	entries []*model.AuditLogModel
}

func (r *imageAuditRepo) Append(entry *model.AuditLogModel) error {
	r.entries = append(r.entries, entry)
	return nil
}

type imageFixture struct {
	events *imageEventRepo
	orgs   *imageOrgRepo
	audit  *imageAuditRepo
}

func newImageFixture(event *model.EventModel, org *model.OrganizationModel) *imageFixture {
	if org == nil && event != nil && event.OrganizerID != nil {
		org = &model.OrganizationModel{OrganizationID: *event.OrganizerID, KeycloakOrgID: "kc-org-a", Alias: "kc-org-a"}
	}
	return &imageFixture{
		events: &imageEventRepo{event: event},
		orgs:   &imageOrgRepo{org: org},
		audit:  &imageAuditRepo{},
	}
}

func (f *imageFixture) transactor() repository.Transactor {
	return transactorOf(repository.Tx{Events: f.events, Organizations: f.orgs, Audit: f.audit})
}

func (f *imageFixture) router(principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	eventService := service.NewEventService(f.events, f.orgs, f.transactor())
	h := NewEventHandler(eventService)

	r := gin.New()
	setPrincipal := func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}
	r.PUT("/api/v1/events/:id/image", setPrincipal, middleware.Audit(audit.EventImageChanged), h.PutEventImageHandler)
	r.GET("/api/v1/events/:eventId/image", h.GetEventImageHandler)
	return r
}

// transactorOf runs the transaction function directly against the given Tx, like the other
// fake transactors of this package.
type passthroughTransactor struct {
	tx repository.Tx
}

func transactorOf(tx repository.Tx) repository.Transactor {
	return passthroughTransactor{tx: tx}
}

func (t passthroughTransactor) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	return fn(t.tx)
}

func putImage(t *testing.T, router http.Handler, eventID, contentType string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/events/"+eventID+"/image", bodyBytes(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func bodyBytes(body []byte) *bytes.Reader {
	return bytes.NewReader(body)
}

func getImage(router http.Handler, eventID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/"+eventID+"/image", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func imageTestEvent(status model.EventStatus) *model.EventModel {
	orgID := uuid.New()
	return &model.EventModel{
		EventID:     uuid.New(),
		Title:       "Test Event",
		Status:      status,
		OrganizerID: &orgID,
	}
}

func TestPutEventImageStoresBase64(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.events.updateCalls != 1 {
		t.Fatalf("update calls = %d, want 1", f.events.updateCalls)
	}
	if f.events.imageContentType == nil || *f.events.imageContentType != model.ImageContentTypePNG {
		t.Errorf("stored content type = %v, want image/png", f.events.imageContentType)
	}
	if f.events.imageBase64 == nil {
		t.Fatal("no image stored")
	}
	decoded, err := base64.StdEncoding.DecodeString(*f.events.imageBase64)
	if err != nil {
		t.Fatalf("stored base64: %v", err)
	}
	if string(decoded) != string(testPNG) {
		t.Errorf("decoded image differs from the uploaded bytes")
	}

	if len(f.audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(f.audit.entries))
	}
	entry := f.audit.entries[0]
	if entry.Action != string(audit.EventImageChanged) || entry.ResourceID != event.EventID.String() {
		t.Errorf("unexpected audit entry: %+v", entry)
	}
}

func TestPutEventImageAcceptsJPEG(t *testing.T) {
	event := imageTestEvent(model.EventStatusDraft)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypeJPEG, testJPEG)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.events.imageContentType == nil || *f.events.imageContentType != model.ImageContentTypeJPEG {
		t.Errorf("stored content type = %v, want image/jpeg", f.events.imageContentType)
	}
}

func TestPutEventImageRefusesUnsupportedType(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	for _, contentType := range []string{"text/plain", "image/gif", ""} {
		rec := putImage(t, router, event.EventID.String(), contentType, testPNG)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("Content-Type %q: expected 415, got %d", contentType, rec.Code)
		}
	}
	if f.events.updateCalls != 0 {
		t.Errorf("stored an image for an unsupported content type")
	}
}

func TestPutEventImageIgnoresContentTypeParameters(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypePNG+"; charset=utf-8", testPNG)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.events.imageContentType == nil || *f.events.imageContentType != model.ImageContentTypePNG {
		t.Errorf("stored content type = %v, want image/png", f.events.imageContentType)
	}
}

func TestPutEventImageRefusesMislabeledBytes(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	// Declared as PNG, but the bytes are a JPEG.
	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypePNG, testJPEG)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d: %s", rec.Code, rec.Body.String())
	}
	if f.events.updateCalls != 0 {
		t.Errorf("stored mislabeled bytes")
	}
}

func TestPutEventImageRefusesEmptyBody(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypePNG, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutEventImageRefusesOversizedBody(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	tooBig := make([]byte, model.MaxEventImageBytes+1)
	copy(tooBig, testPNG)
	rec := putImage(t, router, event.EventID.String(), model.ImageContentTypePNG, tooBig)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d: %s", rec.Code, rec.Body.String())
	}

	exact := make([]byte, model.MaxEventImageBytes)
	copy(exact, testPNG)
	rec = putImage(t, router, event.EventID.String(), model.ImageContentTypePNG, exact)
	if rec.Code != http.StatusOK {
		t.Fatalf("image of exactly the limit: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutEventImageRequiresAuthenticationAndOwnership(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)

	// No principal at all.
	f := newImageFixture(event, nil)
	rec := putImage(t, f.router(nil), event.EventID.String(), model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("no principal: expected 401, got %d", rec.Code)
	}

	// Event manager of another organization.
	f = newImageFixture(event, &model.OrganizationModel{OrganizationID: *event.OrganizerID, KeycloakOrgID: "kc-org-b", Alias: "kc-org-b"})
	rec = putImage(t, f.router(principalManaging("kc-org-a")), event.EventID.String(), model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusForbidden {
		t.Errorf("other organization: expected 403, got %d", rec.Code)
	}

	// Unknown event.
	f = newImageFixture(nil, nil)
	rec = putImage(t, f.router(principalManaging("kc-org-a")), uuid.NewString(), model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown event: expected 404, got %d", rec.Code)
	}

	// Cancelled and completed events are no longer editable.
	for _, status := range []model.EventStatus{model.EventStatusCancelled, model.EventStatusCompleted} {
		event := imageTestEvent(status)
		f = newImageFixture(event, nil)
		rec = putImage(t, f.router(principalManaging("kc-org-a")), event.EventID.String(), model.ImageContentTypePNG, testPNG)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status %s: expected 400, got %d", status, rec.Code)
		}
	}
}

func TestPutEventImageInvalidEventID(t *testing.T) {
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	router := f.router(principalManaging("kc-org-a"))

	rec := putImage(t, router, "not-a-uuid", model.ImageContentTypePNG, testPNG)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetEventImageServesOriginalBytes(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(testJPEG)
	event := imageTestEvent(model.EventStatusPublished)
	event.ImageBase64 = &encoded
	event.ImageContentType = ptrString(model.ImageContentTypeJPEG)
	f := newImageFixture(event, nil)
	router := f.router(nil)

	rec := getImage(router, event.EventID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != model.ImageContentTypeJPEG {
		t.Errorf("Content-Type = %q, want %q", got, model.ImageContentTypeJPEG)
	}
	if rec.Body.String() != string(testJPEG) {
		t.Errorf("served bytes differ from the stored image")
	}
}

func TestGetEventImageNotFoundCases(t *testing.T) {
	// No image stored.
	event := imageTestEvent(model.EventStatusPublished)
	f := newImageFixture(event, nil)
	if rec := getImage(f.router(nil), event.EventID.String()); rec.Code != http.StatusNotFound {
		t.Errorf("no image: expected 404, got %d", rec.Code)
	}

	// Draft events are as invisible as unknown ones, like the public details route.
	draft := imageTestEvent(model.EventStatusDraft)
	draft.ImageBase64 = ptrString(base64.StdEncoding.EncodeToString(testPNG))
	draft.ImageContentType = ptrString(model.ImageContentTypePNG)
	f = newImageFixture(draft, nil)
	if rec := getImage(f.router(nil), draft.EventID.String()); rec.Code != http.StatusNotFound {
		t.Errorf("draft: expected 404, got %d", rec.Code)
	}

	// Unknown event.
	f = newImageFixture(nil, nil)
	if rec := getImage(f.router(nil), uuid.NewString()); rec.Code != http.StatusNotFound {
		t.Errorf("unknown event: expected 404, got %d", rec.Code)
	}

	// Malformed event ID.
	f = newImageFixture(event, nil)
	if rec := getImage(f.router(nil), "not-a-uuid"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad UUID: expected 400, got %d", rec.Code)
	}
}

func ptrString(s string) *string {
	return &s
}
