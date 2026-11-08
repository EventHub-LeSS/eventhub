package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/testdb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The tests in this file run the approval of organizations (EVENTHUB-293) against a real Postgres;
// see testdb.Open for how to enable them.

func deactivateOrganization(t *testing.T, db *gorm.DB, organizationID uuid.UUID) {
	t.Helper()
	if err := db.Exec("UPDATE organizations SET is_active = false WHERE organization_id = ?", organizationID).Error; err != nil {
		t.Fatal(err)
	}
}

// databaseOrganizationAdminRouter mirrors the admin organizations route group of main.go.
func databaseOrganizationAdminRouter(db *gorm.DB) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewOrganizationAdminHandler(repository.NewOrganizationRepository(db),
		service.NewOrganizationApprovalService(repository.NewTransactor(db)))
	r := gin.New()
	g := r.Group("/api/v1/admin/organizations")
	g.Use(func(c *gin.Context) {
		middleware.SetPrincipalForRequest(c, globalPrincipal(middleware.RoleAdmin))
		c.Next()
	}, middleware.RequireGlobalRole(middleware.RoleAdmin))
	g.GET("/pending", h.ListPendingOrganizations)
	g.PATCH("/:organizationID/approve", middleware.Audit(audit.OrganizationApproved), h.ApproveOrganization)
	return r
}

// An admin lists the pending organization and approves it; the approval is recorded against that admin.
func TestOrganizationApprovalDatabase(t *testing.T) {
	db := testdb.Open(t)
	pending := seedEventForOrg(t, db, "kc-pending-org", model.EventStatusPublished)
	deactivateOrganization(t, db, pending.ownerOrgID)
	seedEventForOrg(t, db, "kc-approved-org", model.EventStatusPublished) // already approved, never pending
	admin := databaseOrganizationAdminRouter(db)

	rec := httptest.NewRecorder()
	admin.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/organizations/pending", nil))
	var page PendingOrganizationListResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &page) != nil {
		t.Fatalf("list pending: %d %s", rec.Code, rec.Body.String())
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].OrganizationID != pending.ownerOrgID {
		t.Fatalf("pending organizations = %+v", page)
	}

	if rec := patchApprove(admin, pending.ownerOrgID.String()); rec.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var active bool
	if err := db.Raw("SELECT is_active FROM organizations WHERE organization_id = ?", pending.ownerOrgID).Scan(&active).Error; err != nil {
		t.Fatal(err)
	}
	if !active {
		t.Fatal("organization not active after the approval")
	}

	entries := loadAuditEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("expected one audit entry, got %+v", entries)
	}
	entry := entries[0]
	if entry.Action != string(audit.OrganizationApproved) || entry.OrganizationID != "kc-pending-org" ||
		entry.ResourceType != audit.ResourceOrganization || entry.ResourceID != pending.ownerOrgID.String() ||
		entry.ActorSubject != "moderator-1" || entry.Phase != audit.PhaseCompleted {
		t.Fatalf("unexpected audit entry: %+v", entry)
	}

	// Approving again changes and records nothing.
	if rec := patchApprove(admin, pending.ownerOrgID.String()); rec.Code != http.StatusConflict {
		t.Fatalf("second approval: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := patchApprove(admin, uuid.NewString()); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown organization: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if entries := loadAuditEntries(t, db); len(entries) != 1 {
		t.Fatalf("failed approvals were recorded: %+v", entries)
	}
}

// Without an audit entry the approval does not happen: the organization stays pending.
func TestOrganizationApprovalIsRolledBackWhenAuditCannotBeWrittenDatabase(t *testing.T) {
	db := testdb.Open(t)
	pending := seedEventForOrg(t, db, "kc-rollback-org", model.EventStatusPublished)
	deactivateOrganization(t, db, pending.ownerOrgID)
	for _, stmt := range []string{
		`CREATE FUNCTION fail_approval_audit_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit down'; END $$`,
		`CREATE TRIGGER fail_approval_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION fail_approval_audit_insert()`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER IF EXISTS fail_approval_audit ON audit_logs")
		db.Exec("DROP FUNCTION IF EXISTS fail_approval_audit_insert()")
	})

	rec := patchApprove(databaseOrganizationAdminRouter(db), pending.ownerOrgID.String())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var active bool
	if err := db.Raw("SELECT is_active FROM organizations WHERE organization_id = ?", pending.ownerOrgID).Scan(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active {
		t.Fatal("organization was approved without an audit entry")
	}
}
