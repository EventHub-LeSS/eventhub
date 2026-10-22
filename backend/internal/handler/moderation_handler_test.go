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
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type moderationRepoStub struct {
	target  *model.RatingModerationTarget
	lockErr error
	updated []bool
}

func (s *moderationRepoStub) LockRating(uuid.UUID) (*model.RatingModerationTarget, error) {
	return s.target, s.lockErr
}

func (s *moderationRepoStub) SetRatingVisibility(_ uuid.UUID, visible bool) error {
	s.updated = append(s.updated, visible)
	return nil
}

type moderationAuditStub struct {
	repository.AuditLogRepository
	entries []model.AuditLogModel
	err     error
}

func (s *moderationAuditStub) Append(entry *model.AuditLogModel) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, *entry)
	return nil
}

type moderationTxStub struct {
	ratings *moderationRepoStub
	audit   *moderationAuditStub
	calls   int
}

func (s *moderationTxStub) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	s.calls++
	return fn(repository.Tx{Moderation: s.ratings, Audit: s.audit})
}

func newModerationTxStub(target *model.RatingModerationTarget) *moderationTxStub {
	return &moderationTxStub{ratings: &moderationRepoStub{target: target}, audit: &moderationAuditStub{}}
}

func globalPrincipal(roles ...middleware.GlobalRole) *middleware.Principal {
	p := &middleware.Principal{Subject: "moderator-1", Username: "mod", GlobalRoles: map[middleware.GlobalRole]struct{}{}}
	for _, role := range roles {
		p.GlobalRoles[role] = struct{}{}
	}
	return p
}

// moderationRouter mirrors the moderation route group of main.go.
func moderationRouter(s *service.ModerationService, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewModerationHandler(s)
	r := gin.New()
	g := r.Group("/api/v1/moderation")
	g.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}, middleware.RequireGlobalRole(middleware.RoleAdmin, middleware.RoleModerator))
	g.PUT("/ratings/:ratingId/visibility", middleware.Audit(audit.RatingVisibilityChanged), h.SetRatingVisibilityHandler)
	return r
}

func putRatingVisibility(router http.Handler, ratingID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/api/v1/moderation/ratings/"+ratingID+"/visibility", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeRatingVisibility(t *testing.T, rec *httptest.ResponseRecorder) model.RatingVisibilityResponse {
	t.Helper()
	var resp model.RatingVisibilityResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil {
		t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
	}
	return resp
}

func TestSetRatingVisibilityAuthorization(t *testing.T) {
	ratingID := uuid.New()
	orgID := "org-kc"
	for _, tc := range []struct {
		name      string
		principal *middleware.Principal
		want      int
	}{
		{"unauthenticated", nil, http.StatusUnauthorized},
		{"visitor", globalPrincipal(middleware.RoleVisitor), http.StatusForbidden},
		{"organization manager", principalManaging("org-kc"), http.StatusForbidden},
		{"admin", globalPrincipal(middleware.RoleAdmin), http.StatusOK},
		{"moderator", globalPrincipal(middleware.RoleModerator), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newModerationTxStub(&model.RatingModerationTarget{RatingID: ratingID, IsVisible: true, KeycloakOrgID: &orgID})
			rec := putRatingVisibility(moderationRouter(service.NewModerationService(tx), tc.principal),
				ratingID.String(), `{"hidden":true,"reason":"Scam"}`)
			if rec.Code != tc.want {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if wantCalls := map[bool]int{true: 1, false: 0}[tc.want == http.StatusOK]; tx.calls != wantCalls {
				t.Fatalf("transactions=%d, want %d", tx.calls, wantCalls)
			}
		})
	}
}

func TestSetRatingVisibilityRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		ratingID string
		body     string
	}{
		{"invalid", `{"hidden":true,"reason":"Scam"}`},
		{uuid.NewString(), `{}`},
		{uuid.NewString(), `{"hidden":true}`},
		{uuid.NewString(), `{"reason":"Scam"}`},
		{uuid.NewString(), `{"hidden":"yes","reason":"Scam"}`},
		{uuid.NewString(), `{"hidden":true,"reason":"   "}`},
		{uuid.NewString(), `{"hidden":true,"reason":"` + strings.Repeat("ä", 1001) + `"}`},
		{uuid.NewString(), `not json`},
	} {
		tx := newModerationTxStub(nil)
		rec := putRatingVisibility(moderationRouter(service.NewModerationService(tx), globalPrincipal(middleware.RoleModerator)), tc.ratingID, tc.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("input %s/%.40s: status=%d body=%.200s", tc.ratingID, tc.body, rec.Code, rec.Body.String())
			continue
		}
		assertOrganizationEventsProblem(t, rec, http.StatusBadRequest)
		if tx.calls != 0 {
			t.Errorf("input %.40s reached the service", tc.body)
		}
	}
}

