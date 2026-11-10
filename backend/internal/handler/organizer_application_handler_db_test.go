package handler

import (
	"backend/internal/audit"
	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const applicantEmail = "applicant@eventhub.test"

// applicationRouters mirrors the routes of main.go: the user routes behind authentication and the
// review routes behind the global admin role.
func applicationRouters(t *testing.T, db *gorm.DB, user, admin *middleware.Principal) (userRouter, adminRouter http.Handler, fake *keycloakmock.Fake) {
	t.Helper()
	fake = keycloakmock.New()
	fake.Users[applicantEmail] = "u-applicant"
	kc := fake.KeycloakService(t)
	orgRepo := repository.NewOrganizationRepository(db)
	userRepo := repository.NewUserRepository(db)
	orgService := service.NewOrganizationService(kc, orgRepo, userRepo)
	h := NewOrganizerApplicationHandler(service.NewOrganizerApplicationService(
		repository.NewTransactor(db), repository.NewOrganizerApplicationRepository(db), userRepo, orgRepo, orgService), userRepo)

	gin.SetMode(gin.TestMode)
	withPrincipal := func(p *middleware.Principal) gin.HandlerFunc {
		return func(c *gin.Context) {
			if p != nil {
				middleware.SetPrincipalForRequest(c, p)
			}
			c.Next()
		}
	}
	u := gin.New()
	u.Use(withPrincipal(user))
	u.POST("/api/v1/organizer-applications", h.SubmitApplicationHandler)
	u.GET("/api/v1/me/organizer-applications", h.ListOwnApplicationsHandler)

	a := gin.New()
	g := a.Group("/api/v1/admin/organizer-applications")
	g.Use(withPrincipal(admin), middleware.RequireGlobalRole(middleware.RoleAdmin))
	g.GET("", h.ListApplicationsHandler)
	g.GET("/:applicationId", h.GetApplicationHandler)
	g.POST("/:applicationId/approve", middleware.Audit(audit.OrganizerApplicationApproved), h.ApproveApplicationHandler)
	g.POST("/:applicationId/reject", middleware.Audit(audit.OrganizerApplicationRejected), h.RejectApplicationHandler)
	return u, a, fake
}

func seedApplicant(t *testing.T, db *gorm.DB) (uuid.UUID, *middleware.Principal) {
	t.Helper()
	userID := uuid.New()
	err := db.Exec("INSERT INTO users (user_id, keycloak_user_id, first_name, last_name, email) VALUES (?, ?, ?, ?, ?)",
		userID, "u-applicant", "Ada", "Antrag", applicantEmail).Error
	if err != nil {
		t.Fatalf("seed applicant: %v", err)
	}
	return userID, &middleware.Principal{Subject: "u-applicant", Username: applicantEmail, GlobalRoles: map[middleware.GlobalRole]struct{}{}}
}

func reviewer() *middleware.Principal {
	p := globalAdminPrincipal()
	p.AccessToken = "svc-token"
	return p
}

func sendJSON(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeApplication(t *testing.T, rec *httptest.ResponseRecorder, status int) model.OrganizerApplicationModel {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	var application model.OrganizerApplicationModel
	if err := json.Unmarshal(rec.Body.Bytes(), &application); err != nil {
		t.Fatalf("decode application: %v", err)
	}
	return application
}

const validApplication = `{
	"organizationName": " Stadtfest Bremen ",
	"organizationAlias": "Stadtfest-Bremen",
	"contactEmail": "info@stadtfest.test",
	"city": "Bremen",
	"countryCode": "de",
	"street": "  ",
	"message": "Wir veranstalten jedes Jahr das Stadtfest."
}`

func TestOrganizerApplicationApprovalDatabase(t *testing.T) {
	db := testdb.Open(t)
	applicantID, applicant := seedApplicant(t, db)
	users, admins, fake := applicationRouters(t, db, applicant, reviewer())

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	if submitted.Status != model.OrganizerApplicationPending || submitted.UserID != applicantID {
		t.Fatalf("submitted: %+v", submitted)
	}
	// The form is normalized: trimmed, alias lowercased, country uppercased, blanks dropped.
	if submitted.OrganizationName != "Stadtfest Bremen" || submitted.OrganizationAlias != "stadtfest-bremen" ||
		submitted.CountryCode == nil || *submitted.CountryCode != "DE" || submitted.Street != nil {
		t.Fatalf("normalized: %+v", submitted)
	}

	// Only one application may be pending.
	wantProblem(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusConflict)

	// Reviewers see the queue.
	var page model.OrganizerApplicationPage
	rec := sendJSON(admins, http.MethodGet, "/api/v1/admin/organizer-applications?status=pending", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || page.Total != 1 || page.Items[0].ApplicationID != submitted.ApplicationID {
		t.Fatalf("pending list: %d %s", rec.Code, rec.Body.String())
	}

	path := "/api/v1/admin/organizer-applications/" + submitted.ApplicationID.String()
	approved := decodeApplication(t, sendJSON(admins, http.MethodPost, path+"/approve", ""), http.StatusOK)
	if approved.Status != model.OrganizerApplicationApproved || approved.OrganizationID == nil || approved.DecidedAt == nil {
		t.Fatalf("approved: %+v", approved)
	}

	// The organization exists in Keycloak with the applicant as org_admin ...
	fake.Mu.Lock()
	keycloakOrgID := fake.OrgAliases["stadtfest-bremen"]
	adminGroup := fake.OrgGroups[keycloakOrgID]["org_admin"]
	isMember := fake.OrgMembers[keycloakOrgID]["u-applicant"]
	isAdmin := fake.GroupMembers[adminGroup]["u-applicant"]
	fake.Mu.Unlock()
	if keycloakOrgID == "" || !isMember || !isAdmin {
		t.Fatalf("keycloak org=%q member=%v admin=%v", keycloakOrgID, isMember, isAdmin)
	}
	// ... and in the API database with the contact details and the membership.
	var org model.OrganizationModel
	if err := db.First(&org, "organization_id = ?", *approved.OrganizationID).Error; err != nil {
		t.Fatal(err)
	}
	if org.KeycloakOrgID != keycloakOrgID || org.Name != "Stadtfest Bremen" || org.ContactEmail == nil || *org.ContactEmail != "info@stadtfest.test" {
		t.Fatalf("organization row: %+v", org)
	}
	var memberships int64
	db.Model(&model.OrganizationMembershipModel{}).Where("user_id = ? AND organization_id = ?", applicantID, org.OrganizationID).Count(&memberships)
	if memberships != 1 {
		t.Fatalf("memberships = %d", memberships)
	}

	// The decision is audited under the new organization and the applicant is notified.
	var entries []model.AuditLogModel
	db.Where("resource_type = ? AND resource_id = ?", audit.ResourceOrganizerApplication, submitted.ApplicationID.String()).Find(&entries)
	if len(entries) != 1 || entries[0].OrganizationID != keycloakOrgID || entries[0].Action != string(audit.OrganizerApplicationApproved) {
		t.Fatalf("audit entries: %+v", entries)
	}
	var notes []model.NotificationData
	db.Where("user_id = ? AND occasion = ?", applicantID, service.NotificationOccasionOrganizerApplication).Find(&notes)
	if len(notes) != 1 || !strings.Contains(notes[0].Subject, "angenommen") {
		t.Fatalf("notifications: %+v", notes)
	}

	// A decided application cannot be decided again.
	wantProblem(t, sendJSON(admins, http.MethodPost, path+"/approve", ""), http.StatusConflict)
	wantProblem(t, sendJSON(admins, http.MethodPost, path+"/reject", `{"reason":"zu spät"}`), http.StatusConflict)

	// The applicant sees the outcome; a new application for a taken alias is refused.
	var own []model.OrganizerApplicationModel
	rec = sendJSON(users, http.MethodGet, "/api/v1/me/organizer-applications", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &own); err != nil || len(own) != 1 || own[0].Status != model.OrganizerApplicationApproved {
		t.Fatalf("own applications: %d %s", rec.Code, rec.Body.String())
	}
	wantProblem(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusConflict)
}

func TestOrganizerApplicationRejectionDatabase(t *testing.T) {
	db := testdb.Open(t)
	applicantID, applicant := seedApplicant(t, db)
	users, admins, fake := applicationRouters(t, db, applicant, reviewer())

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	path := "/api/v1/admin/organizer-applications/" + submitted.ApplicationID.String()

	wantProblem(t, sendJSON(admins, http.MethodPost, path+"/reject", `{"reason":"   "}`), http.StatusBadRequest)
	rejected := decodeApplication(t, sendJSON(admins, http.MethodPost, path+"/reject", `{"reason":" Impressum fehlt "}`), http.StatusOK)
	if rejected.Status != model.OrganizerApplicationRejected || rejected.DecisionReason == nil || *rejected.DecisionReason != "Impressum fehlt" || rejected.OrganizationID != nil {
		t.Fatalf("rejected: %+v", rejected)
	}

	fake.Mu.Lock()
	_, created := fake.OrgAliases["stadtfest-bremen"]
	fake.Mu.Unlock()
	if created {
		t.Fatal("rejection must not create an organization")
	}
	var entries []model.AuditLogModel
	db.Where("resource_type = ? AND resource_id = ?", audit.ResourceOrganizerApplication, submitted.ApplicationID.String()).Find(&entries)
	if len(entries) != 1 || entries[0].OrganizationID != audit.ScopePlatform {
		t.Fatalf("audit entries: %+v", entries)
	}
	var notes []model.NotificationData
	db.Where("user_id = ? AND occasion = ?", applicantID, service.NotificationOccasionOrganizerApplication).Find(&notes)
	if len(notes) != 1 || !strings.Contains(notes[0].Content, "Impressum fehlt") {
		t.Fatalf("notifications: %+v", notes)
	}

	// After a rejection the user may apply again.
	decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
}

func TestOrganizerApplicationApprovalConflictDatabase(t *testing.T) {
	db := testdb.Open(t)
	_, applicant := seedApplicant(t, db)
	users, admins, fake := applicationRouters(t, db, applicant, reviewer())

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	// Someone else got the alias in Keycloak between submission and review.
	fake.Mu.Lock()
	fake.OrgAliases["stadtfest-bremen"] = "org-other"
	fake.Mu.Unlock()

	path := "/api/v1/admin/organizer-applications/" + submitted.ApplicationID.String()
	wantProblem(t, sendJSON(admins, http.MethodPost, path+"/approve", ""), http.StatusConflict)
	still := decodeApplication(t, sendJSON(admins, http.MethodGet, path, ""), http.StatusOK)
	if still.Status != model.OrganizerApplicationPending {
		t.Fatalf("application after failed approval: %+v", still)
	}
}

func TestOrganizerApplicationValidationAndAccessDatabase(t *testing.T) {
	db := testdb.Open(t)
	_, applicant := seedApplicant(t, db)
	users, _, _ := applicationRouters(t, db, applicant, reviewer())

	for name, body := range map[string]string{
		"missing message": `{"organizationName":"Fest","organizationAlias":"fest","contactEmail":"a@b.de"}`,
		"short message":   `{"organizationName":"Fest","organizationAlias":"fest","contactEmail":"a@b.de","message":"kurz"}`,
		"invalid email":   `{"organizationName":"Fest","organizationAlias":"fest","contactEmail":"keine-mail","message":"Wir planen viele Events."}`,
		"invalid alias":   `{"organizationName":"Fest","organizationAlias":"fest bremen!","contactEmail":"a@b.de","message":"Wir planen viele Events."}`,
		"hyphen at end":   `{"organizationName":"Fest","organizationAlias":"fest-","contactEmail":"a@b.de","message":"Wir planen viele Events."}`,
		"invalid country": `{"organizationName":"Fest","organizationAlias":"fest","contactEmail":"a@b.de","countryCode":"D1","message":"Wir planen viele Events."}`,
	} {
		t.Run(name, func(t *testing.T) {
			wantProblem(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", body), http.StatusBadRequest)
		})
	}

	// A user without the admin role cannot review applications.
	_, nonAdmin, _ := applicationRouters(t, db, applicant, applicant)
	rec := sendJSON(nonAdmin, http.MethodGet, "/api/v1/admin/organizer-applications", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin list: %d", rec.Code)
	}
	if rec := sendJSON(nonAdmin, http.MethodPost, "/api/v1/admin/organizer-applications/"+uuid.NewString()+"/approve", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin approve: %d", rec.Code)
	}

	_, admins, _ := applicationRouters(t, db, applicant, reviewer())
	wantProblem(t, sendJSON(admins, http.MethodGet, "/api/v1/admin/organizer-applications/"+uuid.NewString(), ""), http.StatusNotFound)
	wantProblem(t, sendJSON(admins, http.MethodGet, "/api/v1/admin/organizer-applications?status=open", ""), http.StatusBadRequest)
}

// EVENTHUB-304: the applications belong to organizations an org admin is not a member of, so the
// org_admin role of another organization grants neither insight nor a decision.
func TestOrganizerApplicationOrgAdminForbiddenDatabase(t *testing.T) {
	db := testdb.Open(t)
	_, applicant := seedApplicant(t, db)
	orgAdmin := orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin)
	users, orgAdmins, fake := applicationRouters(t, db, applicant, orgAdmin)

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	path := "/api/v1/admin/organizer-applications/" + submitted.ApplicationID.String()
	for _, request := range []struct{ name, method, path, body string }{
		{"list", http.MethodGet, "/api/v1/admin/organizer-applications", ""},
		{"inspect", http.MethodGet, path, ""},
		{"approve", http.MethodPost, path + "/approve", ""},
		{"reject", http.MethodPost, path + "/reject", `{"reason":"fremd"}`},
	} {
		rec := sendJSON(orgAdmins, request.method, request.path, request.body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("org admin %s: %d %s", request.name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "stadtfest") {
			t.Fatalf("org admin %s leaks the application: %s", request.name, rec.Body.String())
		}
	}

	fake.Mu.Lock()
	_, created := fake.OrgAliases["stadtfest-bremen"]
	fake.Mu.Unlock()
	if created {
		t.Fatal("a refused approval must not create an organization")
	}
	var stored model.OrganizerApplicationModel
	if err := db.First(&stored, "application_id = ?", submitted.ApplicationID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.OrganizerApplicationPending || stored.DecidedAt != nil {
		t.Fatalf("application after refused org admin review: %+v", stored)
	}
}

// Reviewers need not be allowed to manage Keycloak organizations with their own token: the
// approval runs with the backend service account.
func TestOrganizerApplicationApprovalUsesServiceAccountDatabase(t *testing.T) {
	db := testdb.Open(t)
	_, applicant := seedApplicant(t, db)
	admin := globalAdminPrincipal() // no AccessToken
	users, reviewers, fake := applicationRouters(t, db, applicant, admin)

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	approved := decodeApplication(t, sendJSON(reviewers, http.MethodPost, "/api/v1/admin/organizer-applications/"+submitted.ApplicationID.String()+"/approve", ""), http.StatusOK)
	if approved.Status != model.OrganizerApplicationApproved || approved.DecidedByUsername == nil || *approved.DecidedByUsername != admin.Username {
		t.Fatalf("approved: %+v", approved)
	}
	fake.Mu.Lock()
	keycloakOrgID := fake.OrgAliases["stadtfest-bremen"]
	tokens := fake.TokenRequests
	fake.Mu.Unlock()
	if keycloakOrgID == "" || tokens == 0 {
		t.Fatalf("organization %q created without the service account (token requests: %d)", keycloakOrgID, tokens)
	}
}

func TestOrganizerApplicationNoSelfReviewDatabase(t *testing.T) {
	db := testdb.Open(t)
	_, applicant := seedApplicant(t, db)
	// The applicant is a global admin and applies for an organization of their own.
	applicant.GlobalRoles[middleware.RoleAdmin] = struct{}{}
	users, reviewers, fake := applicationRouters(t, db, applicant, applicant)

	submitted := decodeApplication(t, sendJSON(users, http.MethodPost, "/api/v1/organizer-applications", validApplication), http.StatusCreated)
	path := "/api/v1/admin/organizer-applications/" + submitted.ApplicationID.String()
	wantProblem(t, sendJSON(reviewers, http.MethodPost, path+"/approve", ""), http.StatusForbidden)
	wantProblem(t, sendJSON(reviewers, http.MethodPost, path+"/reject", `{"reason":"selbst"}`), http.StatusForbidden)

	fake.Mu.Lock()
	_, created := fake.OrgAliases["stadtfest-bremen"]
	fake.Mu.Unlock()
	if created {
		t.Fatal("a refused self-approval must not create an organization")
	}
	if still := decodeApplication(t, sendJSON(reviewers, http.MethodGet, path, ""), http.StatusOK); still.Status != model.OrganizerApplicationPending {
		t.Fatalf("application after refused self-review: %+v", still)
	}
}
