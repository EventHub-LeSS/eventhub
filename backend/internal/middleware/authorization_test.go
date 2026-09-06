package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireGlobalRole(t *testing.T) {
	tests := []struct {
		name       string
		principal  *Principal
		wantStatus int
	}{
		{name: "missing principal", wantStatus: http.StatusUnauthorized},
		{name: "required role", principal: principalWithRoles([]GlobalRole{RoleAdmin}, nil), wantStatus: http.StatusNoContent},
		{name: "wrong role", principal: principalWithRoles([]GlobalRole{RoleVisitor}, nil), wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, called := runAuthorizationMiddleware(RequireGlobalRole(RoleAdmin), tt.principal, "")
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if called != (tt.wantStatus == http.StatusNoContent) {
				t.Fatalf("handler called = %t", called)
			}
		})
	}
}

func TestRequireOrganizationRole(t *testing.T) {
	tests := []struct {
		name       string
		principal  *Principal
		orgID      string
		wantStatus int
	}{
		{name: "matching organization and role", principal: principalWithRoles(nil, []OrganizationRole{RoleEventManager}), orgID: "org-123", wantStatus: http.StatusNoContent},
		{name: "global admin bypass", principal: principalWithRoles([]GlobalRole{RoleAdmin}, nil), orgID: "another-org", wantStatus: http.StatusNoContent},
		{name: "different organization", principal: principalWithRoles(nil, []OrganizationRole{RoleEventManager}), orgID: "another-org", wantStatus: http.StatusForbidden},
		{name: "wrong organization role", principal: principalWithRoles(nil, []OrganizationRole{RoleFinanceViewer}), orgID: "org-123", wantStatus: http.StatusForbidden},
		{name: "missing organization", principal: principalWithRoles(nil, nil), orgID: "org-123", wantStatus: http.StatusForbidden},
		{name: "missing principal", orgID: "org-123", wantStatus: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, called := runAuthorizationMiddleware(
				RequireOrganizationRole("organizationID", RoleEventManager, RoleOrganizationAdmin),
				tt.principal,
				tt.orgID,
			)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if called != (tt.wantStatus == http.StatusNoContent) {
				t.Fatalf("handler called = %t", called)
			}
		})
	}
}

func TestRequireOrganizationRoleRejectsMissingRouteParameter(t *testing.T) {
	status, called := runAuthorizationMiddleware(
		RequireOrganizationRole("wrongParameterName", RoleEventManager),
		principalWithRoles(nil, []OrganizationRole{RoleEventManager}),
		"org-123",
	)
	if status != http.StatusForbidden || called {
		t.Fatalf("status = %d, handler called = %t", status, called)
	}
}

func principalWithRoles(global []GlobalRole, organization []OrganizationRole) *Principal {
	principal := &Principal{GlobalRoles: make(map[GlobalRole]struct{})}
	for _, role := range global {
		principal.GlobalRoles[role] = struct{}{}
	}
	if organization != nil {
		principal.ActiveOrganization = &OrganizationAccess{ID: "org-123", Roles: make(map[OrganizationRole]struct{})}
		for _, role := range organization {
			principal.ActiveOrganization.Roles[role] = struct{}{}
		}
	}
	return principal
}

func runAuthorizationMiddleware(handler gin.HandlerFunc, principal *Principal, organizationID string) (int, bool) {
	gin.SetMode(gin.TestMode)
	called := false
	router := gin.New()
	if principal != nil {
		router.Use(func(c *gin.Context) {
			SetPrincipalForRequest(c, principal)
			c.Next()
		})
	}
	router.GET("/organizations/:organizationID", handler, func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})

	if organizationID == "" {
		organizationID = "org-123"
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/organizations/"+organizationID, nil)
	router.ServeHTTP(response, request)
	return response.Code, called
}
