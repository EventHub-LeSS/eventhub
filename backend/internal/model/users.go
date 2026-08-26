package model

import (
	"time"

	"github.com/google/uuid"
)

type UserType string

const (
	UserTypePrivate      UserType = "private"
	UserTypeOrganization UserType = "organization"
)

type UserModel struct {
	UserID      uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"userId"`
	KeycloakSub string    `gorm:"column:keycloak_sub;type:text;not null;uniqueIndex" json:"keycloakSub"`
	UserType    UserType  `gorm:"column:user_type;type:user_type;not null" json:"userType"`
	Email       string    `gorm:"column:email;type:text;not null;uniqueIndex" json:"email"`
	PhoneNumber *string   `gorm:"column:phone_number;type:text" json:"phoneNumber"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;autoCreateTime" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP;autoUpdateTime" json:"updatedAt"`
}

func (UserModel) TableName() string { return "users" }
