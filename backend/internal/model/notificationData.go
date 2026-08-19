package model

import (
	"time"

	"github.com/google/uuid"
)

type NotificationData struct {
	tableName struct{} `pg:"notification_data"`

	NotificationID uuid.UUID `json:"notificationId" pg:"notification_id,pk"`
	UserID         uuid.UUID `json:"userId" pg:"user_id"`
	Subject        string    `json:"subject" pg:"subject"`
	Content        string    `json:"content" pg:"content"`
	CreatedAt      time.Time `json:"createdAt" pg:"created_at"`
	UpdatedAt      time.Time `json:"updatedAt" pg:"updated_at"`
}
