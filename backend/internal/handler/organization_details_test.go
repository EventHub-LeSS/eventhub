package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/audit"
	"backend/internal/keycloakmock"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// orgDetailsRow is the database row of the mocked organization org-1 (alias-1).
var orgDetailsRow = &model.OrganizationModel{
	OrganizationID: uuid.MustParse("00000000-0000-0000-0000-00000000aa01"),
	KeycloakOrgID:  "org-1",
	Alias:          "alias-1",
	Name:           "Acme",
	CreatedAt:      time.Unix(1000, 0).UTC(),
	UpdatedAt:      time.Unix(2000, 0).UTC(),
}

// detailsOrgRepo mirrors the organization rows the organization details endpoints reach, so the
// handler tests exercise the whole change: resolution, authorization, Keycloak and the database.
type detailsOrgRepo struct {
	repository.OrganizationRepository
	row          *model.OrganizationModel
	getErr       error
	updateErr    error
	updatedIDs   []uuid.UUID
	updatedNames []string
}

func (r *detailsOrgRepo) GetByKeycloakOrgID(keycloakOrgID string) (*model.OrganizationModel, error) {
	return r.row, r.getErr
}

func (r *detailsOrgRepo) UpdateName(organizationID uuid.UUID, name string) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updatedIDs = append(r.updatedIDs, organizationID)
	r.updatedNames = append(r.updatedNames, name)
	row := *r.row
	row.Name = name
	r.row = &row
	return nil
}

func (r *detailsOrgRepo) updated() bool {
	return len(r.updatedIDs) > 0
}

// newOrgDetailsRouter mirrors the route registration of cmd/api for the organization details
// endpoints: the handler itself resolves the addressed organization and checks the roles.
func newOrgDetailsRouter(t *testing.T, principal *middleware.Principal, orgRepo *detailsOrgRepo) (http.Handler, *keycloakmock.Fake) {
	t.Helper()
	fake := keycloakmock.New()
	keycloak := fake.KeycloakService(t)
	h := NewOrganizationHandler(keycloak, service.NewOrganizationService(keycloak, orgRepo), orgRepo, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	orgs := r.Group("/api/v1/organizations")
	{
		orgs.GET("/:organizationID", h.GetOrganization)
		orgs.PUT("/:organizationID", middleware.Audit(audit.OrganizationUpdated), h.UpdateOrganization)
	}
	return r, fake
}

func requestOrgDetails(t *testing.T, router http.Handler, method, orgID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/v1/organizations/"+orgID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func patchOrgDetails(t *testing.T, router http.Handler, orgID, body string) *httptest.ResponseRecorder {
	t.Helper()
	return requestOrgDetails(t, router, http.MethodPut, orgID, body)
}

// keycloakAdminRequests filters the admin requests a change made to Keycloak.
func keycloakAdminRequests(fake *keycloakmock.Fake, prefix string) []string {
	fake.Mu.Lock()
	defer fake.Mu.Unlock()
	var matching []string
	for _, request := range fake.AdminRequests {
		if strings.HasPrefix(request, prefix) {
			matching = append(matching, request)
		}
	}
	return matching
}

// quietAuditLog silences the slog output the failure paths of the change produce.
func quietAuditLog() {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
}

// assertDetailsStatus checks the response shape of the organization details endpoints: 401/403
// stay model.APIError, every other status is problem+json (model.ErrorResponse).
func assertDetailsStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, want, rec.Body.String())
	}
	switch want {
	case http.StatusUnauthorized:
		wantAPIError(t, rec, want, "UNAUTHENTICATED")
	case http.StatusForbidden:
		wantAPIError(t, rec, want, "FORBIDDEN")
	case http.StatusOK:
	default:
		wantProblem(t, rec, want)
	}
}

