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
	EventID     uuid.UUID   `json:"eventId" gorm:"column:event_id;type:uuid;primaryKey"`
	Title       string      `json:"title" gorm:"column:title"`
	Description *string     `json:"description" gorm:"column:description"`
	StartTime   time.Time   `json:"startTime" gorm:"column:start_time"`
	EndTime     time.Time   `json:"endTime" gorm:"column:end_time"`
	Capacity    int         `json:"capacity" gorm:"column:capacity"`
	Status      EventStatus `json:"status" gorm:"column:status"`
	Price       float64     `json:"price" gorm:"column:price"`
	CategoryID  uuid.UUID   `json:"categoryId" gorm:"column:category_id;type:uuid"`
	OrganizerID uuid.UUID   `json:"organizerId" gorm:"column:organizer_id;type:uuid"`
	LocationID  uuid.UUID   `json:"locationId" gorm:"column:location_id;type:uuid"`
	CreatedAt   time.Time   `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time   `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (EventModel) TableName() string { return "events" }
