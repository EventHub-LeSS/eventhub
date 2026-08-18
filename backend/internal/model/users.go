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
	UserID      uuid.UUID `json:"userId" db:"user_id"`
	Role        UserRole  `json:"role" db:"role"`
	UserType    UserType  `json:"userType" db:"user_type"`
	Email       string    `json:"email" db:"email"`
	PhoneNumber *string   `json:"phoneNumber" db:"phone_number"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}