// TestGetOrganization_DetailsAreReadByEveryMember covers who may read the organization details.
func TestGetOrganization_DetailsAreReadByEveryMember(t *testing.T) {
	row := *orgDetailsRow
	tests := []struct {
		name      string
		id        string
		principal *middleware.Principal
		row       *model.OrganizationModel
		getErr    error
		want      int
	}{
		{name: "unauthenticated", id: "org-1", want: http.StatusUnauthorized},
		{name: "org admin", id: "org-1", principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusOK},
		{name: "roleless member", id: "org-1", principal: &middleware.Principal{
			Subject:       "u-bob",
			GlobalRoles:   map[middleware.GlobalRole]struct{}{},
			Organizations: []*middleware.OrganizationAccess{{ID: "org-1", Alias: "alias-1"}},
		}, row: &row, want: http.StatusOK},
		{name: "alias only token", id: "org-1", principal: &middleware.Principal{
			Subject:       "u-bob",
			GlobalRoles:   map[middleware.GlobalRole]struct{}{},
			Organizations: []*middleware.OrganizationAccess{{ID: "alias-1", Alias: "alias-1"}},
		}, row: &row, want: http.StatusOK},
		{name: "global admin bypasses membership", id: "org-1", principal: globalAdminPrincipal(), row: &row, want: http.StatusOK},
		{name: "foreign member", id: "org-1", principal: orgRolesPrincipal("org-2", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusForbidden},
		{name: "unknown organization", id: "org-9", principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusForbidden},
		{name: "unknown organization as global admin", id: "org-9", principal: globalAdminPrincipal(), row: &row, want: http.StatusNotFound},
		{name: "database row missing", id: "org-1", principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), want: http.StatusInternalServerError},
		{name: "database lookup fails", id: "org-1", principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, getErr: errors.New("query failed"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietAuditLog()
			router, _ := newOrgDetailsRouter(t, tt.principal, &detailsOrgRepo{row: tt.row, getErr: tt.getErr})

			rec := requestOrgDetails(t, router, http.MethodGet, tt.id, "")
			assertDetailsStatus(t, rec, tt.want)
			if tt.want == http.StatusOK {
				var details model.OrganizationDetailsResponse
				if err := json.Unmarshal(rec.Body.Bytes(), &details); err != nil {
					t.Fatalf("unmarshal response: %v", err)
				}
				if details.ID != "org-1" || details.OrganizationID != row.OrganizationID.String() {
					t.Errorf("response = %+v, want the Keycloak and database organization IDs", details)
				}
				if details.Alias != row.Alias || details.Name != row.Name {
					t.Errorf("response = %+v, want alias %q and name %q", details, row.Alias, row.Name)
				}
			}
		})
	}
}

// TestUpdateOrganization_OrgAdminSetsName covers the org admin change through Keycloak and the
// database row, with the audit log recording started and succeeded under one operationId.
func TestUpdateOrganization_OrgAdminSetsName(t *testing.T) {
	repo := &detailsOrgRepo{row: orgDetailsRow}
	router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)

	rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
	assertDetailsStatus(t, rec, http.StatusOK)
	var resp model.UpdateOrganizationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.ID != "org-1" || resp.Alias != "alias-1" || resp.Name != "Acme Corporation" {
		t.Errorf("response = %+v, want the resolved organization and the new name", resp)
	}
	if repo.updatedNames[0] != "Acme Corporation" {
		t.Errorf("database name = %q, want the requested name", repo.updatedNames[0])
	}
	if repo.updatedIDs[0] != orgDetailsRow.OrganizationID {
		t.Errorf("database update = %s, want the database organization ID", repo.updatedIDs[0])
	}
	if fake.OrgNames["org-1"] != "Acme Corporation" {
		t.Errorf("Keycloak name = %q, want the requested name", fake.OrgNames["org-1"])
	}

	entries := fake.Audit.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want started and succeeded", len(entries))
	}
	if entries[0].Phase != audit.PhaseStarted || entries[1].Phase != audit.PhaseSucceeded {
		t.Errorf("phases = %q, %q; want started and succeeded", entries[0].Phase, entries[1].Phase)
	}
	for _, entry := range entries {
		if entry.Action != string(audit.OrganizationUpdated) {
			t.Errorf("action = %q, want %q", entry.Action, audit.OrganizationUpdated)
		}
		if entry.OrganizationID != "org-1" || entry.ResourceType != audit.ResourceOrganization {
			t.Errorf("entry = %s/%s/%s, want organization/org-1", entry.OrganizationID, entry.ResourceType, entry.ResourceID)
		}
	}
	if got, want := entries[0].Changes["nameBefore"], "Acme"; got != want {
		t.Errorf("nameBefore = %v, want %q", got, want)
	}
	if got, want := entries[0].Changes["nameAfter"], "Acme Corporation"; got != want {
		t.Errorf("nameAfter = %v, want %q", got, want)
	}
	if got, want := entries[1].Changes["name"], "Acme Corporation"; got != want {
		t.Errorf("result name = %v, want %q", got, want)
	}
	if entries[0].OperationID != entries[1].OperationID {
		t.Error("started and result entries share one operationId")
	}
}

