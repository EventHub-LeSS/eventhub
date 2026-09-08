package repository

import (
	"backend/internal/model"

	"gorm.io/gorm"
)

type OrganizationRepository interface {
	CreateOrganization(org *model.OrganizationModel) error
	GetByKeycloakOrgID(keycloakOrgID string) (*model.OrganizationModel, error)
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

func (r *organizationRepository) GetByKeycloakOrgID(keycloakOrgID string) (*model.OrganizationModel, error) {
	org := &model.OrganizationModel{}
	err := r.db.Where("keycloak_org_id = ?", keycloakOrgID).First(org).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return org, nil
}

func (r *organizationRepository) AddMembership(membership *model.OrganizationMembershipModel) error {
	return r.db.Create(membership).Error
}
