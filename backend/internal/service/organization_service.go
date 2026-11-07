package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/henning-kln/gocloak"
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
	userRepo repository.UserRepository
}

func NewOrganizationService(keycloak *KeycloakService, orgRepo repository.OrganizationRepository, userRepo repository.UserRepository) *OrganizationService {
	return &OrganizationService{
		keycloak: keycloak,
		orgRepo:  orgRepo,
		userRepo: userRepo,
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
// no-op — the requested name is current in both systems — writes no entries.
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
	// Keycloak's PUT replaces organization fields. Read its full representation under the
	// lock so the update preserves the alias, enabled state, domains and other settings.
	org, err := s.keycloak.client.GetOrganizationByID(ctx, token, s.keycloak.cfg.UserRealm, keycloakOrgID)
	if err != nil {
		if isKeycloakNotFound(err) {
			return nil, ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to load organization in Keycloak: %w", err)
	}
	if org == nil || org.ID == nil || org.Alias == nil || org.Name == nil {
		return nil, errors.New("incomplete organization representation from Keycloak")
	}
	change = &OrgNameChange{OrganizationID: keycloakOrgID, Alias: row.Alias, Name: name}
	if row.Name == name && *org.Name == name {
		return change, nil
	}

	trail := changeAudit{
		repo:         s.keycloak.audit,
		meta:         meta,
		orgID:        keycloakOrgID,
		resourceType: audit.ResourceOrganization,
		resourceID:   keycloakOrgID,
		details: model.AuditChanges{
			"nameBefore":         row.Name,
			"keycloakNameBefore": *org.Name,
			"nameAfter":          name,
		},
	}
	if err := trail.record(audit.PhaseStarted, nil); err != nil {
		return nil, fmt.Errorf("%w: %w", audit.ErrUnavailable, err)
	}

	// Keycloak cannot take part in a database transaction. If a step fails after another one
	// already applied, the result is recorded as incomplete and the request stays idempotent and
	// safe to retry: every step sets the requested state, whatever the others did before.
	if *org.Name != name {
		org.Name = &name
		if err := s.keycloak.client.UpdateOrganization(ctx, token, s.keycloak.cfg.UserRealm, *org); err != nil {
			stepErr := fmt.Errorf("failed to set the organization name, the request is safe to retry: %w", err)
			return change, trail.conclude(parentCtx, nameResult(change), "keycloak_update", "", stepErr)
		}
	}
	if row.Name != name {
		if err := s.orgRepo.UpdateName(row.OrganizationID, name); err != nil {
			stepErr := fmt.Errorf("failed to update the organization row, the request is safe to retry: %w", err)
			return change, trail.conclude(parentCtx, nameResult(change), "database_update", "", stepErr)
		}
	}
	return change, trail.conclude(parentCtx, nameResult(change), "", "", nil)
}

// nameResult collects the resulting name for the result entry of the audit trail.
func nameResult(change *OrgNameChange) model.AuditChanges {
	return model.AuditChanges{"name": change.Name}
}

// ErrOrgAdminNotSynced means the organization was created in Keycloak but its admin has no row in
// the API database yet, so the membership cannot be mirrored.
var ErrOrgAdminNotSynced = errors.New("org admin user not found in API database, run sync first")

// CreatedOrganization is the outcome of CreateOrganization.
type CreatedOrganization struct {
	KeycloakOrgID  string
	OrganizationID uuid.UUID
	AdminUserID    uuid.UUID
}

// CreateOrganizationAsService is CreateOrganization with the backend service account, for callers
// whose own token cannot manage Keycloak organizations, e.g. org admins approving an organizer
// application.
func (s *OrganizationService) CreateOrganizationAsService(ctx context.Context, req model.CreateOrganizationRequest) (created *CreatedOrganization, err error) {
	token, err := s.keycloak.adminToken(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { s.keycloak.dropAdminTokenIfRejected(err) }()
	created, err = s.CreateOrganization(ctx, token, req)
	if err != nil {
		return nil, err
	}
	// The caller already reviewed the organization (e.g. approved the organizer application),
	// so it skips the pending state that new organizations start in.
	if _, err := s.orgRepo.Activate(created.OrganizationID); err != nil {
		return nil, fmt.Errorf("organization created but failed to activate it: %w", err)
	}
	return created, nil
}

// CreateOrganization creates the organization in Keycloak with its role groups and the given
// admin (username or email) as org_admin, then mirrors it into the API database together with
// the admin's membership. accessToken must be allowed to manage organizations in Keycloak.
//
// Keycloak just created the organization with a new ID. A row from an earlier Keycloak
// generation may still exist in the database (realm re-imports re-create organizations with new
// IDs); that row is reconciled instead of inserting a duplicate that would split the
// organization's events and memberships across copies.
//
// It returns ErrOrganizationExists when the alias or name is taken and ErrOrgAdminNotSynced when
// the admin is unknown to the API database.
func (s *OrganizationService) CreateOrganization(ctx context.Context, accessToken string, req model.CreateOrganizationRequest) (*CreatedOrganization, error) {
	org := gocloak.OrganizationRepresentation{
		Name:  &req.InternalName,
		Alias: &req.DisplayName,
	}
	keycloakOrgID, adminKeycloakUserID, err := s.keycloak.CreateOrganization(ctx, accessToken, org, req.OrgAdmin)
	if err != nil {
		return nil, err
	}

	adminUser, err := s.userRepo.GetByKeycloakUserID(adminKeycloakUserID)
	if err != nil {
		return nil, fmt.Errorf("organization created in Keycloak but failed to look up admin user in database: %w", err)
	}
	if adminUser == nil {
		return nil, ErrOrgAdminNotSynced
	}

	existing, err := s.orgRepo.GetByAnyRef(keycloakOrgID, req.DisplayName, req.InternalName)
	if err != nil {
		return nil, fmt.Errorf("failed to look up organization in database: %w", err)
	}

	orgUUID := uuid.New()
	if existing != nil {
		orgUUID = existing.OrganizationID
		// The submitted create form is the new truth for the reconciled row: the contact
		// columns move as well, not only the Keycloak-owned identity columns.
		existing.KeycloakOrgID = keycloakOrgID
		existing.Alias = req.DisplayName
		existing.Name = req.InternalName
		existing.ContactEmail = req.ContactEmail
		existing.ContactPhoneNumber = req.ContactPhoneNumber
		existing.Street = req.Street
		existing.HouseNumber = req.HouseNumber
		existing.PostalCode = req.PostalCode
		existing.City = req.City
		existing.CountryCode = req.CountryCode
		if err := s.orgRepo.UpdateOrganization(existing); err != nil {
			return nil, fmt.Errorf("organization created in Keycloak but failed to update the existing database row: %w", err)
		}
	} else {
		err = s.orgRepo.CreateOrganization(&model.OrganizationModel{
			OrganizationID:     orgUUID,
			KeycloakOrgID:      keycloakOrgID,
			Alias:              req.DisplayName,
			Name:               req.InternalName,
			ContactEmail:       req.ContactEmail,
			ContactPhoneNumber: req.ContactPhoneNumber,
			Street:             req.Street,
			HouseNumber:        req.HouseNumber,
			PostalCode:         req.PostalCode,
			City:               req.City,
			CountryCode:        req.CountryCode,
		})
		if err != nil {
			return nil, fmt.Errorf("organization created in Keycloak but failed to insert into database: %w", err)
		}
	}

	err = s.orgRepo.AddMembership(&model.OrganizationMembershipModel{
		UserID:         adminUser.UserID,
		OrganizationID: orgUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("organization created in Keycloak and database but failed to add membership: %w", err)
	}
	return &CreatedOrganization{KeycloakOrgID: keycloakOrgID, OrganizationID: orgUUID, AdminUserID: adminUser.UserID}, nil
}
