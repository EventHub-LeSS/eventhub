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

type approvalOrgRepoStub struct {
	repository.OrganizationRepository
	org         *model.OrganizationModel
	activateErr error
	activated   int
}

func (s *approvalOrgRepoStub) Activate(uuid.UUID) (bool, error) {
	if s.activateErr != nil {
		return false, s.activateErr
	}
	if s.org == nil || s.org.IsActive {
		return false, nil
	}
	s.org.IsActive = true
	s.activated++
	return true, nil
}

func (s *approvalOrgRepoStub) GetByID(uuid.UUID) (*model.OrganizationModel, error) {
	if s.org == nil {
		return nil, nil
	}
	org := *s.org
	return &org, nil
}

type approvalTxStub struct {
	orgs  *approvalOrgRepoStub
	audit *moderationAuditStub
	calls int
}

func (s *approvalTxStub) InTransaction(_ context.Context, fn func(repository.Tx) error) error {
	s.calls++
	return fn(repository.Tx{Organizations: s.orgs, Audit: s.audit})
}

func newApprovalTxStub(org *model.OrganizationModel) *approvalTxStub {
	return &approvalTxStub{orgs: &approvalOrgRepoStub{org: org}, audit: &moderationAuditStub{}}
}

// organizationAdminRouter mirrors the admin organizations route group of main.go.
func organizationAdminRouter(tx *approvalTxStub, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewOrganizationAdminHandler(tx.orgs, service.NewOrganizationApprovalService(tx))
	r := gin.New()
	g := r.Group("/api/v1/admin/organizations")
	g.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	}, middleware.RequireGlobalRole(middleware.RoleAdmin))
	g.PATCH("/:organizationID/approve", middleware.Audit(audit.OrganizationApproved), h.ApproveOrganization)
	return r
}

func patchApprove(router http.Handler, organizationID string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/organizations/"+organizationID+"/approve", nil))
	return rec
}

func TestApproveOrganizationRecordsAudit(t *testing.T) {
	orgID := uuid.New()
	tx := newApprovalTxStub(&model.OrganizationModel{OrganizationID: orgID, KeycloakOrgID: "kc-org"})

	rec := patchApprove(organizationAdminRouter(tx, globalPrincipal(middleware.RoleAdmin)), orgID.String())
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var org model.OrganizationModel
	if err := json.Unmarshal(rec.Body.Bytes(), &org); err != nil || org.OrganizationID != orgID || !org.IsActive {
		t.Fatalf("unexpected response %s (%v)", rec.Body.String(), err)
	}
	if len(tx.audit.entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(tx.audit.entries))
	}
	entry := tx.audit.entries[0]
	if entry.Action != string(audit.OrganizationApproved) || entry.ResourceType != audit.ResourceOrganization ||
		entry.ResourceID != orgID.String() || entry.OrganizationID != "kc-org" || entry.Phase != audit.PhaseCompleted ||
		entry.ActorSubject != "moderator-1" || entry.ActorUsername != "mod" || entry.OperationID == uuid.Nil {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}
	fields, _ := entry.Changes["fields"].(map[string]any)
	change, _ := fields["isActive"].(map[string]any)
	if change["from"] != false || change["to"] != true {
		t.Fatalf("unexpected audit changes: %+v", entry.Changes)
	}
}

func TestApproveOrganizationOutcomes(t *testing.T) {
	privateErr := errors.New("private database error")
	for _, tc := range []struct {
		name        string
		org         *model.OrganizationModel
		activateErr error
		auditErr    error
		principal   *middleware.Principal
		id          string
		want        int
	}{
		{name: "invalid id", org: &model.OrganizationModel{}, id: "nope", want: http.StatusBadRequest},
		{name: "unknown organization", want: http.StatusNotFound},
		{name: "already approved", org: &model.OrganizationModel{IsActive: true}, want: http.StatusConflict},
		{name: "database failure", org: &model.OrganizationModel{}, activateErr: privateErr, want: http.StatusInternalServerError},
		{name: "audit failure", org: &model.OrganizationModel{}, auditErr: privateErr, want: http.StatusInternalServerError},
		{name: "not an admin", org: &model.OrganizationModel{}, principal: globalPrincipal(middleware.RoleModerator), want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newApprovalTxStub(tc.org)
			tx.orgs.activateErr, tx.audit.err = tc.activateErr, tc.auditErr
			principal := tc.principal
			if principal == nil {
				principal = globalPrincipal(middleware.RoleAdmin)
			}
			id := tc.id
			if id == "" {
				id = uuid.NewString()
			}
			rec := patchApprove(organizationAdminRouter(tx, principal), id)
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), privateErr.Error()) {
				t.Fatal("database error leaked")
			}
			if len(tx.audit.entries) != 0 {
				t.Fatalf("failed approval recorded an audit entry: %+v", tx.audit.entries)
			}
		})
	}
}

func TestApproveOrganizationRequiresAuditContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tx := newApprovalTxStub(&model.OrganizationModel{})
	r := gin.New()
	r.PATCH("/api/v1/admin/organizations/:organizationID/approve",
		NewOrganizationAdminHandler(tx.orgs, service.NewOrganizationApprovalService(tx)).ApproveOrganization)
	rec := patchApprove(r, uuid.NewString())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if tx.calls != 0 || tx.orgs.activated != 0 {
		t.Fatal("organization approved without audit context")
	}
}

type pendingOrgRepoStub struct {
	repository.OrganizationRepository
	orgs  []*model.OrganizationModel
	total int64
	err   error
	page  [2]int
}

func (s *pendingOrgRepoStub) ListPending(page, limit int) ([]*model.OrganizationModel, int64, error) {
	s.page = [2]int{page, limit}
	return s.orgs, s.total, s.err
}

func listPending(repo *pendingOrgRepoStub, principal *middleware.Principal, query string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	h := NewOrganizationAdminHandler(repo, nil)
	r := gin.New()
	r.GET("/api/v1/admin/organizations/pending", func(c *gin.Context) {
		middleware.SetPrincipalForRequest(c, principal)
		c.Next()
	}, middleware.RequireGlobalRole(middleware.RoleAdmin), h.ListPendingOrganizations)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/pending"+query, nil))
	return rec
}

func TestListPendingOrganizationsPages(t *testing.T) {
	org := &model.OrganizationModel{OrganizationID: uuid.New(), Name: "Pending"}
	repo := &pendingOrgRepoStub{orgs: []*model.OrganizationModel{org}, total: 7}

	rec := listPending(repo, globalPrincipal(middleware.RoleAdmin), "?page=2&limit=3")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp PendingOrganizationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if repo.page != [2]int{2, 3} || resp.Page != 2 || resp.Limit != 3 || resp.Total != 7 ||
		len(resp.Items) != 1 || resp.Items[0].OrganizationID != org.OrganizationID {
		t.Fatalf("unexpected page %+v (repository asked for %v)", resp, repo.page)
	}
}

func TestListPendingOrganizationsDefaultsAndEmptyList(t *testing.T) {
	repo := &pendingOrgRepoStub{}
	rec := listPending(repo, globalPrincipal(middleware.RoleAdmin), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// An empty result is an empty array, not null, so the frontend can render it directly.
	if !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("expected an empty items array, got %s", rec.Body.String())
	}
	if repo.page[0] != 1 || repo.page[1] < 1 {
		t.Fatalf("unexpected default paging %v", repo.page)
	}
}

func TestListPendingOrganizationsRejections(t *testing.T) {
	privateErr := errors.New("private database error")
	for _, tc := range []struct {
		name      string
		principal *middleware.Principal
		query     string
		err       error
		want      int
	}{
		{name: "page below one", principal: globalPrincipal(middleware.RoleAdmin), query: "?page=0", want: http.StatusBadRequest},
		{name: "limit too large", principal: globalPrincipal(middleware.RoleAdmin), query: "?limit=1000", want: http.StatusBadRequest},
		{name: "not a number", principal: globalPrincipal(middleware.RoleAdmin), query: "?page=abc", want: http.StatusBadRequest},
		{name: "moderator", principal: globalPrincipal(middleware.RoleModerator), want: http.StatusForbidden},
		{name: "database failure", principal: globalPrincipal(middleware.RoleAdmin), err: privateErr, want: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := listPending(&pendingOrgRepoStub{err: tc.err}, tc.principal, tc.query)
			if rec.Code != tc.want {
				t.Fatalf("expected %d, got %d: %s", tc.want, rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), privateErr.Error()) {
				t.Fatal("database error leaked")
			}
		})
	}
}
