package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/henning-kln/gocloak"
)

type KeycloakClientConfig struct {
	Host             string
	AdminRealm       string // Only for admin operations, e.g. creating users, creating orgs etc.
	UserRealm        string // Only for eventhub users
	ClientID         string
	ClientSecret     string
	FrontendClientID string // Public client used for password-grant login (sync script)
}

type KeycloakService struct {
	cfg    KeycloakClientConfig
	client *gocloak.GoCloak
}

func NewKeycloakService(cfg KeycloakClientConfig) *KeycloakService {
	return &KeycloakService{
		cfg:    cfg,
		client: gocloak.NewClient(cfg.Host),
	}
}

func (k *KeycloakService) login(ctx context.Context) (*gocloak.JWT, error) {
	token, err := k.client.LoginClient(ctx, k.cfg.ClientID, k.cfg.ClientSecret, k.cfg.AdminRealm)
	if err != nil {
		return nil, fmt.Errorf("keycloak admin login failed: %w", err)
	}
	return token, nil
}

func (k *KeycloakService) LoginUser(ctx context.Context, username, password string) (*gocloak.JWT, error) {
	// organization:* puts all organization memberships (incl. group roles) into the token;
	// without it Keycloak omits the claim for users in more than one organization.
	token, err := k.client.GetToken(ctx, k.cfg.UserRealm, gocloak.TokenOptions{
		ClientID:  gocloak.StringP(k.cfg.FrontendClientID),
		GrantType: gocloak.StringP("password"),
		Username:  gocloak.StringP(username),
		Password:  gocloak.StringP(password),
		Scope:     gocloak.StringP("openid organization:*"),
	})
	if err != nil {
		return nil, fmt.Errorf("keycloak user login failed: %w", err)
	}
	return token, nil
}

