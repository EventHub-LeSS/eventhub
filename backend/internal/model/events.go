package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type EventStatus string

const (
	EventStatusDraft     EventStatus = "draft"
	EventStatusPublished EventStatus = "published"
	EventStatusCancelled EventStatus = "cancelled"
	EventStatusCompleted EventStatus = "completed"
)

// IsValidEventStatus reports whether status is one of the event statuses of the event_status enum.
func IsValidEventStatus(status EventStatus) bool {
	switch status {
	case EventStatusDraft, EventStatusPublished, EventStatusCancelled, EventStatusCompleted:
		return true
	}
	return false
}

type EventModel struct {
	EventID              uuid.UUID       `json:"eventId" gorm:"column:event_id;type:uuid;primaryKey"`
	Title                string          `json:"title" gorm:"column:title"`
	Description          *string         `json:"description" gorm:"column:description"`
	StartTime            time.Time       `json:"startTime" gorm:"column:start_time"`
	EndTime              time.Time       `json:"endTime" gorm:"column:end_time"`
	Capacity             int             `json:"capacity" gorm:"column:capacity"`
	Status               EventStatus     `json:"status" gorm:"column:status"`
	WithdrawnByModerator bool            `json:"withdrawnByModerator" gorm:"column:withdrawn_by_moderator"`
	Price                decimal.Decimal `json:"price" gorm:"column:price;type:numeric(12,2)"`
	CategoryID           *uuid.UUID      `json:"categoryId" gorm:"column:category_id;type:uuid"`
	OrganizerID          *uuid.UUID      `json:"organizerId" gorm:"column:organizer_id;type:uuid"`
	LocationID           *uuid.UUID      `json:"locationId" gorm:"column:location_id;type:uuid"`
	ImageBase64          *string         `json:"-" gorm:"column:image_base64"`
	ImageContentType     *string         `json:"-" gorm:"column:image_content_type"`
	CreatedAt            time.Time       `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt            time.Time       `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

// Image content types an event image may have.
const (
	ImageContentTypePNG  = "image/png"
	ImageContentTypeJPEG = "image/jpeg"
)

// MaxEventImageBytes is the largest raw image size the upload route accepts.
const MaxEventImageBytes = 2 << 20 // 2 MiB

// IsValidEventImageContentType reports whether the MIME type is a supported event image type.
func IsValidEventImageContentType(contentType string) bool {
	switch contentType {
	case ImageContentTypePNG, ImageContentTypeJPEG:
		return true
	}
	return false
}

func (EventModel) TableName() string { return "events" }

// EVENTHUB-78: Veranstaltung bearbeiten
type UpdateEventRequest struct {
	Title       string          `json:"title" binding:"required,min=3,max=200"`
	Description *string         `json:"description" binding:"omitempty,max=5000"`
	StartTime   time.Time       `json:"startTime" binding:"required,gt"`
	EndTime     time.Time       `json:"endTime" binding:"required,gtfield=StartTime"`
	Capacity    int             `json:"capacity" binding:"required,min=1"`
	Price       decimal.Decimal `json:"price" binding:"gte=0"`
	CategoryID  uuid.UUID       `json:"categoryId" binding:"required"`
	// Address of the event; the event is linked to the stored location with this address.
	Location LocationInput `json:"location"`
}

// EVENTHUB-76: Veranstaltung veröffentlichen
type EventActionResponse struct {
	Message string `json:"message" example:"event published"`
}

// EVENTHUB-82: Veranstaltung zurückziehen
type EventWithdrawnResponse struct {
	Message string `json:"message" example:"event withdrawn"`
}

type EventModerationResponse struct {
	EventID uuid.UUID   `json:"eventId"`
	Status  EventStatus `json:"status"`
	Changed bool        `json:"changed"`
}

type ModeratedEvent struct {
	EventModel
	OrganizationName *string `json:"organizationName" gorm:"column:organization_name" extensions:"x-nullable"`
}

type ModeratedEventFilter struct {
	Status         EventStatus
	OrganizationID *uuid.UUID
	EventID        *uuid.UUID
}

type ModeratedEventPage struct {
	Items []ModeratedEvent `json:"items"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
	Total int64            `json:"total"`
}

type ModerateEventContentRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Reason      string  `json:"reason" binding:"required" minLength:"1" maxLength:"1000"`
}

// EVENTHUB-256: Veranstaltungsentwurf löschen
type EventDeletedResponse struct {
	Message string `json:"message" example:"event deleted"`
}

// EVENTHUB-77: Veranstaltung als Entwurf speichern
type CreateDraftRequest struct {
	// Keycloak ID or alias of the organization that owns the draft, as returned by GET /users/me.
	// May be omitted when the caller manages events in exactly one organization.
	OrganizationID string          `json:"organizationId,omitempty" example:"my-org"`
	Title          string          `json:"title" binding:"required,min=3,max=200"`
	Description    *string         `json:"description" binding:"omitempty,max=5000"`
	StartTime      time.Time       `json:"startTime" binding:"required,gt"`
	EndTime        time.Time       `json:"endTime" binding:"required,gtfield=StartTime"`
	Capacity       int             `json:"capacity" binding:"required,min=1,max=100000"`
	Price          decimal.Decimal `json:"price" binding:"gte=0,lte=10000"`
	CategoryID     uuid.UUID       `json:"categoryId" binding:"required"`
	// Address of the event; the event is linked to the stored location with this address, which is
	// created if there is none.
	Location LocationInput `json:"location"`
}
