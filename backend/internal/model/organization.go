package model

// OrganizationRole is a per-organization role, stored in Keycloak as an organization group of the
// same name (EVENTHUB-188). It lives in model so request models and services can use it without
// importing middleware.
type OrganizationRole string

const (
	RoleOrganizationAdmin OrganizationRole = "org_admin"
	RoleEventManager      OrganizationRole = "event_manager"
	RoleFinanceViewer     OrganizationRole = "finance_viewer"
)

// OrganizationRoles is the single source of truth for the per-organization
// roles in canonical order. Request validation, token claim mapping and the
// Keycloak role group management all derive from it (EVENTHUB-188).
var OrganizationRoles = []OrganizationRole{
	RoleOrganizationAdmin,
	RoleEventManager,
	RoleFinanceViewer,
}

// IsValidOrganizationRole reports whether name is one of the canonical organization roles.
func IsValidOrganizationRole(name string) bool {
	for _, role := range OrganizationRoles {
		if string(role) == name {
			return true
		}
	}
	return false
}

// OrganizationRoleNames returns the canonical role names in order.
func OrganizationRoleNames() []string {
	names := make([]string, 0, len(OrganizationRoles))
	for _, role := range OrganizationRoles {
		names = append(names, string(role))
	}
	return names
}

type CreateOrganizationRequest struct {
	InternalName       string  `json:"name" binding:"required"`
	DisplayName        string  `json:"alias" binding:"required"`
	OrgAdmin           string  `json:"orgAdmin" binding:"required"`
	ContactEmail       *string `json:"contactEmail,omitempty"`
	ContactPhoneNumber *string `json:"contactPhoneNumber,omitempty"`
	Street             *string `json:"street,omitempty"`
	HouseNumber        *string `json:"houseNumber,omitempty"`
	PostalCode         *string `json:"postalCode,omitempty"`
	City               *string `json:"city,omitempty"`
	CountryCode        *string `json:"countryCode,omitempty"`
}

type CreateOrganizationResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Alias   string `json:"alias"`
	Message string `json:"message,omitempty"`
}

// EVENTHUB-188: ConfigureOrgRolesRequest is the complete role set of an organization member.
// roles is required so a missing or misspelled field cannot strip all roles; an explicit empty
// array does, except for the last organization admin. Role names are checked by the org_role
// binding validator registered by the handler package; OrganizationRoles is the single source of truth.
type ConfigureOrgRolesRequest struct {
	Roles []OrganizationRole `json:"roles" binding:"required,unique,dive,org_role"`
}

type ConfigureOrgRolesResponse struct {
	Username       string   `json:"username"`
	OrganizationID string   `json:"organizationId"`
	Roles          []string `json:"roles"`
	Message        string   `json:"message,omitempty"`
}

type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}
