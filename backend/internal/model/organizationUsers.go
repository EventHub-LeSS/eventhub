package model

import (
	"github.com/google/uuid"
)

type OrganizationUserModel struct {
	UserID            uuid.UUID `json:"userId" db:"user_id"`
	OrganizationName  string    `json:"organizationName" db:"organization_name"`
	ContactPersonName string    `json:"contactPersonName" db:"contact_person_name"`
}
