package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/audit"
	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeOrgResolver map[string]service.OrganizationRef

func (f fakeOrgResolver) ResolveOrganization(_ context.Context, ref string) (service.OrganizationRef, error) {
	if org, ok := f[ref]; ok {
		return org, nil
	}
	return service.OrganizationRef{}, service.ErrOrganizationNotFound
}

func newAuditLogRouter(principal *middleware.Principal, log *keycloakmock.AuditLog) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewAuditLogHandler(fakeOrgResolver{
		"org-1":   {ID: "org-1", Alias: "alias-1"},
		"alias-1": {ID: "org-1", Alias: "alias-1"},
		"org-2":   {ID: "org-2", Alias: "alias-2"},
	}, log)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	r.GET("/api/v1/organizations/:organizationID/audit-logs", h.ListAuditLogs)
	return r
}

func seedAuditLog(t *testing.T, log *keycloakmock.AuditLog, orgID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		err := log.Append(&model.AuditLogModel{
			OperationID:    uuid.New(),
			ActorSubject:   "sub-" + orgID,
			ActorUsername:  "user@" + orgID,
			OrganizationID: orgID,
			Action:         fmt.Sprintf("%s.e%d", orgID, i),
			ResourceType:   audit.ResourceEvent,
			ResourceID:     uuid.NewString(),
			Phase:          audit.PhaseCompleted,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func getAuditLogs(router http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/organizations/"+path, nil))
	return rec
}

func decodePage(t *testing.T, rec *httptest.ResponseRecorder) model.AuditLogPage {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	var page model.AuditLogPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v, body: %s", err, rec.Body.String())
	}
	return page
}

func TestListAuditLogs_OrgAdminSeesOnlyOwnOrganization(t *testing.T) {
	log := &keycloakmock.AuditLog{}
	seedAuditLog(t, log, "org-1", 3)
	seedAuditLog(t, log, "org-2", 2)
	router := newAuditLogRouter(orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), log)

	for _, ref := range []string{"org-1", "alias-1"} {
		page := decodePage(t, getAuditLogs(router, ref+"/audit-logs"))
		if len(page.Items) != 3 || page.NextCursor != nil {
			t.Fatalf("%s: got %d items, cursor %v", ref, len(page.Items), page.NextCursor)
		}
		for _, item := range page.Items {
			if item.OrganizationID != "org-1" {
				t.Errorf("entry of foreign organization leaked: %+v", item)
			}
		}
		if page.Items[0].Action != "org-1.e2" {
			t.Errorf("newest entry first expected, got %s", page.Items[0].Action)
		}
	}
}

func TestListAuditLogs_Authorization(t *testing.T) {
	log := &keycloakmock.AuditLog{}
	seedAuditLog(t, log, "org-1", 1)
	seedAuditLog(t, log, "org-2", 1)

	tests := []struct {
		name      string
		principal *middleware.Principal
		org       string
		want      int
	}{
		{"org admin of the organization", orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), "org-1", http.StatusOK},
		{"event manager", orgRolesPrincipal("org-1", middleware.RoleEventManager), "org-1", http.StatusForbidden},
		{"finance viewer", orgRolesPrincipal("org-1", middleware.RoleFinanceViewer), "org-1", http.StatusForbidden},
		{"org admin of another organization", orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), "org-2", http.StatusForbidden},
		{"unknown organization looks like a foreign one", orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), "org-x", http.StatusForbidden},
		{"visitor without organization", &middleware.Principal{Subject: "v"}, "org-1", http.StatusForbidden},
		{"global admin", globalAdminPrincipal(), "org-2", http.StatusOK},
		{"global admin, unknown organization", globalAdminPrincipal(), "org-x", http.StatusNotFound},
		{"unauthenticated", nil, "org-1", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getAuditLogs(newAuditLogRouter(tt.principal, log), tt.org+"/audit-logs")
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestListAuditLogs_Pagination(t *testing.T) {
	log := &keycloakmock.AuditLog{}
	seedAuditLog(t, log, "org-1", 5)
	router := newAuditLogRouter(orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), log)

	var actions []string
	path := "org-1/audit-logs?limit=2"
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("pagination does not terminate")
		}
		page := decodePage(t, getAuditLogs(router, path))
		for _, item := range page.Items {
			actions = append(actions, item.Action)
		}
		if page.NextCursor == nil {
			break
		}
		path = "org-1/audit-logs?limit=2&cursor=" + *page.NextCursor
	}
	want := []string{"org-1.e4", "org-1.e3", "org-1.e2", "org-1.e1", "org-1.e0"}
	if fmt.Sprint(actions) != fmt.Sprint(want) {
		t.Errorf("actions = %v, want %v", actions, want)
	}
}

func TestListAuditLogs_DefaultsAndEmptyHistory(t *testing.T) {
	log := &keycloakmock.AuditLog{}
	seedAuditLog(t, log, "org-1", 60)
	router := newAuditLogRouter(orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), log)

	page := decodePage(t, getAuditLogs(router, "org-1/audit-logs"))
	if len(page.Items) != 50 || page.NextCursor == nil {
		t.Errorf("default page: %d items, cursor %v; want 50 and a cursor", len(page.Items), page.NextCursor)
	}
	if page := decodePage(t, getAuditLogs(router, "org-1/audit-logs?limit=100")); len(page.Items) != 60 {
		t.Errorf("limit 100 returned %d items", len(page.Items))
	}

	empty := newAuditLogRouter(globalAdminPrincipal(), &keycloakmock.AuditLog{})
	rec := getAuditLogs(empty, "org-2/audit-logs")
	decodePage(t, rec)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(rec.Body.Bytes(), &raw)
	if string(raw["items"]) != "[]" {
		t.Errorf("empty history must be an empty array, got %s", raw["items"])
	}
}

func TestListAuditLogs_RejectsInvalidParameters(t *testing.T) {
	log := &keycloakmock.AuditLog{}
	router := newAuditLogRouter(orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), log)
	for _, query := range []string{"limit=0", "limit=101", "limit=abc", "limit=-1", "cursor=garbage", "cursor=" + model.AuditLogCursor{}.Encode() + "x"} {
		t.Run(query, func(t *testing.T) {
			wantProblem(t, getAuditLogs(router, "org-1/audit-logs?"+query), http.StatusBadRequest)
		})
	}
}
