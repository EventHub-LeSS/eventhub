package model

import (
	"time"

	"github.com/google/uuid"
)

type LocationModel struct {
	LocationID  uuid.UUID `json:"locationId" gorm:"column:location_id;type:uuid;primaryKey"`
	Name        string    `json:"name" gorm:"column:name"`
	City        string    `json:"city" gorm:"column:city"`
	PostalCode  string    `json:"postalCode" gorm:"column:postal_code"`
	Street      string    `json:"street" gorm:"column:street"`
	HouseNumber *string   `json:"houseNumber" gorm:"column:house_number"`
	CreatedAt   time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (LocationModel) TableName() string { return "locations" }
