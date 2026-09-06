package model

import (
	"time"

	"github.com/google/uuid"
)

type OrganizationModel struct {
	OrganizationID     uuid.UUID `json:"organizationId" gorm:"column:organization_id;type:uuid;primaryKey"`
	KeycloakOrgID      string    `json:"keycloakOrgId" gorm:"column:keycloak_org_id;type:text;not null;uniqueIndex"`
	Name               string    `json:"name" gorm:"column:name;type:text;not null"`
	ContactEmail       *string   `json:"contactEmail" gorm:"column:contact_email;type:text"`
	ContactPhoneNumber *string   `json:"contactPhoneNumber" gorm:"column:contact_phone_number;type:text"`
	Street             *string   `json:"street" gorm:"column:street;type:text"`
	HouseNumber        *string   `json:"houseNumber" gorm:"column:house_number;type:text"`
	PostalCode         *string   `json:"postalCode" gorm:"column:postal_code;type:text"`
	City               *string   `json:"city" gorm:"column:city;type:text"`
	CountryCode        *string   `json:"countryCode" gorm:"column:country_code;type:varchar(2)"`
	CreatedAt          time.Time `json:"createdAt" gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;autoCreateTime"`
	UpdatedAt          time.Time `json:"updatedAt" gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP;autoUpdateTime"`
}

func (OrganizationModel) TableName() string { return "organizations" }