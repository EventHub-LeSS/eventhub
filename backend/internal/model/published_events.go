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
}

type PublishedEventCategory struct {
	CategoryID uuid.UUID `json:"categoryId" gorm:"column:id"`
	Category   string    `json:"category" gorm:"column:name"`
}

type PublishedEventLocation struct {
	LocationID  uuid.UUID `json:"locationId" gorm:"column:id"`
	Name        string    `json:"name"`
	City        string    `json:"city"`
	PostalCode  string    `json:"postalCode"`
	Street      string    `json:"street"`
	HouseNumber *string   `json:"houseNumber"`
}

// PublishedEventFilter limits the public listing by city or venue name.
type PublishedEventFilter struct {
	Location string
}