// TestUpdateOrganization_UnchangedNameWritesNoEntry covers the no-op: the requested name is
// already current, so nothing changes and the audit log stays silent.
func TestUpdateOrganization_UnchangedNameWritesNoEntry(t *testing.T) {
	repo := &detailsOrgRepo{row: orgDetailsRow}
	router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)

	rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme"}`)
	assertDetailsStatus(t, rec, http.StatusOK)
	var resp model.UpdateOrganizationResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Name != "Acme" {
		t.Errorf("name = %q, want the current name", resp.Name)
	}
	if repo.updated() {
		t.Error("database name was updated although the request was a no-op")
	}
	if got := keycloakAdminRequests(fake, "PUT /admin/realms/eventhub/organizations/"); len(got) != 0 {
		t.Errorf("Keycloak PUT requests = %v, want none", got)
	}
	if entries := fake.Audit.Snapshot(); len(entries) != 0 {
		t.Errorf("audit entries = %d, want none for a no-op", len(entries))
	}
}

func TestUpdateOrganization_PreservesKeycloakSettings(t *testing.T) {
	repo := &detailsOrgRepo{row: orgDetailsRow}
	router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)
	fake.OrgDetails["org-1"] = map[string]any{
		"enabled":     false,
		"description": "Original description",
		"redirectUrl": "https://example.com/",
		"attributes":  map[string]any{"department": []string{"events"}},
		"domains":     []any{map[string]any{"name": "example.com", "verified": true}},
	}

	rec := patchOrgDetails(t, router, "org-1", `{"name":"Acme Corporation"}`)
	assertDetailsStatus(t, rec, http.StatusOK)
	details := fake.OrgDetails["org-1"]
	if fake.OrgAliases["alias-1"] != "org-1" || details["enabled"] != false ||
		details["description"] != "Original description" || details["redirectUrl"] != "https://example.com/" {
		t.Fatalf("organization settings changed: alias=%v details=%v", fake.OrgAliases, details)
	}
	actual, _ := json.Marshal(details["domains"])
	if string(actual) != `[{"name":"example.com","verified":true}]` {
		t.Errorf("domains changed: %s", actual)
	}
	actual, _ = json.Marshal(details["attributes"])
	if string(actual) != `{"department":["events"]}` {
		t.Errorf("attributes changed: %s", actual)
	}
}

func TestUpdateOrganization_ReconcilesKeycloakWhenDatabaseAlreadyMatches(t *testing.T) {
	repo := &detailsOrgRepo{row: orgDetailsRow}
	router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)
	fake.OrgNames["org-1"] = "Acme Corporation"

	rec := patchOrgDetails(t, router, "org-1", `{"name":"Acme"}`)
	assertDetailsStatus(t, rec, http.StatusOK)
	if fake.OrgNames["org-1"] != "Acme" || repo.updated() {
		t.Fatalf("expected Keycloak to be repaired without updating database: name=%q dbUpdated=%v", fake.OrgNames["org-1"], repo.updated())
	}
	entries := fake.Audit.Snapshot()
	if len(entries) != 2 || entries[0].Changes["keycloakNameBefore"] != "Acme Corporation" || entries[1].Phase != audit.PhaseSucceeded {
		t.Fatalf("repair not audited: %+v", entries)
	}
}

// TestUpdateOrganization_AliasIsReadOnly covers that a change of the alias is rejected instead of
// being dropped silently; an unchanged alias value is accepted like any stale form data.
func TestUpdateOrganization_AliasIsReadOnly(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "changed alias", body: `{"name": "Acme Corporation", "alias": "acme"}`, want: http.StatusBadRequest},
		{name: "unchanged alias", body: `{"name": "Acme Corporation", "alias": "alias-1"}`, want: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &detailsOrgRepo{row: orgDetailsRow}
			router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)

			rec := patchOrgDetails(t, router, "org-1", tt.body)
			assertDetailsStatus(t, rec, tt.want)
			changed := repo.updated() || fake.OrgNames["org-1"] != ""
			if tt.want != http.StatusOK && changed {
				t.Error("rejected request must not change anything")
			}
			if tt.want == http.StatusOK && !repo.updated() {
				t.Error("accepted request must update the name")
			}
		})
	}
}

