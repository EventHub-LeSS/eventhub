package model

import (
	"github.com/google/uuid"
)

type OrganizationUserModel struct {
	UserID            uuid.UUID `json:"userId" gorm:"column:user_id;type:uuid;primaryKey"`
	OrganizationName  string    `json:"organizationName" gorm:"column:organization_name"`
	ContactPersonName string    `json:"contactPersonName" gorm:"column:contact_person_name"`
}

func (OrganizationUserModel) TableName() string { return "organization_users" }
