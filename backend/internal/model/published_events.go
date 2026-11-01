package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PublishedEventResponse contains the public information needed for event listings.
type PublishedEventResponse struct {
	EventID   uuid.UUID              `json:"eventId" gorm:"column:event_id"`
	Title     string                 `json:"title"`
	StartTime time.Time              `json:"startTime"`
	EndTime   time.Time              `json:"endTime"`
	Status    EventStatus            `json:"status"`
	Price     decimal.Decimal        `json:"price" swaggertype:"string" example:"20.00"`
	Category  PublishedEventCategory `json:"category" gorm:"embedded;embeddedPrefix:category_"`
	Location  PublishedEventLocation `json:"location" gorm:"embedded;embeddedPrefix:location_"`
	// Database UUID of the organization that owns the event, as GET /organizations/{organizationID}/rating-summary expects it.
	OrganizerID uuid.UUID `json:"organizerId" gorm:"column:organizer_id"`
	// Alias of the organization that owns the event, as tokens and GET /users/me carry it.
	Alias string `json:"organizerAlias" gorm:"column:organizer_alias"`
}

type PublishedEventCategory struct {
	CategoryID uuid.UUID `json:"categoryId" gorm:"column:id"`
	Category   string    `json:"category" gorm:"column:name"`
}

// PublishedEventLocation is the address of an event, as event listings, event details, own
// bookings and GET /locations show it.
type PublishedEventLocation struct {
	LocationID  uuid.UUID `json:"locationId" gorm:"column:id"`
	Street      string    `json:"street"`
	HouseNumber *string   `json:"houseNumber"`
	PostalCode  string    `json:"postalCode"`
	City        string    `json:"city"`
	// Venue, hall or floor; null when there is none.
	AddressAddition *string `json:"addressAddition"`
	Accessible      bool    `json:"accessible"`
}

// LocationResponse returns the location as events and GET /locations show it.
func LocationResponse(l *LocationModel) PublishedEventLocation {
	return PublishedEventLocation{
		LocationID:      l.LocationID,
		Street:          l.Street,
		HouseNumber:     l.HouseNumber,
		PostalCode:      l.PostalCode,
		City:            l.City,
		AddressAddition: l.AddressAddition,
		Accessible:      l.Accessible,
	}
}

// PublishedEventFilter limits the public listing by start date, category and city or address addition.
type PublishedEventFilter struct {
	Location   string
	CategoryID *uuid.UUID
	// Date is midnight in the calendar timezone; events overlapping this calendar day
	// (including multi-day events that span it) are included. Nil means no date filter.
	Date *time.Time
}
