package model

import (
	"github.com/google/uuid"
)

type CategoryModel struct {
	CategoryID  uuid.UUID `json:"categoryId" gorm:"column:category_id;type:uuid;primaryKey"`
	Category    string    `json:"category" gorm:"column:category"`
	Description *string   `json:"description" gorm:"column:description"`
}

func (CategoryModel) TableName() string { return "categories" }
