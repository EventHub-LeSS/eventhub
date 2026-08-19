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
	tableName struct{} `pg:"events"`

	EventID     uuid.UUID   `json:"eventId" pg:"event_id,pk"`
	Title       string      `json:"title" pg:"title"`
	Description *string     `json:"description" pg:"description"`
	StartTime   time.Time   `json:"startTime" pg:"start_time"`
	EndTime     time.Time   `json:"endTime" pg:"end_time"`
	Capacity    int         `json:"capacity" pg:"capacity"`
	Status      EventStatus `json:"status" pg:"status"`
	Price       float64     `json:"price" pg:"price"`
	CategoryID  uuid.UUID   `json:"categoryId" pg:"category_id"`
	OrganizerID uuid.UUID   `json:"organizerId" pg:"organizer_id"`
	LocationID  uuid.UUID   `json:"locationId" pg:"location_id"`
	CreatedAt   time.Time   `json:"createdAt" pg:"created_at"`
	UpdatedAt   time.Time   `json:"updatedAt" pg:"updated_at"`
}
