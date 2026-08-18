package model

import (
	"time"

	"github.com/google/uuid"
)

type EventStatus string

const (
	EventStatusDraft     EventStatus = "draft"
	EventStatusPublished EventStatus = "published"
	EventStatusCancelled EventStatus = "cancelled"
	EventStatusCompleted EventStatus = "completed"
)

type EventModel struct {
	EventID     uuid.UUID   `json:"eventId" db:"event_id"`
	Title       string      `json:"title" db:"title"`
	Description *string     `json:"description" db:"description"`
	StartTime   time.Time   `json:"startTime" db:"start_time"`
	EndTime     time.Time   `json:"endTime" db:"end_time"`
	Capacity    int         `json:"capacity" db:"capacity"`
	Status      EventStatus `json:"status" db:"status"`
	Price       float64     `json:"price" db:"price"`
	CategoryID  uuid.UUID   `json:"categoryId" db:"category_id"`
	OrganizerID uuid.UUID   `json:"organizerId" db:"organizer_id"`
	LocationID  uuid.UUID   `json:"locationId" db:"location_id"`
	CreatedAt   time.Time   `json:"createdAt" db:"created_at"`
	UpdatedAt   time.Time   `json:"updatedAt" db:"updated_at"`
}
