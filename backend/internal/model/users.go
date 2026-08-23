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
	UserID      uuid.UUID `json:"userId" gorm:"column:user_id;type:uuid;primaryKey"`
	Role        UserRole  `json:"role" gorm:"column:role"`
	UserType    UserType  `json:"userType" gorm:"column:user_type"`
	Email       string    `json:"email" gorm:"column:email"`
	PhoneNumber *string   `json:"phoneNumber" gorm:"column:phone_number"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (UserModel) TableName() string { return "users" }
