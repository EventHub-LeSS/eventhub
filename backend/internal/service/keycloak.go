package service

import (
	"backend/internal/middleware"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
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

	// Cached service-account token for admin operations (EVENTHUB-188).
	adminTokenMu      sync.Mutex
	adminTokenValue   string
	adminTokenExpires time.Time
}

func NewKeycloakService(cfg KeycloakClientConfig) *KeycloakService {
	return &KeycloakService{
		cfg:    cfg,
		client: gocloak.NewClient(cfg.Host),
	}
}

// adminToken returns the service-account token for admin operations, caching it
// until shortly before it expires instead of logging in on every call.
func (k *KeycloakService) adminToken(ctx context.Context) (string, error) {
	k.adminTokenMu.Lock()
	defer k.adminTokenMu.Unlock()
	if k.adminTokenValue != "" && time.Now().Before(k.adminTokenExpires) {
		return k.adminTokenValue, nil
	}
	token, err := k.client.LoginClient(ctx, k.cfg.ClientID, k.cfg.ClientSecret, k.cfg.AdminRealm)
	if err != nil {
		return "", fmt.Errorf("keycloak admin login failed: %w", err)
	}
	const expirySkew = 30 * time.Second
	k.adminTokenValue = token.AccessToken
	k.adminTokenExpires = time.Now().Add(time.Duration(token.ExpiresIn)*time.Second - expirySkew)
	return token.AccessToken, nil
}

func (k *KeycloakService) LoginUser(ctx context.Context, username, password string) (*gocloak.JWT, error) {
	token, err := k.client.Login(ctx, k.cfg.FrontendClientID, "", k.cfg.UserRealm, username, password)
	if err != nil {
		return nil, fmt.Errorf("keycloak user login failed: %w", err)
	}
	return token, nil
}

