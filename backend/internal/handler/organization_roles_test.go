package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"

	"github.com/gin-gonic/gin"
)

// newOrgRolesRouter mirrors the route registration of cmd/api: only organization
// admins of the addressed organization (or global admins) reach the handler.
func newOrgRolesRouter(t *testing.T, principal *middleware.Principal) (http.Handler, *keycloakmock.Fake) {
	t.Helper()
	fake := keycloakmock.New()
	h := NewOrganizationHandler(fake.KeycloakService(t), nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	orgs := r.Group("/api/v1/organizations")
	orgs.Use(middleware.RequireOrganizationRole("organizationID", middleware.RoleOrganizationAdmin))
	{
		orgs.PUT("/:organizationID/members/:username/roles", h.ConfigureMemberRoles)
	}
	return r, fake
}

func orgRolesPrincipal(orgID string, role middleware.OrganizationRole) *middleware.Principal {
	return &middleware.Principal{
		Subject:     "u-caller",
		Username:    "caller",
		GlobalRoles: map[middleware.GlobalRole]struct{}{},
		Organizations: []*middleware.OrganizationAccess{{
			ID:    orgID,
			Alias: orgID,
			Roles: map[middleware.OrganizationRole]struct{}{role: {}},
		}},
	}
}

func globalAdminPrincipal() *middleware.Principal {
	return &middleware.Principal{
		Subject:     "u-caller",
		Username:    "caller",
		GlobalRoles: map[middleware.GlobalRole]struct{}{middleware.RoleAdmin: {}},
	}
}

func putOrgRolesBody(t *testing.T, router http.Handler, orgID, username, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/organizations/"+orgID+"/members/"+username+"/roles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func putOrgRoles(t *testing.T, router http.Handler, orgID, username string, roles []string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"roles": roles})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return putOrgRolesBody(t, router, orgID, username, string(payload))
}

func wantAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, status, rec.Body.String())
	}
	var apiErr model.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("body is not a model.APIError: %v, body: %s", err, rec.Body.String())
	}
	if apiErr.Error.Code != code {
		t.Errorf("error code = %q, want %q (body: %s)", apiErr.Error.Code, code, rec.Body.String())
	}
}

// wantProblem checks a problem+json (model.ErrorResponse) response produced by
// the handler. Middleware 401/403 stay model.APIError and are checked with wantAPIError.
func wantProblem(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, status, rec.Body.String())
	}
	var p model.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a problem+json ErrorResponse: %v, body: %s", err, rec.Body.String())
	}
	if p.Status != status {
		t.Errorf("problem status = %d, want %d (body: %s)", p.Status, status, rec.Body.String())
	}
	if p.Title != http.StatusText(status) {
		t.Errorf("problem title = %q, want %q (body: %s)", p.Title, http.StatusText(status), rec.Body.String())
	}
}

func TestConfigureMemberRoles_UpdatesRolesAsOrgAdmin(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"org_admin", "finance_viewer"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	var resp model.ConfigureOrgRolesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Username != "bob" || resp.OrganizationID != "org-1" {
		t.Errorf("response = %+v, want username bob and organizationId org-1", resp)
	}
	if got, want := strings.Join(resp.Roles, ","), "org_admin,finance_viewer"; got != want {
		t.Errorf("roles = %q, want %q", got, want)
	}
	if len(fake.Grants) == 0 || len(fake.Revokes) == 0 {
		t.Errorf("expected grants and revokes, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestConfigureMemberRoles_AcceptsOrganizationAliasAsPathParameter(t *testing.T) {
	// Tokens may identify organizations by alias, so the route parameter can be one.
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("alias-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "alias-1", "bob", []string{"event_manager", "finance_viewer"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	var resp model.ConfigureOrgRolesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.OrganizationID != "alias-1" {
		t.Errorf("organizationId = %q, want the alias echoed back", resp.OrganizationID)
	}
	if got, want := strings.Join(resp.Roles, ","), "event_manager,finance_viewer"; got != want {
		t.Errorf("roles = %q, want %q", got, want)
	}
	if len(fake.Grants) == 0 {
		t.Error("expected the alias to resolve to the organization groups")
	}
}

func TestConfigureMemberRoles_GlobalAdminBypassesOrgRoleCheck(t *testing.T) {
	router, _ := newOrgRolesRouter(t, globalAdminPrincipal())

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"event_manager"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestConfigureMemberRoles_ForbiddenWithoutOrgAdminRole(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleEventManager))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"event_manager"})
	wantAPIError(t, rec, http.StatusForbidden, "FORBIDDEN")
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("handler must not be reached, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestConfigureMemberRoles_ForbiddenForForeignOrganization(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-2", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"event_manager"})
	wantAPIError(t, rec, http.StatusForbidden, "FORBIDDEN")
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("handler must not be reached, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestConfigureMemberRoles_RequiresAuthentication(t *testing.T) {
	router, _ := newOrgRolesRouter(t, nil)

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"event_manager"})
	wantAPIError(t, rec, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestConfigureMemberRoles_RejectsUnknownRole(t *testing.T) {
	router, _ := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"superadmin"})
	wantProblem(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "allowed roles") {
		t.Errorf("body = %s, want the allowed roles listed in the message", rec.Body.String())
	}
}

