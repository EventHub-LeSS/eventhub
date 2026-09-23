package seed

import (
	"context"
	"fmt"
	"log"

	"backend/internal/service"

	"github.com/henning-kln/gocloak"
)

type KeycloakSeeder struct {
	kc         *service.KeycloakService
	realm      string
	token      string
	groupNames []string
}

func NewKeycloakSeeder(kc *service.KeycloakService, realm, token string) *KeycloakSeeder {
	return &KeycloakSeeder{
		kc:         kc,
		realm:      realm,
		token:      token,
		groupNames: []string{"org_admin", "event_manager", "finance_viewer"},
	}
}

func (s *KeycloakSeeder) Seed(ctx context.Context) error {
	if err := s.ensureOrganizationClaimName(ctx); err != nil {
		return err
	}
	if err := s.ensureOrganizationGroupsMapper(ctx); err != nil {
		return err
	}
	if err := s.ensureServiceAccount(ctx); err != nil {
		return err
	}
	if err := s.ensureEmailUsernames(ctx); err != nil {
		return err
	}
	if err := s.seedUsers(ctx); err != nil {
		return err
	}
	if err := s.seedOrganizations(ctx); err != nil {
		return err
	}
	return s.seedMemberships(ctx)
}

// ensureOrganizationClaimName repairs realms that were imported before claim.name was added
// to the organization membership mapper; --import-realm never updates an existing realm.
func (s *KeycloakSeeder) ensureOrganizationClaimName(ctx context.Context) error {
	updated, err := s.kc.EnsureOrganizationClaimName(ctx, s.token, s.realm)
	if err != nil {
		return fmt.Errorf("ensure organization claim name: %w", err)
	}
	if updated {
		log.Println("keycloak: set claim.name on the organization membership mapper")
	}
	return nil
}

// ensureOrganizationGroupsMapper repairs realms that were imported before the organization groups
// mapper was added; without it tokens carry no organization roles (EVENTHUB-188).
func (s *KeycloakSeeder) ensureOrganizationGroupsMapper(ctx context.Context) error {
	added, err := s.kc.EnsureOrganizationGroupsMapper(ctx, s.token, s.realm)
	if err != nil {
		return fmt.Errorf("ensure organization groups mapper: %w", err)
	}
	if added {
		log.Println("keycloak: added the organization groups mapper to the organization client scope")
	}
	return nil
}

// ensureServiceAccount repairs realms that were imported before the backend client got its
// service account (EVENTHUB-188); --import-realm never updates an existing realm.
func (s *KeycloakSeeder) ensureServiceAccount(ctx context.Context) error {
	updated, err := s.kc.EnsureServiceAccount(ctx, s.token, s.realm)
	if err != nil {
		return fmt.Errorf("ensure backend service account: %w", err)
	}
	if updated {
		log.Println("keycloak: enabled the backend service account and granted its roles")
	}
	return nil
}

// ensureEmailUsernames repairs realms seeded before usernames were unified to email addresses:
// the realm registers with registrationEmailAsUsername, but --import-realm never updates an
// existing realm, so users like the legacy "großmeister_finn" keep their stored plain username
// until it is rewritten. Without the rewrite the seed could not even create the email-named
// user, because the email is already taken. The admin API reports the email as the username
// in such realms, so the repair writes every user with an email address; service accounts
// have no email and are skipped.
func (s *KeycloakSeeder) ensureEmailUsernames(ctx context.Context) error {
	updated, err := s.kc.EnsureEmailUsernames(ctx, s.token, s.realm)
	if err != nil {
		return fmt.Errorf("ensure email usernames: %w", err)
	}
	if updated > 0 {
		log.Printf("keycloak: set the stored username of %d user(s) to their email address", updated)
	}
	return nil
}

