package model

import (
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	UserRoleAdmin     UserRole = "administrator"
	UserRoleVisitor   UserRole = "visitor"
	UserRoleOrganizer UserRole = "organizer"
)

type UserType string

const (
	UserTypePrivate      UserType = "private"
	UserTypeOrganization UserType = "organization"
)

type UserModel struct {
	tableName struct{} `pg:"users"`

	UserID      uuid.UUID `json:"userId" pg:"user_id,pk"`
	Role        UserRole  `json:"role" pg:"role"`
	UserType    UserType  `json:"userType" pg:"user_type"`
	Email       string    `json:"email" pg:"email"`
	PhoneNumber *string   `json:"phoneNumber" pg:"phone_number"`
	CreatedAt   time.Time `json:"createdAt" pg:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" pg:"updated_at"`
}