func TestSetRatingVisibilityRecordsChange(t *testing.T) {
	ratingID, eventID := uuid.New(), uuid.New()
	orgID := "org-kc"
	tx := newModerationTxStub(&model.RatingModerationTarget{RatingID: ratingID, IsVisible: true, EventID: &eventID, KeycloakOrgID: &orgID})
	router := moderationRouter(service.NewModerationService(tx), globalPrincipal(middleware.RoleModerator))
	reason := strings.Repeat("ä", 1000)

	resp := decodeRatingVisibility(t, putRatingVisibility(router, ratingID.String(), `{"hidden":true,"reason":"  `+reason+`  "}`))
	if resp.RatingID != ratingID || !resp.Hidden || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(tx.ratings.updated) != 1 || tx.ratings.updated[0] {
		t.Fatalf("expected one update to invisible, got %v", tx.ratings.updated)
	}
	if len(tx.audit.entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(tx.audit.entries))
	}
	entry := tx.audit.entries[0]
	if entry.Action != string(audit.RatingVisibilityChanged) || entry.ResourceType != audit.ResourceRating ||
		entry.ResourceID != ratingID.String() || entry.OrganizationID != orgID || entry.Phase != audit.PhaseCompleted ||
		entry.ActorSubject != "moderator-1" || entry.ActorUsername != "mod" || entry.OperationID == uuid.Nil {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
	if entry.Changes["reason"] != reason || entry.Changes["eventId"] != eventID.String() {
		t.Fatalf("unexpected audit changes: %+v", entry.Changes)
	}
	fields, _ := entry.Changes["fields"].(map[string]any)
	change, _ := fields["isVisible"].(map[string]any)
	if change["from"] != true || change["to"] != false {
		t.Fatalf("unexpected visibility change: %+v", entry.Changes)
	}
}

func TestSetRatingVisibilityOutcomes(t *testing.T) {
	privateErr := errors.New("private database error")
	ratingID := uuid.New()
	for _, tc := range []struct {
		name      string
		target    *model.RatingModerationTarget
		lockErr   error
		auditErr  error
		body      string
		want      int
		wantAudit string
	}{
		{name: "unknown rating", body: `{"hidden":true,"reason":"Scam"}`, want: http.StatusNotFound},
		{name: "lookup failure", lockErr: privateErr, body: `{"hidden":true,"reason":"Scam"}`, want: http.StatusInternalServerError},
		{name: "audit failure", target: &model.RatingModerationTarget{RatingID: ratingID, IsVisible: true}, auditErr: privateErr,
			body: `{"hidden":true,"reason":"Scam"}`, want: http.StatusInternalServerError},
		{name: "already hidden", target: &model.RatingModerationTarget{RatingID: ratingID}, body: `{"hidden":true,"reason":"Scam"}`, want: http.StatusOK},
		{name: "already visible", target: &model.RatingModerationTarget{RatingID: ratingID, IsVisible: true}, body: `{"hidden":false,"reason":"ok"}`, want: http.StatusOK},
		{name: "show detached rating", target: &model.RatingModerationTarget{RatingID: ratingID}, body: `{"hidden":false,"reason":"ok"}`,
			want: http.StatusOK, wantAudit: audit.ScopePlatform},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newModerationTxStub(tc.target)
			tx.ratings.lockErr, tx.audit.err = tc.lockErr, tc.auditErr
			rec := putRatingVisibility(moderationRouter(service.NewModerationService(tx), globalPrincipal(middleware.RoleAdmin)), ratingID.String(), tc.body)
			if strings.Contains(rec.Body.String(), privateErr.Error()) {
				t.Fatal("database error leaked")
			}
			if tc.want != http.StatusOK {
				assertOrganizationEventsProblem(t, rec, tc.want)
				return
			}
			resp := decodeRatingVisibility(t, rec)
			if resp.Changed != (tc.wantAudit != "") {
				t.Fatalf("unexpected response: %+v", resp)
			}
			if tc.wantAudit == "" {
				if len(tx.ratings.updated) != 0 || len(tx.audit.entries) != 0 {
					t.Fatalf("no-op changed or recorded something: %v %v", tx.ratings.updated, tx.audit.entries)
				}
				return
			}
			if len(tx.audit.entries) != 1 || tx.audit.entries[0].OrganizationID != tc.wantAudit || tx.audit.entries[0].Changes["eventId"] != nil {
				t.Fatalf("unexpected audit entries: %+v", tx.audit.entries)
			}
		})
	}
}

func TestSetRatingVisibilityRequiresAuditContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tx := newModerationTxStub(&model.RatingModerationTarget{IsVisible: true})
	r := gin.New()
	r.PUT("/api/v1/moderation/ratings/:ratingId/visibility", NewModerationHandler(service.NewModerationService(tx)).SetRatingVisibilityHandler)
	rec := putRatingVisibility(r, uuid.NewString(), `{"hidden":true,"reason":"Scam"}`)
	assertOrganizationEventsProblem(t, rec, http.StatusInternalServerError)
	if tx.calls != 0 {
		t.Fatal("visibility changed without audit context")
	}
}
