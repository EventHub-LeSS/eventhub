package middleware

import "time"

type GlobalRole string

const (
	RoleAdmin     GlobalRole = "admin"
	RoleModerator GlobalRole = "moderator"
	RoleVisitor   GlobalRole = "visitor"
)

type OrganizationRole string

const (
	RoleOrganizationAdmin OrganizationRole = "org_admin"
	RoleEventManager      OrganizationRole = "event_manager"
	RoleFinanceViewer     OrganizationRole = "finance_viewer"
)

type OrganizationAccess struct {
	ID    string
	Alias string
	Roles map[OrganizationRole]struct{}
}

type Principal struct {
	Subject            string
	Username           string
	AuthorizedParty    string
	GlobalRoles        map[GlobalRole]struct{}
	ActiveOrganization *OrganizationAccess
	ExpiresAt          time.Time
}

func (p *Principal) HasGlobalRole(role GlobalRole) bool {
	if p == nil {
		return false
	}
	_, ok := p.GlobalRoles[role]
	return ok
}

func (p *Principal) HasOrganizationRole(role OrganizationRole) bool {
	if p == nil || p.ActiveOrganization == nil {
		return false
	}
	_, ok := p.ActiveOrganization.Roles[role]
	return ok
}
