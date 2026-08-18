package model

import (
	"github.com/google/uuid"
)

type PrivateUserModel struct {
	UserID    uuid.UUID `json:"userId" db:"user_id"`
	FirstName string    `json:"firstName" db:"first_name"`
	LastName  string    `json:"lastName" db:"last_name"`
}
