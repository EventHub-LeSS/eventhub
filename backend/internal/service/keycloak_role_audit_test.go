package service_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"backend/internal/audit"
	"backend/internal/keycloakmock"
	"backend/internal/model"
	"backend/internal/service"
)

func roles(names ...string) []model.OrganizationRole {
	out := make([]model.OrganizationRole, 0, len(names))
	for _, n := range names {
		out = append(out, model.OrganizationRole(n))
	}
	return out
}

func TestRoleChangeAudit_RecordsStartBeforeChangeAndResultAfter(t *testing.T) {
	fake := keycloakmock.New()
	var changesAtStart int
	fake.Audit.Fail = func(e *model.AuditLogModel) error {
		if e.Phase == audit.PhaseStarted {
			fake.Mu.Lock()
			changesAtStart = len(fake.Grants) + len(fake.Revokes)
			fake.Mu.Unlock()
		}
		return nil
	}
	kc := fake.KeycloakService(t)

	// bob holds event_manager; he becomes finance_viewer.
	if _, err := kc.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "bob", roles("finance_viewer"), globalAdmin); err != nil {
		t.Fatal(err)
	}

	if changesAtStart != 0 {
		t.Errorf("the start was recorded after %d Keycloak changes", changesAtStart)
	}
	entries := fake.Audit.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("expected start and result entry, got %d", len(entries))
	}
	start, result := entries[0], entries[1]
	if start.Phase != audit.PhaseStarted || result.Phase != audit.PhaseSucceeded {
		t.Errorf("phases = %s, %s", start.Phase, result.Phase)
	}
	if start.OperationID != result.OperationID {
		t.Error("start and result must share the operation ID")
	}
	for _, e := range entries {
		if e.ActorSubject != "u-caller" || e.ActorUsername != "caller" {
			t.Errorf("actor = %q/%q, want the personal account of the caller", e.ActorSubject, e.ActorUsername)
		}
		if e.OrganizationID != "org-1" || e.Action != string(audit.OrganizationMemberRolesChange) ||
			e.ResourceType != audit.ResourceOrganizationMember || e.ResourceID != "u-bob" {
			t.Errorf("unexpected entry %+v", e)
		}
		if e.Changes["targetUsername"] != "bob" {
			t.Errorf("target = %v", e.Changes["targetUsername"])
		}
	}
	if got := result.Changes["granted"].([]any); len(got) != 1 || got[0] != "finance_viewer" {
		t.Errorf("granted = %v", got)
	}
	if got := result.Changes["revoked"].([]any); len(got) != 1 || got[0] != "event_manager" {
		t.Errorf("revoked = %v", got)
	}
}

func TestRoleChangeAudit_UnchangedRolesWriteNoEntry(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	if _, err := kc.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "alice", roles("org_admin"), globalAdmin); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Audit.Snapshot()); n != 0 {
		t.Errorf("no-op recorded %d entries", n)
	}
}

func TestRoleChangeAudit_NoChangeWhenStartCannotBeRecorded(t *testing.T) {
	fake := keycloakmock.New()
	fake.Audit.Fail = func(*model.AuditLogModel) error { return errors.New("database down") }
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "bob", roles("finance_viewer"), globalAdmin)
	if !errors.Is(err, audit.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if len(fake.Grants)+len(fake.Revokes) != 0 {
		t.Errorf("Keycloak was changed without an audit record: grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestRoleChangeAudit_RefusesWithoutContextOrAuditLog(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", roles("finance_viewer"), globalAdmin); !errors.Is(err, audit.ErrMissingContext) {
		t.Fatalf("expected ErrMissingContext, got %v", err)
	}
	if len(fake.AdminRequests) != 0 {
		t.Errorf("Keycloak was contacted without audit context: %v", fake.AdminRequests)
	}

	unconfigured := service.NewKeycloakService(service.KeycloakClientConfig{Host: fake.Server(t).URL, AdminRealm: "master", UserRealm: "eventhub", ClientID: "backend", ClientSecret: "x"})
	if _, err := unconfigured.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "bob", roles("finance_viewer"), globalAdmin); !errors.Is(err, audit.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable without audit log, got %v", err)
	}
}

func TestRoleChangeAudit_PartialFailureIsRecordedAsIncomplete(t *testing.T) {
	fake := keycloakmock.New()
	fake.FailAdminRequest = func(method, path string) bool {
		return method == http.MethodPut && strings.HasSuffix(path, "/groups/g-admin/members/u-bob")
	}
	kc := fake.KeycloakService(t)

	// bob: event_manager revoked, then granting org_admin fails.
	_, err := kc.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "bob", roles("org_admin"), globalAdmin)
	if err == nil {
		t.Fatal("expected an error")
	}
	entries := fake.Audit.Snapshot()
	if len(entries) != 2 {
		t.Fatalf("expected start and result, got %d", len(entries))
	}
	result := entries[1]
	if result.Phase != audit.PhaseIncomplete {
		t.Fatalf("phase = %s, want incomplete", result.Phase)
	}
	if revoked := result.Changes["revoked"].([]any); len(revoked) != 1 || revoked[0] != "event_manager" {
		t.Errorf("confirmed revokes = %v", revoked)
	}
	if result.Changes["failedStep"] != "grant" || result.Changes["failedRole"] != "org_admin" || result.Changes["failedStepOutcome"] != "unconfirmed" {
		t.Errorf("failure details = %v", result.Changes)
	}
	for _, v := range result.Changes {
		if s, ok := v.(string); ok && strings.Contains(s, "500") {
			t.Errorf("raw provider error leaked into the audit log: %q", s)
		}
	}
}

func TestRoleChangeAudit_ResultIsRecordedWhenCallerGoesAway(t *testing.T) {
	fake := keycloakmock.New()
	ctx, cancel := context.WithCancel(auditCtx())
	defer cancel()
	fake.Audit.Fail = func(e *model.AuditLogModel) error {
		if e.Phase == audit.PhaseStarted {
			cancel() // the client disconnects right after the start was recorded
		}
		return nil
	}
	kc := fake.KeycloakService(t)

	if _, err := kc.ConfigureOrganizationMemberRoles(ctx, "org-1", "bob", roles("finance_viewer"), globalAdmin); err == nil {
		t.Fatal("expected an error for the cancelled request")
	}
	entries := fake.Audit.Snapshot()
	if len(entries) != 2 || entries[1].Phase != audit.PhaseIncomplete {
		t.Fatalf("expected start and incomplete result, got %+v", entries)
	}
}

func TestRoleChangeAudit_UnrecordedResultIsReported(t *testing.T) {
	fake := keycloakmock.New()
	fake.Audit.Fail = func(e *model.AuditLogModel) error {
		if e.Phase != audit.PhaseStarted {
			return errors.New("database down")
		}
		return nil
	}
	kc := fake.KeycloakService(t)

	change, err := kc.ConfigureOrganizationMemberRoles(auditCtx(), "org-1", "bob", roles("finance_viewer"), globalAdmin)
	if !errors.Is(err, audit.ErrResultNotRecorded) {
		t.Fatalf("expected ErrResultNotRecorded, got %v", err)
	}
	if change == nil || len(change.Granted) != 1 {
		t.Errorf("applied change must still be reported, got %+v", change)
	}
	if entries := fake.Audit.Snapshot(); len(entries) != 1 || entries[0].Phase != audit.PhaseStarted {
		t.Errorf("a started entry without result must remain: %+v", entries)
	}
}
