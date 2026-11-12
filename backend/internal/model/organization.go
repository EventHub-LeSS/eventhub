package model

import (
	"time"
)

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

// OrganizationMemberResponse is one member of an organization with their roles in it.
type OrganizationMemberResponse struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	JoinedAt *string  `json:"joinedAt"`
	Rights   []string `json:"rights"`
}

type UpdateOrganizationRequest struct {
	// Name is the display name of the organization ("Organisationsdetails"); it is required and
	// must not be empty after trimming.
	Name string `json:"name" binding:"required"`
	// Alias is the Keycloak-owned identity reference of the organization and deliberately not
	// changeable: the token claim carries it, so every member would have to refresh their token
	// and every stored reference would drift. The field is optional so an unchanged value is
	// ignored, while an attempt to change it is rejected with 400 instead of being dropped
	// silently.
	Alias *string `json:"alias,omitempty"`
}

type UpdateOrganizationResponse struct {
	ID      string `json:"id"`
	Alias   string `json:"alias"`
	Name    string `json:"name"`
	Message string `json:"message,omitempty"`
}

// OrganizationDetailsResponse is the response of GET /organizations/{organizationID}: the details
// of one organization, loaded from the API database. id is the Keycloak organization ID, which
// roles and audit entries are addressed by, and organizationId is the database UUID events
// reference in their organizer_id. The contact columns belong to the organization details and are
// returned as they are stored, null when never set.
type OrganizationDetailsResponse struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organizationId"`
	Alias              string    `json:"alias"`
	Name               string    `json:"name"`
	ContactEmail       *string   `json:"contactEmail"`
	ContactPhoneNumber *string   `json:"contactPhoneNumber"`
	Street             *string   `json:"street"`
	HouseNumber        *string   `json:"houseNumber"`
	PostalCode         *string   `json:"postalCode"`
	City               *string   `json:"city"`
	CountryCode        *string   `json:"countryCode"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type ErrorResponse struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}