// LoginUserWithOrganizations logs a user in like LoginUser, but requests the organization:*
// scope so the access token carries all organization memberships and their group roles.
// Keycloak omits the claim for users in more than one organization unless all are requested.
func (k *KeycloakService) LoginUserWithOrganizations(ctx context.Context, username, password string) (*gocloak.JWT, error) {
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

const organizationMembershipMapper = "oidc-organization-membership-mapper"

// EnsureOrganizationClaimName sets claim.name on the organization membership mapper of the
// "organization" client scope if it is missing. Without it Keycloak throws a
// NullPointerException for every token request with an organization scope. Realms imported
// before the fix keep the broken mapper, because --import-realm skips existing realms.
// The mapper is read and written as raw JSON so that no config keys get lost.
// Reports whether a mapper was updated.
func (k *KeycloakService) EnsureOrganizationClaimName(ctx context.Context, accessToken, realm string) (bool, error) {
	scopesURL := strings.TrimRight(k.cfg.Host, "/") + "/admin/realms/" + realm + "/client-scopes"

	var scopes []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := k.adminGetJSON(ctx, accessToken, scopesURL, &scopes); err != nil {
		return false, fmt.Errorf("list client scopes: %w", err)
	}
	scopeID := ""
	for _, scope := range scopes {
		if scope.Name == "organization" {
			scopeID = scope.ID
			break
		}
	}
	if scopeID == "" {
		return false, nil
	}

	// Read the scope itself: the .../protocol-mappers/models endpoints fill in claim.name as a
	// default even when it is not stored, which would hide exactly the broken mapper.
	var scope struct {
		ProtocolMappers []map[string]any `json:"protocolMappers"`
	}
	if err := k.adminGetJSON(ctx, accessToken, scopesURL+"/"+scopeID, &scope); err != nil {
		return false, fmt.Errorf("read organization client scope: %w", err)
	}
	mappersURL := scopesURL + "/" + scopeID + "/protocol-mappers/models"
	mappers := scope.ProtocolMappers

	updated := false
	for _, mapper := range mappers {
		if mapper["protocolMapper"] != organizationMembershipMapper {
			continue
		}
		config, _ := mapper["config"].(map[string]any)
		if config == nil {
			config = map[string]any{}
			mapper["config"] = config
		}
		if name, _ := config["claim.name"].(string); name != "" {
			continue
		}
		config["claim.name"] = "organization"
		id, _ := mapper["id"].(string)
		resp, err := k.client.GetRequestWithBearerAuth(ctx, accessToken).
			SetBody(mapper).
			Put(mappersURL + "/" + id)
		if err := adminResponseError(resp, err); err != nil {
			return updated, fmt.Errorf("update organization membership mapper: %w", err)
		}
		updated = true
	}
	return updated, nil
}

func (k *KeycloakService) adminGetJSON(ctx context.Context, accessToken, url string, target any) error {
	resp, err := k.client.GetRequestWithBearerAuth(ctx, accessToken).Get(url)
	if err := adminResponseError(resp, err); err != nil {
		return err
	}
	return json.Unmarshal(resp.Body(), target)
}

func adminResponseError(resp *resty.Response, err error) error {
	if err != nil {
		return err
	}
	if resp.IsError() {
		return fmt.Errorf("%s: %s", resp.Status(), resp.String())
	}
	return nil
}

// ServiceAccountRoles are the realm-management roles the backend service account needs to
// configure organization member roles (EVENTHUB-188): manage-organizations to change org group
// memberships, manage-users to look up users and write the memberships. realm-admin is not needed.
var ServiceAccountRoles = []string{"manage-organizations", "manage-users"}

// EnsureServiceAccount enables the service account of the configured backend client and grants it
// ServiceAccountRoles, both as role mapping and as scope mapping: the client has
// fullScopeAllowed=false, so without the scope mapping the roles never reach the token. Realms
// imported before the service account was added stay without it, because --import-realm skips
// existing realms. Clients are read and written as raw JSON so that no attributes get lost.
// Reports whether anything was changed.
func (k *KeycloakService) EnsureServiceAccount(ctx context.Context, accessToken, realm string) (bool, error) {
	adminURL := strings.TrimRight(k.cfg.Host, "/") + "/admin/realms/" + realm

	client, err := k.adminFindClient(ctx, accessToken, adminURL, k.cfg.ClientID)
	if err != nil {
		return false, err
	}
	if client == nil {
		return false, fmt.Errorf("client %q not found", k.cfg.ClientID)
	}
	clientID, _ := client["id"].(string)

	updated := false
	if enabled, _ := client["serviceAccountsEnabled"].(bool); !enabled {
		client["serviceAccountsEnabled"] = true
		resp, err := k.client.GetRequestWithBearerAuth(ctx, accessToken).SetBody(client).Put(adminURL + "/clients/" + clientID)
		if err := adminResponseError(resp, err); err != nil {
			return false, fmt.Errorf("enable service account: %w", err)
		}
		updated = true
	}

	realmManagement, err := k.adminFindClient(ctx, accessToken, adminURL, "realm-management")
	if err != nil {
		return updated, err
	}
	if realmManagement == nil {
		return updated, fmt.Errorf("client %q not found", "realm-management")
	}
	realmManagementID, _ := realmManagement["id"].(string)

	var serviceAccountUser struct {
		ID string `json:"id"`
	}
	if err := k.adminGetJSON(ctx, accessToken, adminURL+"/clients/"+clientID+"/service-account-user", &serviceAccountUser); err != nil {
		return updated, fmt.Errorf("read service account user: %w", err)
	}

	var roles []map[string]any
	if err := k.adminGetJSON(ctx, accessToken, adminURL+"/clients/"+realmManagementID+"/roles", &roles); err != nil {
		return updated, fmt.Errorf("list realm-management roles: %w", err)
	}

	for _, mapping := range []struct{ name, url string }{
		{"role mapping", adminURL + "/users/" + serviceAccountUser.ID + "/role-mappings/clients/" + realmManagementID},
		{"scope mapping", adminURL + "/clients/" + clientID + "/scope-mappings/clients/" + realmManagementID},
	} {
		added, err := k.adminAddMissingRoles(ctx, accessToken, mapping.url, roles, ServiceAccountRoles)
		if err != nil {
			return updated, fmt.Errorf("service account %s: %w", mapping.name, err)
		}
		updated = updated || added
	}
	return updated, nil
}

// adminFindClient returns the raw representation of the client with the given clientId, or nil.
func (k *KeycloakService) adminFindClient(ctx context.Context, accessToken, adminURL, clientID string) (map[string]any, error) {
	var clients []map[string]any
	if err := k.adminGetJSON(ctx, accessToken, adminURL+"/clients?clientId="+url.QueryEscape(clientID), &clients); err != nil {
		return nil, fmt.Errorf("look up client %q: %w", clientID, err)
	}
	for _, client := range clients {
		if client["clientId"] == clientID {
			return client, nil
		}
	}
	return nil, nil
}

// adminAddMissingRoles adds the wanted roles to a role or scope mapping endpoint unless they are
// already mapped. available are the client's role representations the wanted names refer to.
func (k *KeycloakService) adminAddMissingRoles(ctx context.Context, accessToken, mappingURL string, available []map[string]any, wanted []string) (bool, error) {
	var mapped []map[string]any
	if err := k.adminGetJSON(ctx, accessToken, mappingURL, &mapped); err != nil {
		return false, err
	}
	have := make(map[string]bool, len(mapped))
	for _, role := range mapped {
		if name, ok := role["name"].(string); ok {
			have[name] = true
		}
	}
	byName := make(map[string]map[string]any, len(available))
	for _, role := range available {
		if name, ok := role["name"].(string); ok {
			byName[name] = role
		}
	}
	var missing []map[string]any
	for _, name := range wanted {
		if have[name] {
			continue
		}
		role, ok := byName[name]
		if !ok {
			return false, fmt.Errorf("role %q not found", name)
		}
		missing = append(missing, role)
	}
	if len(missing) == 0 {
		return false, nil
	}
	resp, err := k.client.GetRequestWithBearerAuth(ctx, accessToken).SetBody(missing).Post(mappingURL)
	if err := adminResponseError(resp, err); err != nil {
		return false, err
	}
	return true, nil
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
	// Only proceed to creation when the organization is known to not exist;
	// an infrastructure error must not be mistaken for "does not exist".
	oid, err := k.GetOrganizationIDBySlug(ctx, accessToken, *org.Name)
	switch {
	case err == nil:
		return oid, "", fmt.Errorf("organization already exists")
	case errors.Is(err, ErrOrganizationNotFound):
	default:
		return "", "", fmt.Errorf("failed to check whether the organization exists: %w", err)
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
			return "", fmt.Errorf("failed to list organizations: %w", err)
		}
		for _, org := range orgs {
			if org.Alias != nil && *org.Alias == slug && org.ID != nil {
				return *org.ID, nil
			}
		}
		if len(orgs) < pageSize {
			break
		}
		page++
	}

	return "", ErrOrganizationNotFound
}

