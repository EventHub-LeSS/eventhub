package model

import (
	"time"

	"github.com/google/uuid"
)

type UserModel struct {
	UserID         uuid.UUID `gorm:"column:user_id;type:uuid;primaryKey" json:"userId"`
	KeycloakUserID string    `gorm:"column:keycloak_user_id;type:text;not null;uniqueIndex" json:"keycloakUserId"`
	FirstName      string    `gorm:"column:first_name;type:text;not null" json:"firstName"`
	LastName       string    `gorm:"column:last_name;type:text;not null" json:"lastName"`
	Email          string    `gorm:"column:email;type:text;not null;uniqueIndex" json:"email"`
	PhoneNumber    *string   `gorm:"column:phone_number;type:text" json:"phoneNumber"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;autoCreateTime" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP;autoUpdateTime" json:"updatedAt"`
}

func (UserModel) TableName() string { return "users" }
