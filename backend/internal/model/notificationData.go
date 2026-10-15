package model

import (
	"time"

	"github.com/google/uuid"
)

type NotificationData struct {
	Occasion       string     `json:"occasion" gorm:"column:occasion;default:general"`
	BookingID      *uuid.UUID `json:"bookingId" gorm:"column:booking_id;type:uuid"`
	ScheduledAt    *time.Time `json:"scheduledAt" gorm:"column:scheduled_at"`
	Simulated      bool       `json:"simulated" gorm:"column:simulated"`
	NotificationID uuid.UUID  `json:"notificationId" gorm:"column:notification_id;type:uuid;primaryKey"`
	UserID         *uuid.UUID `json:"userId" gorm:"column:user_id;type:uuid"`
	Subject        string     `json:"subject" gorm:"column:subject"`
	Content        string     `json:"content" gorm:"column:content"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `json:"updatedAt" gorm:"column:updated_at;autoUpdateTime"`
}

func (NotificationData) TableName() string { return "notification_data" }