func (k *KeycloakService) GetAllUsers(ctx context.Context, accessToken string) ([]*gocloak.User, error) {
	maxResults := 100
	page := 0
	var allUsers []*gocloak.User
	for {
		users, err := k.client.GetUsers(ctx, accessToken, k.cfg.UserRealm, gocloak.GetUsersParams{
			First: &page,
			Max:   &maxResults,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get users: %w", err)
		}
		allUsers = append(allUsers, users...)
		if len(users) < maxResults {
			break
		}
		page += maxResults
	}
	return allUsers, nil
}

func (k *KeycloakService) GetAllOrganizations(ctx context.Context, accessToken string) ([]*gocloak.OrganizationRepresentation, error) {
	maxResults := 100
	page := 0
	var allOrgs []*gocloak.OrganizationRepresentation
	for {
		orgs, err := k.client.GetOrganizations(ctx, accessToken, k.cfg.UserRealm, gocloak.GetOrganizationsParams{
			First: &page,
			Max:   &maxResults,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get organizations: %w", err)
		}
		allOrgs = append(allOrgs, orgs...)
		if len(orgs) < maxResults {
			break
		}
		page += maxResults
	}
	return allOrgs, nil
}

func (k *KeycloakService) GetOrganizationMembers(ctx context.Context, accessToken, organizationID string) ([]*gocloak.MemberRepresentation, error) {
	maxResults := 100
	page := 0
	var allMembers []*gocloak.MemberRepresentation
	for {
		members, err := k.client.GetOrganizationMembers(ctx, accessToken, k.cfg.UserRealm, organizationID, gocloak.GetMembersParams{
			First: &page,
			Max:   &maxResults,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get organization members: %w", err)
		}
		allMembers = append(allMembers, members...)
		if len(members) < maxResults {
			break
		}
		page += maxResults
	}
	return allMembers, nil
}

func (k *KeycloakService) CreateOrganization(ctx context.Context, accessToken string, org gocloak.OrganizationRepresentation, orgAdmin string) (string, string, error) {
	//check if org exists
	oid, err := k.GetOrganizationIDBySlug(ctx, accessToken, *org.Name)
	if err == nil {
		return oid, "", fmt.Errorf("organization already exists")
	}

	oId, err := k.client.CreateOrganization(ctx, accessToken, k.cfg.UserRealm, org)
	if err != nil {
		return "", "", fmt.Errorf("failed to create organization: %w", err)
	}

	var orgAdminGroupID string
	for _, groupName := range []string{"org_admin", "event_manager", "finance_viewer"} {
		name := groupName
		gId, err := k.client.CreateOrganizationGroup(ctx, accessToken, k.cfg.UserRealm, oId, gocloak.Group{Name: &name})
		if err != nil {
			k.rollbackOrganization(ctx, accessToken, oId)
			return "", "", fmt.Errorf("failed to create organization group %q: %w", groupName, err)
		}
		if groupName == "org_admin" {
			orgAdminGroupID = gId
		}
	}

	userID, err := k.resolveUserID(ctx, accessToken, orgAdmin)
	if err != nil {
		k.rollbackOrganization(ctx, accessToken, oId)
		return "", "", err
	}

	if err := k.client.AddUserToOrganization(ctx, accessToken, k.cfg.UserRealm, oId, userID); err != nil {
		k.rollbackOrganization(ctx, accessToken, oId)
		return "", "", fmt.Errorf("failed to add user to organization: %w", err)
	}

	if err := k.client.AddUserToOrganizationGroup(ctx, accessToken, k.cfg.UserRealm, userID, oId, orgAdminGroupID); err != nil {
		k.rollbackOrganization(ctx, accessToken, oId)
		return "", "", fmt.Errorf("failed to assign user to org_admin group: %w", err)
	}

	return oId, userID, nil
}

func (k *KeycloakService) rollbackOrganization(ctx context.Context, accessToken, orgID string) {
	if err := k.client.DeleteOrganization(ctx, accessToken, k.cfg.UserRealm, orgID); err != nil {
		fmt.Printf("WARNING: failed to rollback organization %s: %v\n", orgID, err)
	}
}

func (k *KeycloakService) GetOrganizationIDBySlug(ctx context.Context, accessToken, slug string) (string, error) {
	pageSize := 50
	page := 0
	for {
		orgs, err := k.client.GetOrganizations(ctx, accessToken, k.cfg.UserRealm,
			gocloak.GetOrganizationsParams{
				First: new(page * pageSize),
				Max:   &pageSize,
			},
		)
		if err != nil {
			return "", fmt.Errorf("failed to get organization %q: %w", orgs, err)
		}
		for _, org := range orgs {
			if strings.Compare(*org.Alias, slug) == 0 {
				return *org.ID, nil
			}
		}
		if len(orgs) < pageSize {
			break
		}
		page++
	}

	return "", fmt.Errorf("organization not found")
}

func (k *KeycloakService) GetOrganizationByID(id string) (*gocloak.OrganizationRepresentation, error) {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return nil, err
	}
	org, err := k.client.GetOrganizationByID(ctx, token.AccessToken, k.cfg.UserRealm, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization %q: %w", org, err)
	}
	return org, nil
}

func (k *KeycloakService) AddUserToOrganization(orgID, userID string) error {
	ctx := context.Background()
	token, err := k.login(ctx)
	if err != nil {
		return err
	}
	return k.client.AddUserToOrganization(ctx, token.AccessToken, k.cfg.UserRealm, orgID, userID)
}

func (k *KeycloakService) resolveUserID(ctx context.Context, accessToken, usernameOrID string) (string, error) {
	user, err := k.GetUserByUsername(ctx, accessToken, k.cfg.UserRealm, usernameOrID)
	if err != nil {
		return "", err
	}
	if user == nil || user.ID == nil {
		return "", fmt.Errorf("user %q not found", usernameOrID)
	}
	return *user.ID, nil
}

func (k *KeycloakService) GetUserByUsername(ctx context.Context, accessToken, realm, username string) (*gocloak.User, error) {
	exact := true
	maxResults := 2
	users, err := k.client.GetUsers(ctx, accessToken, realm, gocloak.GetUsersParams{
		Username: &username,
		Exact:    &exact,
		Max:      &maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to look up user %q: %w", username, err)
	}
	if len(users) == 0 {
		return nil, nil
	}
	if len(users) == 1 {
		return users[0], nil
	}
	return nil, fmt.Errorf("ambiguous user %q: found %d matches", username, len(users))
}

func (k *KeycloakService) CreateUserWithToken(ctx context.Context, accessToken, realm string, user gocloak.User) (string, error) {
	return k.client.CreateUser(ctx, accessToken, realm, user)
}

func (k *KeycloakService) AddUserToOrganizationWithToken(ctx context.Context, accessToken, orgID, userID string) error {
	return k.client.AddUserToOrganization(ctx, accessToken, k.cfg.UserRealm, orgID, userID)
}

func (k *KeycloakService) GetOrganizationGroups(ctx context.Context, accessToken, realm, orgID string) ([]*gocloak.Group, error) {
	maxResults := 100
	groups, err := k.client.GetOrganizationGroups(ctx, accessToken, realm, orgID, gocloak.GetGroupsParams{
		Max: &maxResults,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get org groups: %w", err)
	}
	return groups, nil
}

func (k *KeycloakService) CreateOrganizationGroupWithToken(ctx context.Context, accessToken, realm, orgID string, group gocloak.Group) (string, error) {
	return k.client.CreateOrganizationGroup(ctx, accessToken, realm, orgID, group)
}

func (k *KeycloakService) AssignUserToOrgGroup(ctx context.Context, accessToken, userID, orgID, groupID string) error {
	return k.client.AddUserToOrganizationGroup(ctx, accessToken, k.cfg.UserRealm, userID, orgID, groupID)
}

func (k *KeycloakService) GetOrganizationGroupIDByName(ctx context.Context, accessToken, orgID, name string) (string, error) {
	groups, err := k.GetOrganizationGroups(ctx, accessToken, k.cfg.UserRealm, orgID)
	if err != nil {
		return "", err
	}
	for _, g := range groups {
		if g.Name != nil && *g.Name == name {
			if g.ID != nil {
				return *g.ID, nil
			}
		}
	}
	return "", fmt.Errorf("organization group %q not found", name)
}