func (k *KeycloakService) GetOrganizationByID(id string) (*gocloak.OrganizationRepresentation, error) {
	ctx := context.Background()
	token, err := k.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	org, err := k.client.GetOrganizationByID(ctx, token, k.cfg.UserRealm, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization %q: %w", org, err)
	}
	return org, nil
}

func (k *KeycloakService) AddUserToOrganization(orgID, userID string) error {
	ctx := context.Background()
	token, err := k.adminToken(ctx)
	if err != nil {
		return err
	}
	return k.client.AddUserToOrganization(ctx, token, k.cfg.UserRealm, orgID, userID)
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

// EVENTHUB-188: errors returned by ConfigureOrganizationMemberRoles.
var (
	ErrUserNotFound         = errors.New("user not found")
	ErrOrganizationNotFound = errors.New("organization not found")
	ErrNotAMember           = errors.New("user is not a member of the organization")
	ErrOrgGroupsMissing     = errors.New("organization role groups are not initialized")
	ErrLastAdmin            = errors.New("cannot remove the last organization admin")
)

// OrgRoleChange is the outcome of a role configuration: the resolved organization,
// which roles were granted and revoked, and the resulting role set.
type OrgRoleChange struct {
	OrganizationID string
	Granted        []string
	Revoked        []string
	Applied        []string
}

// orgMutexes serializes role changes per organization so that concurrent calls
// cannot interleave (e.g. two admins demoting each other between the last-admin
// guard check and the writes). It only works because the API runs as a single
// instance; a global platform admin can repair organizations through the same
// endpoint if roles ever end up inconsistent.
var orgMutexes sync.Map // map[string]*sync.Mutex

func lockOrganization(orgID string) func() {
	value, _ := orgMutexes.LoadOrStore(orgID, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ConfigureOrganizationMemberRoles replaces the organization roles of a user with the requested
// set; an empty set strips all roles. It refuses to remove the last organization admin so an
// organization cannot lock itself out. The member's current roles are resolved by scanning the
// member lists of the organization's role groups: Keycloak hides organization groups from the
// per-user groups endpoint (GET /users/{id}/groups filters them out by group type), so the admin
// API offers no per-user view of org group membership. The admin guard is only evaluated, capped
// at two entries, when an admin is actually demoted. Revokes run before grants so a failure
// mid-sequence can only leave the member with fewer roles than intended, never with the union of
// old and new roles; the request is idempotent and safe to retry. Keycloak calls run with the
// backend service account; callers must enforce that only organization admins may use it
// (EVENTHUB-188).
func (k *KeycloakService) ConfigureOrganizationMemberRoles(ctx context.Context, keycloakOrgID, username string, requestedRoles []string) (*OrgRoleChange, error) {
	orgAdminRole := string(middleware.RoleOrganizationAdmin)

	token, err := k.adminToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := k.GetUserByUsername(ctx, token, k.cfg.UserRealm, username)
	if err != nil {
		return nil, err
	}
	if user == nil || user.ID == nil {
		return nil, ErrUserNotFound
	}
	userID := *user.ID

	// Tokens may identify organizations by alias when the organization claim
	// carries no ID, so the parameter is resolved to the Keycloak org ID.
	// Authorization happened on the raw parameter before this point.
	keycloakOrgID, err = k.resolveOrganizationID(ctx, token, keycloakOrgID)
	if err != nil {
		return nil, err
	}

	unlock := lockOrganization(keycloakOrgID)
	defer unlock()

	groupIDByName, err := k.organizationRoleGroupIDs(ctx, token, keycloakOrgID)
	if err != nil {
		return nil, err
	}

	if _, err := k.client.GetOrganizationMemberByID(ctx, token, k.cfg.UserRealm, keycloakOrgID, userID); err != nil {
		if isKeycloakNotFound(err) {
			return nil, ErrNotAMember
		}
		return nil, fmt.Errorf("failed to check organization membership: %w", err)
	}

	currentRoles, err := k.currentUserRoles(ctx, token, keycloakOrgID, userID, groupIDByName)
	if err != nil {
		return nil, err
	}

	requested := make(map[string]bool, len(requestedRoles))
	for _, role := range requestedRoles {
		requested[role] = true
	}

	if currentRoles[orgAdminRole] && !requested[orgAdminRole] {
		other, err := k.hasOtherAdmin(ctx, token, keycloakOrgID, groupIDByName[orgAdminRole], userID)
		if err != nil {
			return nil, err
		}
		if !other {
			return nil, ErrLastAdmin
		}
	}

	change := &OrgRoleChange{OrganizationID: keycloakOrgID}
	// Two separate loops on purpose: all revokes complete before the first
	// grant, so a failure can never leave the union of old and new roles.
	for _, role := range middleware.OrganizationRoles {
		name := string(role)
		if !requested[name] && currentRoles[name] {
			if err := k.client.DeleteUserFromOrganizationGroup(ctx, token, k.cfg.UserRealm, userID, keycloakOrgID, groupIDByName[name]); err != nil {
				return nil, fmt.Errorf("failed to revoke role %q, the request is safe to retry: %w", name, err)
			}
			change.Revoked = append(change.Revoked, name)
		}
	}
	for _, role := range middleware.OrganizationRoles {
		name := string(role)
		if requested[name] && !currentRoles[name] {
			if err := k.client.AddUserToOrganizationGroup(ctx, token, k.cfg.UserRealm, userID, keycloakOrgID, groupIDByName[name]); err != nil {
				return nil, fmt.Errorf("failed to grant role %q, the request is safe to retry: %w", name, err)
			}
			change.Granted = append(change.Granted, name)
		}
	}

	change.Applied = make([]string, 0, len(middleware.OrganizationRoles))
	for _, role := range middleware.OrganizationRoles {
		if requested[string(role)] {
			change.Applied = append(change.Applied, string(role))
		}
	}
	return change, nil
}

// currentUserRoles returns which of the organization's role groups the user belongs to by
// scanning the member list of every role group. The per-user groups endpoint cannot be used
// here: Keycloak filters organization groups out of GET /users/{id}/groups by group type, so a
// member of org role groups gets an empty list there; the member listings of the org groups are
// the only view the admin API offers, and the gocloak fork has no per-user function for it
// either (EVENTHUB-188).
func (k *KeycloakService) currentUserRoles(ctx context.Context, accessToken, keycloakOrgID, userID string, groupIDByName map[string]string) (map[string]bool, error) {
	current := make(map[string]bool, len(middleware.OrganizationRoles))
	for _, role := range middleware.OrganizationRoles {
		name := string(role)
		memberIDs, err := k.getOrganizationGroupMemberIDs(ctx, accessToken, keycloakOrgID, groupIDByName[name])
		if err != nil {
			return nil, err
		}
		for _, memberID := range memberIDs {
			if memberID == userID {
				current[name] = true
			}
		}
	}
	return current, nil
}

// getOrganizationGroupMemberIDs returns the IDs of all members of an organization group.
func (k *KeycloakService) getOrganizationGroupMemberIDs(ctx context.Context, accessToken, keycloakOrgID, groupID string) ([]string, error) {
	maxResults := 100
	page := 0
	var memberIDs []string
	for {
		users, err := k.client.GetOrganizationGroupMembers(ctx, accessToken, k.cfg.UserRealm, keycloakOrgID, groupID, gocloak.GetGroupsParams{
			First: &page,
			Max:   &maxResults,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get members of organization group %q: %w", groupID, err)
		}
		for _, u := range users {
			if u.ID != nil {
				memberIDs = append(memberIDs, *u.ID)
			}
		}
		if len(users) < maxResults {
			break
		}
		page += maxResults
	}
	return memberIDs, nil
}

// hasOtherAdmin reports whether anyone other than userID belongs to the
// organization admin group. Keycloak offers no per-user view of a group, so the
// member listing is fetched capped at two entries: a single other admin is
// enough to allow the demotion (EVENTHUB-188).
func (k *KeycloakService) hasOtherAdmin(ctx context.Context, accessToken, keycloakOrgID, adminGroupID, userID string) (bool, error) {
	maxResults := 2
	members, err := k.client.GetOrganizationGroupMembers(ctx, accessToken, k.cfg.UserRealm, keycloakOrgID, adminGroupID, gocloak.GetGroupsParams{
		Max: &maxResults,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get members of organization group %q: %w", adminGroupID, err)
	}
	for _, member := range members {
		if member.ID != nil && *member.ID != userID {
			return true, nil
		}
	}
	return false, nil
}

// resolveOrganizationID accepts a Keycloak organization ID or alias and returns the ID.
func (k *KeycloakService) resolveOrganizationID(ctx context.Context, accessToken, organizationIDOrAlias string) (string, error) {
	org, err := k.client.GetOrganizationByID(ctx, accessToken, k.cfg.UserRealm, organizationIDOrAlias)
	if err == nil {
		if org != nil && org.ID != nil {
			return *org.ID, nil
		}
	} else if !isKeycloakNotFound(err) {
		return "", fmt.Errorf("failed to look up organization: %w", err)
	}
	id, err := k.GetOrganizationIDBySlug(ctx, accessToken, organizationIDOrAlias)
	if err != nil {
		if errors.Is(err, ErrOrganizationNotFound) {
			return "", ErrOrganizationNotFound
		}
		return "", fmt.Errorf("failed to look up organization by alias: %w", err)
	}
	return id, nil
}

// organizationRoleGroupIDs maps the canonical role group names to their Keycloak group IDs.
func (k *KeycloakService) organizationRoleGroupIDs(ctx context.Context, accessToken, keycloakOrgID string) (map[string]string, error) {
	groups, err := k.GetOrganizationGroups(ctx, accessToken, k.cfg.UserRealm, keycloakOrgID)
	if err != nil {
		if isKeycloakNotFound(err) {
			return nil, ErrOrganizationNotFound
		}
		return nil, err
	}
	groupIDByName := make(map[string]string, len(middleware.OrganizationRoles))
	for _, g := range groups {
		if g.Name == nil || g.ID == nil {
			continue
		}
		groupIDByName[*g.Name] = *g.ID
	}
	for _, role := range middleware.OrganizationRoles {
		if _, ok := groupIDByName[string(role)]; !ok {
			return nil, fmt.Errorf("%w: missing group %q", ErrOrgGroupsMissing, role)
		}
	}
	return groupIDByName, nil
}

func isKeycloakNotFound(err error) bool {
	var apiErr *gocloak.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Code == http.StatusNotFound
	}
	return false
}
