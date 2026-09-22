package service_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"backend/internal/keycloakmock"
	"backend/internal/model"
	"backend/internal/service"

	"github.com/henning-kln/gocloak"
)

// globalAdmin is the actor of tests that are not about the actor check.
var globalAdmin = service.OrgRoleActor{GlobalAdmin: true}

func TestConfigureOrganizationMemberRoles_GrantsAndRevokesRoles(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// bob currently holds event_manager only.
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"org_admin", "finance_viewer"}, globalAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(change.Applied, ","), "org_admin,finance_viewer"; got != want {
		t.Errorf("applied = %q, want %q", got, want)
	}
	if got, want := strings.Join(change.Granted, ","), "org_admin,finance_viewer"; got != want {
		t.Errorf("granted = %q, want %q", got, want)
	}
	if got, want := strings.Join(change.Revoked, ","), "event_manager"; got != want {
		t.Errorf("revoked = %q, want %q", got, want)
	}
	if change.OrganizationID != "org-1" {
		t.Errorf("organizationID = %q, want resolved id org-1", change.OrganizationID)
	}
	if got, want := strings.Join(fake.Grants, ","), "g-admin/u-bob,g-finance/u-bob"; got != want {
		t.Errorf("grants = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.Revokes, ","), "g-manager/u-bob"; got != want {
		t.Errorf("revokes = %q, want %q", got, want)
	}
}

func TestConfigureOrganizationMemberRoles_IsIdempotentForUnchangedRoles(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// alice currently holds org_admin only.
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"org_admin"}, globalAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(change.Applied, ","), "org_admin"; got != want {
		t.Errorf("applied = %q, want %q", got, want)
	}
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("expected no group changes, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
}