func TestConfigureMemberRoles_RejectsDuplicateRoles(t *testing.T) {
	router, _ := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"org_admin", "org_admin"})
	wantProblem(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "duplicate role") {
		t.Errorf("body = %s, want a duplicate-role message", rec.Body.String())
	}
}

func TestConfigureMemberRoles_RejectsMalformedJSONWithSanitizedMessage(t *testing.T) {
	router, _ := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRolesBody(t, router, "org-1", "bob", `{"roles":`)
	wantProblem(t, rec, http.StatusBadRequest)
	if !strings.Contains(rec.Body.String(), "request body must be a JSON object with a roles array") {
		t.Errorf("body = %s, want the sanitized binding message", rec.Body.String())
	}
}

func TestConfigureMemberRoles_UserNotFound(t *testing.T) {
	router, _ := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "dave", []string{"event_manager"})
	wantProblem(t, rec, http.StatusNotFound)
}

func TestConfigureMemberRoles_ConflictWhenRemovingLastAdmin(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "alice", nil)
	wantProblem(t, rec, http.StatusConflict)
	if !strings.Contains(rec.Body.String(), "last organization admin") {
		t.Errorf("body = %s, want last-admin conflict detail", rec.Body.String())
	}
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("expected no group changes, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestConfigureMemberRoles_RoleGroupsMissingSurfaces(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-2", middleware.RoleOrganizationAdmin))
	fake.OrgGroups["org-2"] = map[string]string{"org_admin": "g2-admin"}
	fake.OrgMembers["org-2"] = map[string]bool{"u-bob": true}

	rec := putOrgRoles(t, router, "org-2", "bob", []string{"event_manager"})
	wantProblem(t, rec, http.StatusInternalServerError)
	if !strings.Contains(rec.Body.String(), "missing group") {
		t.Errorf("body = %s, want the missing group detail", rec.Body.String())
	}
}

func TestConfigureMemberRoles_OutageYields500Not404(t *testing.T) {
	// A global admin passes the route check so the request reaches the handler.
	router, fake := newOrgRolesRouter(t, globalAdminPrincipal())
	// org-x is unknown; the by-ID lookup 404s and the alias listing fails with 500.
	fake.FailAdminRequest = func(method, path string) bool {
		return method == http.MethodGet && path == "/admin/realms/eventhub/organizations"
	}

	rec := putOrgRoles(t, router, "org-x", "bob", []string{"event_manager"})
	wantProblem(t, rec, http.StatusInternalServerError)
}

func TestConfigureMemberRoles_FailsClosedWhenGrantFailsAfterRevoke(t *testing.T) {
	router, fake := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))
	fake.FailAdminRequest = func(method, path string) bool {
		return method == http.MethodPut && strings.HasSuffix(path, "/groups/g-admin/members/u-bob")
	}

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"org_admin", "finance_viewer"})
	wantProblem(t, rec, http.StatusInternalServerError)
	if !strings.Contains(rec.Body.String(), "safe to retry") {
		t.Errorf("body = %s, want a safe-to-retry hint", rec.Body.String())
	}
	// Fail closed: bob ends up without roles instead of keeping the old role
	// plus the newly granted ones.
	for _, group := range []string{"g-admin", "g-manager", "g-finance"} {
		if fake.GroupMembers[group]["u-bob"] {
			t.Errorf("bob must not be in group %q after the failed update", group)
		}
	}
}

func TestConfigureMemberRoles_WritesAuditLog(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	router, _ := newOrgRolesRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := putOrgRoles(t, router, "org-1", "bob", []string{"org_admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}

	logged := buf.String()
	for _, want := range []string{
		"org_member_roles_changed",
		"actor_username=caller",
		"target_username=bob",
		"organization_id=org-1",
		"granted=org_admin",
	} {
		if !strings.Contains(logged, want) {
			t.Errorf("audit log missing %q, got: %s", want, logged)
		}
	}
}
