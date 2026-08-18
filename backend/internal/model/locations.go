package model

import (
	"time"

	"github.com/google/uuid"
)

type LocationModel struct {
	LocationID  uuid.UUID `json:"locationId" db:"location_id"`
	Name        string    `json:"name" db:"name"`
	City        string    `json:"city" db:"city"`
	PostalCode  string    `json:"postalCode" db:"postal_code"`
	Street      string    `json:"street" db:"street"`
	HouseNumber *string   `json:"houseNumber" db:"house_number"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}
