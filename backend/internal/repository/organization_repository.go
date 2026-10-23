package repository

import (
	"backend/internal/model"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrganizationRepository interface {
	CreateOrganization(org *model.OrganizationModel) error
	GetByID(organizationID uuid.UUID) (*model.OrganizationModel, error)
	GetByKeycloakOrgID(keycloakOrgID string) (*model.OrganizationModel, error)
	// GetByAnyRef returns the organization referenced by its Keycloak organization ID, its alias
	// or its name. Keycloak keeps all three unique across organizations, so at most one row
	// matches; matching by alias or name survives realm re-imports, which change the ID but
	// keep alias and name. Alias and name match case-insensitively, mirroring the unique
	// indexes of migration 000011; the Keycloak ID matches exactly.
	GetByAnyRef(keycloakOrgID, alias, name string) (*model.OrganizationModel, error)
	// UpdateKeycloakRefs follows an organization that Keycloak re-created with a new ID, e.g.
	// after a realm re-import: the database keeps its single row and only the Keycloak-owned
	// identity columns move. An empty alias or name leaves the column unchanged.
	UpdateKeycloakRefs(organizationID uuid.UUID, keycloakOrgID, alias, name string) error
	// UpdateOrganization persists every column of the given organization row. Create uses it to
	// reconcile a row from an earlier Keycloak generation: the submitted request is the new
	// truth for the row, including its contact columns.
	UpdateOrganization(org *model.OrganizationModel) error
	// ListByKeycloakOrgIDsOrAliases returns the organizations whose Keycloak ID or alias is in refs.
	// Tokens identify organizations by alias when the claim carries no ID.
	ListByKeycloakOrgIDsOrAliases(refs []string) ([]*model.OrganizationModel, error)
	AddMembership(membership *model.OrganizationMembershipModel) error
}

type organizationRepository struct {
	db *gorm.DB
}

func NewOrganizationRepository(db *gorm.DB) OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) CreateOrganization(org *model.OrganizationModel) error {
	return r.db.Create(org).Error
}

func (r *organizationRepository) GetByID(organizationID uuid.UUID) (*model.OrganizationModel, error) {
	org := &model.OrganizationModel{}
	err := r.db.First(org, "organization_id = ?", organizationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (r *organizationRepository) GetByKeycloakOrgID(keycloakOrgID string) (*model.OrganizationModel, error) {
	org := &model.OrganizationModel{}
	err := r.db.Where("keycloak_org_id = ?", keycloakOrgID).First(org).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (r *organizationRepository) GetByAnyRef(keycloakOrgID, alias, name string) (*model.OrganizationModel, error) {
	org := &model.OrganizationModel{}
	err := r.db.Where(
		"keycloak_org_id = ? OR (COALESCE(alias, '') <> '' AND LOWER(alias) = LOWER(?)) OR (COALESCE(name, '') <> '' AND LOWER(name) = LOWER(?))",
		keycloakOrgID, alias, name,
	).Order("created_at").First(org).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (r *organizationRepository) UpdateKeycloakRefs(organizationID uuid.UUID, keycloakOrgID, alias, name string) error {
	updates := map[string]any{"keycloak_org_id": keycloakOrgID}
	if alias != "" {
		updates["alias"] = alias
	}
	if name != "" {
		updates["name"] = name
	}
	return r.db.Model(&model.OrganizationModel{}).
		Where("organization_id = ?", organizationID).
		Updates(updates).Error
}

func (r *organizationRepository) UpdateOrganization(org *model.OrganizationModel) error {
	return r.db.Save(org).Error
}

func (r *organizationRepository) ListByKeycloakOrgIDsOrAliases(refs []string) ([]*model.OrganizationModel, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	var orgs []*model.OrganizationModel
	if err := r.db.Where("keycloak_org_id IN ? OR alias IN ?", refs, refs).Find(&orgs).Error; err != nil {
		return nil, err
	}
	return orgs, nil
}

func (r *organizationRepository) AddMembership(membership *model.OrganizationMembershipModel) error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(membership).Error
}