func (s *KeycloakSeeder) seedUsers(ctx context.Context) error {
	log.Println("keycloak: seeding users...")
	for _, mu := range mockUsers {
		existing, err := s.kc.GetUserByUsername(ctx, s.token, s.realm, mu.Username)
		if err != nil {
			return fmt.Errorf("lookup user %q: %w", mu.Username, err)
		}
		if existing != nil {
			continue
		}
		enabled := true
		emailVerified := true
		temporary := false
		credType := "password"
		cred := gocloak.CredentialRepresentation{
			Type:      &credType,
			Value:     ptrString(defaultPassword),
			Temporary: &temporary,
		}
		userRep := gocloak.User{
			Username:      ptrString(mu.Username),
			Email:         ptrString(mu.Email),
			FirstName:     ptrString(mu.FirstName),
			LastName:      ptrString(mu.LastName),
			Enabled:       &enabled,
			EmailVerified: &emailVerified,
			Credentials:   &[]gocloak.CredentialRepresentation{cred},
		}
		if _, err := s.kc.CreateUserWithToken(ctx, s.token, s.realm, userRep); err != nil {
			return fmt.Errorf("create user %q: %w", mu.Username, err)
		}
		log.Printf("  created user %s", mu.Username)
	}
	return nil
}

func (s *KeycloakSeeder) seedOrganizations(ctx context.Context) error {
	log.Println("keycloak: seeding organizations...")
	for _, mo := range mockOrgs {
		orgID, err := s.kc.GetOrganizationIDBySlug(ctx, s.token, mo.Alias)
		if err == nil {
			if err := s.ensureOrgGroups(ctx, orgID); err != nil {
				return fmt.Errorf("ensure groups for org %q: %w", mo.Alias, err)
			}
			continue
		}
		desc := mo.Description
		enabled := true
		orgRep := gocloak.OrganizationRepresentation{
			Name:        ptrString(mo.Name),
			Alias:       ptrString(mo.Alias),
			Description: &desc,
			Enabled:     &enabled,
		}
		orgID, _, err = s.kc.CreateOrganization(ctx, s.token, orgRep, mo.AdminUser)
		if err != nil {
			return fmt.Errorf("create org %q: %w", mo.Alias, err)
		}
		log.Printf("  created org %s (%s)", mo.Name, orgID)
		if err := s.ensureOrgGroups(ctx, orgID); err != nil {
			return fmt.Errorf("ensure groups for org %q: %w", mo.Alias, err)
		}
	}
	return nil
}

func (s *KeycloakSeeder) seedMemberships(ctx context.Context) error {
	log.Println("keycloak: seeding memberships and group assignments...")
	for _, mm := range mockMemberships {
		user, err := s.kc.GetUserByUsername(ctx, s.token, s.realm, mm.UserUsername)
		if err != nil || user == nil || user.ID == nil {
			return fmt.Errorf("lookup member %q: %w", mm.UserUsername, err)
		}
		orgID, err := s.kc.GetOrganizationIDBySlug(ctx, s.token, mm.OrgAlias)
		if err != nil {
			return fmt.Errorf("lookup org %q for membership: %w", mm.OrgAlias, err)
		}
		if err := s.kc.AddUserToOrganizationWithToken(ctx, s.token, orgID, *user.ID); err != nil {
			log.Printf("  skip membership %s->%s (likely exists): %v", mm.UserUsername, mm.OrgAlias, err)
		}
		groupID, err := s.kc.GetOrganizationGroupIDByName(ctx, s.token, orgID, mm.Role)
		if err != nil {
			return fmt.Errorf("find group %q in org %q: %w", mm.Role, mm.OrgAlias, err)
		}
		if err := s.kc.AssignUserToOrgGroup(ctx, s.token, *user.ID, orgID, groupID); err != nil {
			log.Printf("  skip group %s@%s=%s (likely assigned): %v", mm.UserUsername, mm.OrgAlias, mm.Role, err)
		} else {
			log.Printf("  assigned %s -> %s/%s", mm.UserUsername, mm.OrgAlias, mm.Role)
		}
	}
	return nil
}

func (s *KeycloakSeeder) ensureOrgGroups(ctx context.Context, orgID string) error {
	groups, err := s.kc.GetOrganizationGroups(ctx, s.token, s.realm, orgID)
	if err != nil {
		return fmt.Errorf("list org groups: %w", err)
	}
	existing := make(map[string]string)
	for _, g := range groups {
		if g.Name != nil {
			existing[*g.Name] = ptrValue(g.ID)
		}
	}
	for _, name := range s.groupNames {
		if _, ok := existing[name]; ok {
			continue
		}
		gName := name
		gID, err := s.kc.CreateOrganizationGroupWithToken(ctx, s.token, s.realm, orgID, gocloak.Group{Name: &gName})
		if err != nil {
			return fmt.Errorf("create org group %q: %w", name, err)
		}
		existing[name] = gID
	}
	return nil
}

func ptrString(s string) *string { return &s }
func ptrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
