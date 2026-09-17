package service_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"backend/internal/keycloakmock"
	"backend/internal/service"
)

func TestConfigureOrganizationMemberRoles_GrantsAndRevokesRoles(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// bob currently holds event_manager only.
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []string{"org_admin", "finance_viewer"})
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
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []string{"org_admin"})
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
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []string{"event_manager"})
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

	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []string{"event_manager"})
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

func TestConfigureOrganizationMemberRoles_ResolvesCurrentRolesPerUser(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// bob holds event_manager only and no admin is demoted, so the admin group
	// member listing must not be requested at all.
	if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []string{"finance_viewer"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	usedUserGroups := false
	for _, request := range fake.AdminRequests {
		if strings.HasSuffix(request, "/groups/g-admin/members") {
			t.Errorf("admin group members were listed although no admin was demoted: %s", request)
		}
		if strings.HasSuffix(request, "/users/u-bob/groups") {
			usedUserGroups = true
		}
	}
	if !usedUserGroups {
		t.Error("current roles must be resolved through the per-user groups listing")
	}
}

func TestConfigureOrganizationMemberRoles_EmptyRoleSetStripsAllRoles(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", nil)
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
	change, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "alias-1", "bob", []string{"finance_viewer"})
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

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "dave", []string{"event_manager"})
	if !errors.Is(err, service.ErrUserNotFound) {
		t.Fatalf("err = %v, want ErrUserNotFound", err)
	}
}

func TestConfigureOrganizationMemberRoles_UserNotAMember(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	// carol exists in the realm but is not a member of org-1.
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "carol", []string{"event_manager"})
	if !errors.Is(err, service.ErrNotAMember) {
		t.Fatalf("err = %v, want ErrNotAMember", err)
	}
}

func TestConfigureOrganizationMemberRoles_UnknownOrganization(t *testing.T) {
	fake := keycloakmock.New()
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-x", "bob", []string{"event_manager"})
	if !errors.Is(err, service.ErrOrganizationNotFound) {
		t.Fatalf("err = %v, want ErrOrganizationNotFound", err)
	}
}

func TestConfigureOrganizationMemberRoles_IncompleteRoleGroups(t *testing.T) {
	fake := keycloakmock.New()
	fake.OrgGroups["org-2"] = map[string]string{"org_admin": "g2-admin"}
	fake.OrgMembers["org-2"] = map[string]bool{"u-bob": true}
	kc := fake.KeycloakService(t)

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-2", "bob", []string{"event_manager"})
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
	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "bob", []string{"org_admin", "finance_viewer"})
	if err == nil {
		t.Fatal("expected an error when the grant fails")
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

	_, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-x", "bob", []string{"event_manager"})
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
			_, errs[i] = kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", target, []string{"event_manager"})
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
		if _, err := kc.ConfigureOrganizationMemberRoles(context.Background(), "org-1", "alice", []string{"org_admin"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if fake.TokenRequests != 1 {
		t.Errorf("token requests = %d, want 1 (cached across calls)", fake.TokenRequests)
	}
}
