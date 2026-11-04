package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
)

// OrgNameChange is the outcome of an organization name change: the resolved organization and the
// resulting name.
type OrgNameChange struct {
	OrganizationID string
	Alias          string
	Name           string
}

// ErrOrganizationNotInDatabase is returned when an organization exists in Keycloak but has no row
// in the API database: details cannot be read or changed against an unknown row.
var ErrOrganizationNotInDatabase = errors.New("organization is not synchronized in the API database")

// OrganizationService changes the details of an organization. The change spans Keycloak, which
// owns the organization identity, and the mirrored database row; one transaction cannot cover
// both, so the service follows the start/result audit protocol like role changes (EVENTHUB-188).
type OrganizationService struct {
	keycloak *KeycloakService
	orgRepo  repository.OrganizationRepository
}

func NewOrganizationService(keycloak *KeycloakService, orgRepo repository.OrganizationRepository) *OrganizationService {
	return &OrganizationService{
		keycloak: keycloak,
		orgRepo:  orgRepo,
	}
}

// UpdateOrganizationName sets the display name of the organization: Keycloak first with the
// backend service account, then the mirrored database row, so a member reads the new details only
// once the whole change succeeded. Keycloak keeps the alias; the token claim carries it, so
// changing it would break every member's token.
//
// An admin who is no global admin is checked again against Keycloak under the organization lock,
// so an admin demoted in the meantime cannot act on a token that still carries the old role.
// Keycloak calls are bounded by orgChangeTimeout (EVENTHUB-188).
//
// Effective changes are written to the audit log (audit.Meta from ctx is required): a "started"
// entry before the first change and a result entry afterwards, sharing one operation ID. A
// no-op — the requested name is already current — writes no entries.
func (s *OrganizationService) UpdateOrganizationName(ctx context.Context, keycloakOrgID, name string, actor OrgRoleActor) (change *OrgNameChange, err error) {
	parentCtx := ctx
	meta, err := audit.MetaFromContext(ctx, audit.OrganizationUpdated)
	if err != nil {
		return nil, err
	}
	if s.keycloak.audit == nil {
		return nil, fmt.Errorf("%w: no audit log configured", audit.ErrUnavailable)
	}
	ctx, cancel := context.WithTimeout(ctx, orgChangeTimeout)
	defer cancel()
	adminRole := string(model.RoleOrganizationAdmin)

	token, adminErr := s.keycloak.adminToken(ctx)
	if adminErr != nil {
		return nil, adminErr
	}
	defer func() { s.keycloak.dropAdminTokenIfRejected(err) }()

	unlock := lockOrganization(keycloakOrgID)
	defer unlock()

	groupIDByName, err := s.keycloak.organizationRoleGroupIDs(ctx, token, keycloakOrgID)
	if err != nil {
		return nil, err
	}
	if !actor.GlobalAdmin {
		isAdmin, err := s.keycloak.isOrganizationGroupMember(ctx, token, keycloakOrgID, groupIDByName[adminRole], actor.UserID)
		if err != nil {
			return nil, err
		}
		if !isAdmin {
			return nil, ErrActorNotOrgAdmin
		}
	}

	row, err := s.orgRepo.GetByKeycloakOrgID(keycloakOrgID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrOrganizationNotInDatabase
	}
	change = &OrgNameChange{OrganizationID: keycloakOrgID, Alias: row.Alias, Name: name}
	if row.Name == name {
		return change, nil
	}

	trail := changeAudit{
		repo:         s.keycloak.audit,
		meta:         meta,
		orgID:        keycloakOrgID,
		resourceType: audit.ResourceOrganization,
		resourceID:   keycloakOrgID,
		details: model.AuditChanges{
			"nameBefore": row.Name,
			"nameAfter":  name,
		},
	}
	if err := trail.record(audit.PhaseStarted, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", audit.ErrUnavailable, err)
	}

	// Keycloak cannot take part in a database transaction. If a step fails after another one
	// already applied, the result is recorded as incomplete and the request stays idempotent and
	// safe to retry: every step sets the requested state, whatever the others did before.
	if err := s.keycloak.updateOrganizationName(ctx, keycloakOrgID, name); err != nil {
		stepErr := fmt.Errorf("failed to set the organization name, the request is safe to retry: %w", err)
		return change, trail.conclude(parentCtx, nameResult(change), "keycloak_update", "", stepErr)
	}
	if err := s.orgRepo.UpdateName(row.OrganizationID, name); err != nil {
		stepErr := fmt.Errorf("failed to update the organization row, the request is safe to retry: %w", err)
		return change, trail.conclude(parentCtx, nameResult(change), "database_update", "", stepErr)
	}
	return change, trail.conclude(parentCtx, nameResult(change), "", "", nil)
}

// nameResult collects the resulting name for the result entry of the audit trail.
func nameResult(change *OrgNameChange) model.AuditChanges {
	return model.AuditChanges{"name": change.Name}
}
