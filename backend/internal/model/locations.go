package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// LocationModel is an address at which events take place. Every distinct address is stored once and
// never changed; events with another address point to another row.
type LocationModel struct {
	LocationID      uuid.UUID `json:"locationId" gorm:"column:location_id;type:uuid;primaryKey"`
	Street          string    `json:"street" gorm:"column:street"`
	HouseNumber     *string   `json:"houseNumber" gorm:"column:house_number"`
	PostalCode      string    `json:"postalCode" gorm:"column:postal_code"`
	City            string    `json:"city" gorm:"column:city"`
	AddressAddition *string   `json:"addressAddition" gorm:"column:address_addition"`
	Accessible      bool      `json:"accessible" gorm:"column:accessible"`
	CreatedAt       time.Time `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (LocationModel) TableName() string { return "locations" }

// Address joins the parts of the location as reminders show it, e.g.
// "Musikhalle Hamburg, Lombardsbrücke 1, 20354 Hamburg". The reminder refresh in
// notificationRepository builds the same text in SQL.
func (l *LocationModel) Address() string {
	street := l.Street
	if l.HouseNumber != nil {
		street += " " + *l.HouseNumber
	}
	parts := []string{street, l.PostalCode + " " + l.City}
	if l.AddressAddition != nil {
		parts = append([]string{*l.AddressAddition}, parts...)
	}
	return strings.Join(parts, ", ")
}

// LocationInput is the location of an event as organizers enter it. The event is linked to the stored
// location with the same address, which is created if there is none.
type LocationInput struct {
	Street string `json:"street" binding:"required,notblank,max=200" example:"Lombardsbrücke"`
	// Omitted or empty when the address has no house number.
	HouseNumber *string `json:"houseNumber" binding:"omitempty,max=20" example:"1"`
	// German postal code with five digits.
	PostalCode string `json:"postalCode" binding:"required,len=5,number" example:"20354"`
	City       string `json:"city" binding:"required,notblank,max=100" example:"Hamburg"`
	// Optional detail such as venue, hall or floor.
	AddressAddition *string `json:"addressAddition" binding:"omitempty,max=200" example:"Musikhalle Hamburg, Großer Saal"`
	// Whether the place is accessible; false when omitted.
	Accessible bool `json:"accessible"`
}

// Location returns the input as a location without ID. Surrounding whitespace is removed and empty
// optional parts become nil, so equal addresses compare equal.
func (in LocationInput) Location() *LocationModel {
	return &LocationModel{
		Street:          strings.TrimSpace(in.Street),
		HouseNumber:     optionalText(in.HouseNumber),
		PostalCode:      strings.TrimSpace(in.PostalCode),
		City:            strings.TrimSpace(in.City),
		AddressAddition: optionalText(in.AddressAddition),
		Accessible:      in.Accessible,
	}
}

func optionalText(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
