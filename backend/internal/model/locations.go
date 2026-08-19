package model

import (
	"time"

	"github.com/google/uuid"
)

type LocationModel struct {
	tableName struct{} `pg:"locations"`

	LocationID  uuid.UUID `json:"locationId" pg:"location_id,pk"`
	Name        string    `json:"name" pg:"name"`
	City        string    `json:"city" pg:"city"`
	PostalCode  string    `json:"postalCode" pg:"postal_code"`
	Street      string    `json:"street" pg:"street"`
	HouseNumber *string   `json:"houseNumber" pg:"house_number"`
	CreatedAt   time.Time `json:"createdAt" pg:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" pg:"updated_at"`
}
