package model

import (
	"github.com/google/uuid"
)

type CategoryModel struct {
	tableName struct{} `pg:"categories"`

	CategoryID  uuid.UUID `json:"categoryId" pg:"category_id,pk"`
	Category    string    `json:"category" pg:"category"`
	Description *string   `json:"description" pg:"description"`
}
