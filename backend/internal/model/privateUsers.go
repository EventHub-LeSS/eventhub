package model

import (
	"github.com/google/uuid"
)

type PrivateUserModel struct {
	tableName struct{} `pg:"private_users"`

	UserID    uuid.UUID `json:"userId" pg:"user_id,pk"`
	FirstName string    `json:"firstName" pg:"first_name"`
	LastName  string    `json:"lastName" pg:"last_name"`
}