func TestConfigureOrganizationMemberRoles_RejectsRemovingLastAdmin(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// alice is the only org_admin of org-1.
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if !errors.Is(err, service.ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("expected no group changes, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}
	if !fake.GroupMembers["g-admin"]["u-alice"] {
		t.Error("alice must keep the org_admin role")
	}
}

func TestConfigureOrganizationMemberRoles_AllowsRemovingAdminWithSecondAdmin(t *testing.T) {
	fake := keycloakmock.New()
	fake.GroupMembers["g-admin"]["u-bob"] = true
	kc := fake.KeycloakService(t)

	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(change.Applied, ","), "event_manager"; got != want {
		t.Errorf("applied = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.Revokes, ","), "g-admin/u-alice"; got != want {
		t.Errorf("revokes = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.Grants, ","), "g-manager/u-alice"; got != want {
		t.Errorf("grants = %q, want %q", got, want)
	}
}

func TestConfigureOrganizationMemberRoles_ResolvesCurrentRolesThroughGroupMembers(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// bob holds event_manager only and no admin is demoted.
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"finance_viewer"}, globalAdmin); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	scanned := make(map[string]bool)
	for _, request := range fake.AdminRequests {
		// Keycloak filters organization groups out of the per-user groups
		// endpoint, so current roles must never be resolved through it.
		if strings.HasSuffix(request, "/users/u-bob/groups") {
			t.Errorf("per-user groups listing requested although it never returns org groups: %s", request)
		}
		if strings.HasPrefix(request, "GET /admin/realms/eventhub/organizations/org-1/groups/") && strings.HasSuffix(request, "/members") {
			scanned[request] = true
		}
	}
	for _, groupID := range []string{"g-admin", "g-manager", "g-finance"} {
		expected := "GET /admin/realms/eventhub/organizations/org-1/groups/" + groupID + "/members"
		if !scanned[expected] {
			t.Errorf("current roles must be resolved through the member listing of every role group, missing: %s", expected)
		}
	}
}

func TestConfigureOrganizationMemberRoles_EmptyRoleSetStripsAllRoles(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", nil, globalAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if change.Applied == nil || len(change.Applied) != 0 {
		t.Errorf("applied = %v, want empty non-nil slice", change.Applied)
	}
	if got, want := strings.Join(fake.Revokes, ","), "g-manager/u-bob"; got != want {
		t.Errorf("revokes = %q, want %q", got, want)
	}
}

func TestConfigureOrganizationMemberRoles_ResolvesOrganizationAlias(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// Tokens may identify organizations by alias when the claim carries no ID.
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "alias-1", "bob", []model.OrganizationRole{"finance_viewer"}, globalAdmin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.Join(change.Applied, ","), "finance_viewer"; got != want {
		t.Errorf("applied = %q, want %q", got, want)
	}
	if change.OrganizationID != "org-1" {
		t.Errorf("organizationID = %q, want resolved id org-1", change.OrganizationID)
	}
	if got, want := strings.Join(fake.Grants, ","), "g-finance/u-bob"; got != want {
		t.Errorf("grants = %q, want %q", got, want)
	}
	if got, want := strings.Join(fake.Revokes, ","), "g-manager/u-bob"; got != want {
		t.Errorf("revokes = %q, want %q", got, want)
	}
}

func TestConfigureOrganizationMemberRoles_UserNotFound(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "dave", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if !errors.Is(err, service.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestConfigureOrganizationMemberRoles_UserNotAMember(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// carol exists in the realm but is not a member of org-1.
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "carol", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if !errors.Is(err, service.ErrNotAMember) {
		t.Fatalf("err = %v, want ErrNotAMember", err)
	}
}

func TestConfigureOrganizationMemberRoles_UnknownOrganization(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-x", "bob", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if !errors.Is(err, service.ErrOrganizationNotFound) {
		t.Fatalf("err = %v, want ErrOrganizationNotFound", err)
	}
}

func TestConfigureOrganizationMemberRoles_IncompleteRoleGroups(t *testing.T) {
	fake := keycloakmock.New()
	fake.OrgGroups["org-2"] = map[string]string{"org_admin": "g2-admin"}
	fake.OrgMembers["org-2"] = map[string]bool{"u-bob": true}
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-2", "bob", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if !errors.Is(err, service.ErrOrgGroupsMissing) {
		t.Fatalf("err = %v, want ErrOrgGroupsMissing", err)
	}
}

func TestConfigureOrganizationMemberRoles_FailsClosedWhenGrantFailsAfterRevoke(t *testing.T) {
	fake := keycloakmock.New()
	// Fail the org_admin grant; the event_manager revoke happens before it.
	fake.FailAdminRequest = func(method, path string) bool {
		return method == http.MethodPut && strings.HasSuffix(path, "/groups/g-admin/members/u-bob")
	}
	kc := fake.KeycloakService(t)

	// bob holds event_manager and requests org_admin + finance_viewer.
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"org_admin", "finance_viewer"}, globalAdmin)
	if err == nil {
		t.Fatal("expected an error when the grant fails")
	}
	// The changes applied before the failure are reported for the audit log.
	if change == nil || strings.Join(change.Revoked, ",") != "event_manager" || len(change.Granted) != 0 {
		t.Errorf("partial change = %+v, want revoked=[event_manager] and nothing granted", change)
	}
	if errors.Is(err, service.ErrLastAdmin) || errors.Is(err, service.ErrOrgGroupsMissing) {
		t.Fatalf("unexpected sentinel error: %v", err)
	}

	// The revoke ran before the failing grant.
	if got, want := strings.Join(fake.Revokes, ","), "g-manager/u-bob"; got != want {
		t.Errorf("revokes = %q, want %q (revoke must run before grants)", got, want)
	}
	// Fail closed: bob must not hold the union of old and new roles, nor the
	// intended new set; he ends up with no roles at all.
	for _, group := range []string{"g-admin", "g-manager", "g-finance"} {
		if fake.GroupMembers[group]["u-bob"] {
			t.Errorf("bob must not be in group %q after the failed update", group)
		}
	}
}

func TestConfigureOrganizationMemberRoles_OutageDuringAliasLookupIsNotOrganizationNotFound(t *testing.T) {
	fake := keycloakmock.New()
	// The by-ID lookup 404s for org-x; the alias listing then fails with 500.
	fake.FailAdminRequest = func(method, path string) bool {
		return method == http.MethodGet && path == "/admin/realms/eventhub/organizations"
	}
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-x", "bob", []model.OrganizationRole{"event_manager"}, globalAdmin)
	if err == nil {
		t.Fatal("expected an error when the organization listing fails")
	}
	if errors.Is(err, service.ErrOrganizationNotFound) {
		t.Fatalf("infrastructure outage must not surface as ErrOrganizationNotFound: %v", err)
	}
}

func TestConfigureOrganizationMemberRoles_ConcurrentDemotionsLeaveOneAdmin(t *testing.T) {
	fake := keycloakmock.New()
	fake.GroupMembers["g-admin"]["u-bob"] = true // alice and bob are both org_admins
	kc := fake.KeycloakService(t)

	// alice and bob demote each other concurrently; without per-organization
	// serialization both would pass the last-admin guard and strip all admins.
	const calls = 2
	errs := make([]error, calls)
	var wg sync.WaitGroup
	targets := []string{"alice", "bob"}
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target string) {
			defer wg.Done()
			_, errs[i] = kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", target, []model.OrganizationRole{"event_manager"}, globalAdmin)
		}(i, target)
	}
	wg.Wait()

	lastAdmin := 0
	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, service.ErrLastAdmin):
			lastAdmin++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || lastAdmin != 1 {
		t.Fatalf("succeeded = %d, lastAdmin rejections = %d; want exactly one of each (errs: %v)", succeeded, lastAdmin, errs)
	}
	if got := len(fake.GroupMembers["g-admin"]); got != 1 {
		t.Fatalf("org-1 has %d admins left, want exactly 1", got)
	}
}

