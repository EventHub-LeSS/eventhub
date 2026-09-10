package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

func TestCurrentUserReturnsTrustedPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	principal := &middleware.Principal{
		Subject:     "user-123",
		Username:    "alice",
		GlobalRoles: map[middleware.GlobalRole]struct{}{middleware.RoleVisitor: {}},
		ActiveOrganization: &middleware.OrganizationAccess{
			ID:    "org-123",
			Alias: "acme",
			Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleEventManager: {}},
		},
		Organizations: []*middleware.OrganizationAccess{
			{
				ID:    "org-123",
				Alias: "acme",
				Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleEventManager: {}},
			},
			{
				ID:    "org-456",
				Alias: "beta",
				Roles: map[middleware.OrganizationRole]struct{}{middleware.RoleOrganizationAdmin: {}},
			},
		},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		middleware.SetPrincipalForRequest(c, principal)
		c.Next()
	})
	router.GET("/users/me", CurrentUser)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/me", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body CurrentUserResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Subject != "user-123" || body.Organization == nil || body.Organization.ID != "org-123" {
		t.Fatalf("unexpected response: %#v", body)
	}
	if body.Username != "alice" || len(body.GlobalRoles) != 1 || body.GlobalRoles[0] != middleware.RoleVisitor {
		t.Fatalf("unexpected user roles: %#v", body)
	}
	if body.Organization.Alias != "acme" || len(body.Organization.Roles) != 1 || body.Organization.Roles[0] != middleware.RoleEventManager {
		t.Fatalf("unexpected organization roles: %#v", body.Organization)
	}
	if len(body.Organizations) != 2 {
		t.Fatalf("len(organizations) = %d, want 2", len(body.Organizations))
	}
	if body.Organizations[0].ID != "org-123" || body.Organizations[0].Alias != "acme" ||
		len(body.Organizations[0].Roles) != 1 || body.Organizations[0].Roles[0] != middleware.RoleEventManager {
		t.Fatalf("unexpected organizations[0]: %#v", body.Organizations[0])
	}
	if body.Organizations[1].ID != "org-456" || body.Organizations[1].Alias != "beta" ||
		len(body.Organizations[1].Roles) != 1 || body.Organizations[1].Roles[0] != middleware.RoleOrganizationAdmin {
		t.Fatalf("unexpected organizations[1]: %#v", body.Organizations[1])
	}
}

func TestCurrentUserRejectsMissingPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/users/me", CurrentUser)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/me", nil))

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}
