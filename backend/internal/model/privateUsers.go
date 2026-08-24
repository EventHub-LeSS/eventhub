package model

import (
	"github.com/google/uuid"
)

type PrivateUserModel struct {
	UserID    uuid.UUID `json:"userId" gorm:"column:user_id;type:uuid;primaryKey"`
	FirstName string    `json:"firstName" gorm:"column:first_name"`
	LastName  string    `json:"lastName" gorm:"column:last_name"`
}

func (PrivateUserModel) TableName() string { return "private_users" }
