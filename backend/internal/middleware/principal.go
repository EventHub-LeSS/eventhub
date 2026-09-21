package middleware

import (
	"time"

	"backend/internal/model"
)

type GlobalRole string

const (
	RoleAdmin     GlobalRole = "admin"
	RoleModerator GlobalRole = "moderator"
	RoleVisitor   GlobalRole = "visitor"
)

// Organization roles are defined in model (EVENTHUB-188); the aliases keep the middleware API.
type OrganizationRole = model.OrganizationRole

const (
	RoleOrganizationAdmin = model.RoleOrganizationAdmin
	RoleEventManager      = model.RoleEventManager
	RoleFinanceViewer     = model.RoleFinanceViewer
)

var OrganizationRoles = model.OrganizationRoles

type OrganizationAccess struct {
	ID    string
	Alias string
	Roles map[OrganizationRole]struct{}
}

type Principal struct {
	Subject            string
	Username           string
	AuthorizedParty    string
	AccessToken        string
	GlobalRoles        map[GlobalRole]struct{}
	ActiveOrganization *OrganizationAccess
	Organizations      []*OrganizationAccess
	ExpiresAt          time.Time
}

func (p *Principal) HasGlobalRole(role GlobalRole) bool {
	if p == nil {
		return false
	}
	_, ok := p.GlobalRoles[role]
	return ok
}

// HasOrganizationRole reports whether the principal holds the role in any of its organizations.
func (p *Principal) HasOrganizationRole(role OrganizationRole) bool {
	return len(p.OrganizationIDsWithRole(role)) > 0
}

// HasOrganizationRoleIn reports whether the principal holds the role in the given organization.
func (p *Principal) HasOrganizationRoleIn(organizationID string, role OrganizationRole) bool {
	if p == nil {
		return false
	}
	for _, org := range p.Organizations {
		if org != nil && org.ID == organizationID {
			_, ok := org.Roles[role]
			return ok
		}
	}
	return false
}

// OrganizationIDsWithRole returns the IDs of all organizations in which the principal holds the role.
func (p *Principal) OrganizationIDsWithRole(role OrganizationRole) []string {
	if p == nil {
		return nil
	}
	ids := make([]string, 0, len(p.Organizations))
	for _, org := range p.Organizations {
		if org == nil {
			continue
		}
		if _, ok := org.Roles[role]; ok {
			ids = append(ids, org.ID)
		}
	}
	return ids
}