func TestConfigureOrganizationMemberRoles_CachesServiceAccountToken(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	for range 2 {
		if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"org_admin"}, globalAdmin); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if fake.TokenRequests != 1 {
		t.Errorf("token requests = %d, want 1 (cached across calls)", fake.TokenRequests)
	}
}

func TestConfigureOrganizationMemberRoles_RechecksThatTheActorIsStillOrgAdmin(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// bob's token may still claim org_admin, but in Keycloak he is only event_manager.
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"org_admin"}, service.OrgRoleActor{UserID: "u-bob"})
	if !errors.Is(err, service.ErrActorNotOrgAdmin) {
		t.Fatalf("err = %v, want ErrActorNotOrgAdmin", err)
	}
	if len(fake.Grants) != 0 || len(fake.Revokes) != 0 {
		t.Errorf("no roles may change, got grants=%v revokes=%v", fake.Grants, fake.Revokes)
	}

	// alice is org_admin in Keycloak and may change roles.
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"finance_viewer"}, service.OrgRoleActor{UserID: "u-alice"}); err != nil {
		t.Fatalf("unexpected error for an org admin actor: %v", err)
	}
}

func TestConfigureOrganizationMemberRoles_RejectsActorWithoutUserID(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"finance_viewer"}, service.OrgRoleActor{})
	if !errors.Is(err, service.ErrActorNotOrgAdmin) {
		t.Fatalf("err = %v, want ErrActorNotOrgAdmin", err)
	}
}

func TestConfigureOrganizationMemberRoles_DropsRejectedServiceAccountToken(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// Keycloak no longer accepts the cached token, e.g. after a restart.
	fake.RejectAdminRequests = 1
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"org_admin"}, globalAdmin); err == nil {
		t.Fatal("expected the rejected request to fail")
	}
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []model.OrganizationRole{"org_admin"}, globalAdmin); err != nil {
		t.Fatalf("the next request must log in again and succeed: %v", err)
	}
	if fake.TokenRequests != 2 {
		t.Errorf("token requests = %d, want 2 (rejected token dropped from the cache)", fake.TokenRequests)
	}
}

func TestConfigureOrganizationMemberRoles_TimesOutAndReleasesTheLock(t *testing.T) {
	defer service.SetOrgRoleChangeTimeout(100 * time.Millisecond)()
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	fake.Mu.Lock()
	fake.AdminDelay = time.Second
	fake.Mu.Unlock()
	start := time.Now()
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"finance_viewer"}, globalAdmin)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("call took %v, want it bounded by the timeout", elapsed)
	}

	fake.Mu.Lock()
	fake.AdminDelay = 0
	fake.Mu.Unlock()
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []model.OrganizationRole{"finance_viewer"}, globalAdmin); err != nil {
		t.Fatalf("the organization lock must be released after the timeout: %v", err)
	}
}

func TestResolveOrganization_ByIDAndByAlias(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	for _, idOrAlias := range []string{"org-1", "alias-1"} {
		ref, err := kc.ResolveOrganization(context.Background(), idOrAlias)
		if err != nil {
			t.Fatalf("ResolveOrganization(%q): %v", idOrAlias, err)
		}
		if ref.ID != "org-1" || ref.Alias != "alias-1" {
			t.Errorf("ResolveOrganization(%q) = %+v, want ID org-1 and alias alias-1", idOrAlias, ref)
		}
	}
	if _, err := kc.ResolveOrganization(context.Background(), "org-x"); !errors.Is(err, service.ErrOrganizationNotFound) {
		t.Errorf("unknown organization: err = %v, want ErrOrganizationNotFound", err)
	}
}

func TestCreateOrganization_RejectsTakenAlias(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	name, alias := "Another name", "alias-1"
	_, _, err := kc.CreateOrganization(context.Background(), "svc-token", gocloak.OrganizationRepresentation{Name: &name, Alias: &alias}, "alice")
	if !errors.Is(err, service.ErrOrganizationExists) {
		t.Fatalf("err = %v, want ErrOrganizationExists", err)
	}
	for _, request := range fake.AdminRequests {
		if strings.HasPrefix(request, "POST ") {
			t.Errorf("nothing may be created for a taken alias, got %s", request)
		}
	}
}