// TestUpdateOrganization_AuthorizationAndValidation covers who may change the details and which
// bodies are accepted; a rejected request leaves Keycloak and the database untouched.
func TestUpdateOrganization_AuthorizationAndValidation(t *testing.T) {
	row := *orgDetailsRow
	tests := []struct {
		name      string
		id        string
		body      string
		principal *middleware.Principal
		row       *model.OrganizationModel
		want      int
	}{
		{name: "unauthenticated", id: "org-1", body: `{"name": "Neu"}`, want: http.StatusUnauthorized},
		{name: "foreign org admin", id: "org-1", body: `{"name": "Neu"}`, principal: orgRolesPrincipal("org-2", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusForbidden},
		{name: "event manager is no org admin", id: "org-1", body: `{"name": "Neu"}`, principal: orgRolesPrincipal("org-1", middleware.RoleEventManager), row: &row, want: http.StatusForbidden},
		{name: "roleless member", id: "org-1", body: `{"name": "Neu"}`, principal: &middleware.Principal{
			Subject:       "u-bob",
			GlobalRoles:   map[middleware.GlobalRole]struct{}{},
			Organizations: []*middleware.OrganizationAccess{{ID: "org-1", Alias: "alias-1"}},
		}, row: &row, want: http.StatusForbidden},
		{name: "unknown organization", id: "org-9", body: `{"name": "Neu"}`, principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusForbidden},
		{name: "unknown organization as global admin", id: "org-9", body: `{"name": "Neu"}`, principal: globalAdminPrincipal(), row: &row, want: http.StatusNotFound},
		{name: "global admin without org role", id: "org-1", body: `{"name": "Neu"}`, principal: globalAdminPrincipal(), row: &row, want: http.StatusOK},
		{name: "empty name", id: "org-1", body: `{"name": "   "}`, principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusBadRequest},
		{name: "missing name", id: "org-1", body: `{}`, principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusBadRequest},
		{name: "body is not an object", id: "org-1", body: `{"name"`, principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), row: &row, want: http.StatusBadRequest},
		{name: "database row missing", id: "org-1", body: `{"name": "Neu"}`, principal: orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietAuditLog()
			repo := &detailsOrgRepo{row: tt.row}
			router, fake := newOrgDetailsRouter(t, tt.principal, repo)

			rec := patchOrgDetails(t, router, tt.id, tt.body)
			assertDetailsStatus(t, rec, tt.want)
			changed := repo.updated() || fake.OrgNames["org-1"] != ""
			if tt.want != http.StatusOK && changed {
				t.Errorf("rejected request changed Keycloak or the database row (Keycloak name %q)", fake.OrgNames["org-1"])
			}
			if tt.want == http.StatusOK && !repo.updated() {
				t.Error("accepted request must update the database row")
			}
		})
	}
}

// TestUpdateOrganization_GlobalAdminChangeIsRecorded covers that a global admin change is audited
// under the personal account of the admin, not anonymously.
func TestUpdateOrganization_GlobalAdminChangeIsRecorded(t *testing.T) {
	router, fake := newOrgDetailsRouter(t, globalAdminPrincipal(), &detailsOrgRepo{row: orgDetailsRow})

	rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
	assertDetailsStatus(t, rec, http.StatusOK)
	entries := fake.Audit.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("audit entries = %d, want started and succeeded", len(entries))
	}
	if entries[0].ActorUsername != "caller" || entries[0].ActorSubject != "u-caller" {
		t.Errorf("actor = %s/%s, want the personal account of the admin", entries[0].ActorSubject, entries[0].ActorUsername)
	}
}

// TestUpdateOrganization_DemotedAdminCannotActOnOldToken covers the Keycloak re-check: the token
// still carries org_admin, but the role group of Keycloak no longer holds the member.
func TestUpdateOrganization_DemotedAdminCannotActOnOldToken(t *testing.T) {
	repo := &detailsOrgRepo{row: orgDetailsRow}
	router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)
	fake.Mu.Lock()
	delete(fake.GroupMembers["g-admin"], "u-alice")
	fake.Mu.Unlock()

	rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
	wantAPIError(t, rec, http.StatusForbidden, "FORBIDDEN")
	if repo.updated() || fake.OrgNames["org-1"] != "" {
		t.Error("demoted admin must not change anything")
	}
}

