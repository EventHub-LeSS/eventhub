package model

import (
	"github.com/google/uuid"
)

type CategoryModel struct {
	CategoryID  uuid.UUID `json:"categoryId" db:"category_id"`
	Category    string    `json:"category" db:"category"`
	Description *string   `json:"description" db:"description"`
}
