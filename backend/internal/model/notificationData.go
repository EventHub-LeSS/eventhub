package model

import (
	"time"

	"github.com/google/uuid"
)

type NotificationData struct {
	NotificationID uuid.UUID `json:"notificationId" db:"notification_id"`
	UserID         uuid.UUID `json:"userId" db:"user_id"`
	Subject        string    `json:"subject" db:"subject"`
	Content        string    `json:"content" db:"content"`
	CreatedAt      time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" db:"updated_at"`
}
