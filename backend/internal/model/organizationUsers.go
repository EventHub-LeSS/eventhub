package model

import (
	"github.com/google/uuid"
)

type OrganizationUserModel struct {
	tableName struct{} `pg:"organization_users"`

	UserID            uuid.UUID `json:"userId" pg:"user_id,pk"`
	OrganizationName  string    `json:"organizationName" pg:"organization_name"`
	ContactPersonName string    `json:"contactPersonName" pg:"contact_person_name"`
}