// TestUpdateOrganization_FailuresAreRecordedAsIncomplete covers the failure paths: a started
// entry that cannot be written refuses the whole change, a failed step is recorded as incomplete
// with the step that failed, and what already applied stays applied.
func TestUpdateOrganization_FailuresAreRecordedAsIncomplete(t *testing.T) {
	row := *orgDetailsRow
	tests := []struct {
		name string
		fail func(t *testing.T, fake *keycloakmock.Fake, repo *detailsOrgRepo)
		want int
	}{
		{name: "started entry cannot be written", fail: func(t *testing.T, fake *keycloakmock.Fake, repo *detailsOrgRepo) {
			fake.Audit.Fail = func(*model.AuditLogModel) error { return errors.New("disk full") }
		}, want: http.StatusInternalServerError},
		{name: "Keycloak update fails", fail: func(t *testing.T, fake *keycloakmock.Fake, repo *detailsOrgRepo) {
			fake.FailAdminRequest = func(method, path string) bool {
				return method == "PUT" && strings.Contains(path, "/organizations/")
			}
		}, want: http.StatusInternalServerError},
		{name: "database update fails", fail: func(t *testing.T, fake *keycloakmock.Fake, repo *detailsOrgRepo) {
			repo.updateErr = errors.New("row locked")
		}, want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietAuditLog()
			repo := &detailsOrgRepo{row: &row}
			router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)
			tt.fail(t, fake, repo)

			rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
			assertDetailsStatus(t, rec, tt.want)

			if tt.name == "started entry cannot be written" {
				// The refused change must not reach Keycloak or the database row.
				if got := keycloakAdminRequests(fake, "PUT /admin/realms/eventhub/organizations/"); len(got) != 0 {
					t.Errorf("Keycloak PUT requests = %v, want none", got)
				}
				if repo.updated() {
					t.Error("database name was updated although the started entry could not be written")
				}
				return
			}

			entries := fake.Audit.Snapshot()
			if len(entries) < 2 {
				t.Fatalf("audit entries = %d, want started and result", len(entries))
			}
			started := 0
			for _, entry := range entries {
				if entry.Phase == audit.PhaseStarted {
					started++
				}
			}
			if started != 1 {
				t.Errorf("started entries = %d, want exactly one", started)
			}
			last := entries[len(entries)-1]
			if last.Phase != audit.PhaseIncomplete {
				t.Errorf("phase = %q, want incomplete", last.Phase)
			}
			if last.Changes["failedStepOutcome"] != "unconfirmed" {
				t.Errorf("failedStepOutcome = %v, want unconfirmed", last.Changes["failedStepOutcome"])
			}
		})
	}
}

// TestUpdateOrganization_FailedStepsConverge covers the two failure steps: a failed Keycloak
// update leaves the database row unchanged, a failed database update still leaves the new name in
// Keycloak, so a retry sets the requested state whatever step failed before.
func TestUpdateOrganization_FailedStepsConverge(t *testing.T) {
	tests := []struct {
		name         string
		failKeycloak bool
		failDatabase bool
	}{
		{name: "failed Keycloak update", failKeycloak: true},
		{name: "failed database update", failDatabase: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quietAuditLog()
			repo := &detailsOrgRepo{row: orgDetailsRow}
			router, fake := newOrgDetailsRouter(t, orgRolesPrincipal("org-1", middleware.RoleOrganizationAdmin), repo)
			if tt.failKeycloak {
				fake.FailAdminRequest = func(method, path string) bool {
					return method == "PUT" && strings.Contains(path, "/organizations/")
				}
			}
			if tt.failDatabase {
				repo.updateErr = errors.New("row locked")
			}

			rec := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500, body: %s", rec.Code, rec.Body.String())
			}
			if tt.failKeycloak {
				if fake.OrgNames["org-1"] != "" {
					t.Errorf("Keycloak name = %q, want unchanged", fake.OrgNames["org-1"])
				}
				if repo.updated() {
					t.Error("database name was updated although the Keycloak update failed")
				}
			}
			if tt.failDatabase {
				if fake.OrgNames["org-1"] != "Acme Corporation" {
					t.Errorf("Keycloak name = %q, want the requested name", fake.OrgNames["org-1"])
				}
				if repo.updated() {
					t.Error("database name was updated although the update failed")
				}
			}
			fake.FailAdminRequest = nil
			repo.updateErr = nil
			retry := patchOrgDetails(t, router, "org-1", `{"name": "Acme Corporation"}`)
			assertDetailsStatus(t, retry, http.StatusOK)
			if fake.OrgNames["org-1"] != "Acme Corporation" || repo.row.Name != "Acme Corporation" {
				t.Fatalf("retry did not converge: Keycloak=%q database=%q", fake.OrgNames["org-1"], repo.row.Name)
			}
		})
	}
}
