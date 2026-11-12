package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/henning-kln/gocloak"
)

func newOrgMembersRouter(t *testing.T, principal *middleware.Principal) http.Handler {
	t.Helper()
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)
	h := NewOrganizationHandler(kc, service.NewOrganizationService(kc, nil, nil), nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	r.GET("/api/v1/organizations/:organizationID/members", h.ListMembers)
	return r
}

func getOrgMembers(t *testing.T, router http.Handler, orgRef string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+orgRef+"/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// EVENTHUB-307: an organization admin who is no global admin sees the members, including
// themselves, with their roles.
func TestListMembers_OrgAdminSeesMembersWithRoles(t *testing.T) {
	router := newOrgMembersRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin))

	rec := getOrgMembers(t, router, "org-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var members []model.OrganizationMemberResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &members); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rights := map[string][]string{}
	for _, m := range members {
		rights[m.Username] = m.Rights
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v, want alice and bob", members)
	}
	if !slices.Equal(rights["alice"], []string{"org_admin"}) {
		t.Errorf("alice rights = %v, want [org_admin]", rights["alice"])
	}
	if !slices.Equal(rights["bob"], []string{"event_manager"}) {
		t.Errorf("bob rights = %v, want [event_manager]", rights["bob"])
	}
}

func TestListMembers_ByAlias(t *testing.T) {
	principal := orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin)
	principal.Organizations[0].Alias = "alias-1"
	router := newOrgMembersRouter(t, principal)

	if rec := getOrgMembers(t, router, "alias-1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListMembers_GlobalAdmin(t *testing.T) {
	router := newOrgMembersRouter(t, globalAdminPrincipal())

	if rec := getOrgMembers(t, router, "org-1"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := getOrgMembers(t, router, "unknown"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown org status = %d, want 404", rec.Code)
	}
}

func TestListMembers_NonAdminForbidden(t *testing.T) {
	router := newOrgMembersRouter(t, orgRolesPrincipal("org-1", middleware.RoleEventManager))

	if rec := getOrgMembers(t, router, "org-1"); rec.Code != http.StatusForbidden {
		t.Fatalf("event_manager status = %d, want 403", rec.Code)
	}
	if rec := getOrgMembers(t, router, "unknown"); rec.Code != http.StatusForbidden {
		t.Fatalf("unknown org status = %d, want 403", rec.Code)
	}
}

func TestListMembers_Unauthenticated(t *testing.T) {
	router := newOrgMembersRouter(t, nil)

	if rec := getOrgMembers(t, router, "org-1"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

// EVENTHUB-307: the admin of a newly created organization appears in its member list.
func TestListMembers_NewOrganizationListsItsAdmin(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)
	name, alias := "New Org", "new-org"
	orgID, _, err := kc.CreateOrganization(t.Context(), "svc-token", gocloak.OrganizationRepresentation{Name: &name, Alias: &alias}, "carol")
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	members, err := kc.ListOrganizationMembersWithRoles(t.Context(), orgID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].Username != "carol" || !slices.Equal(members[0].Rights, []string{"org_admin"}) {
		t.Fatalf("members = %+v, want carol as org_admin", members)
	}
}
