package repository

import (
	"backend/internal/model"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NotificationRepository interface {
	ListForUser(context.Context, uuid.UUID) ([]model.NotificationData, error)
	GetReminder(uuid.UUID, uuid.UUID) (*model.NotificationData, error)
	Create(*model.NotificationData) error
	GetLocation(uuid.UUID) (*model.LocationModel, error)
	DatabaseNow() (time.Time, error)
}
type notificationRepository struct{ db *gorm.DB }

func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{db: db}
}
func (r *notificationRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]model.NotificationData, error) {
	notifications := make([]model.NotificationData, 0)
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at DESC, notification_id DESC").Find(&notifications).Error
	return notifications, err
}
func (r *notificationRepository) GetReminder(bookingID, userID uuid.UUID) (*model.NotificationData, error) {
	var notification model.NotificationData
	err := r.db.Where("booking_id = ? AND user_id = ? AND occasion = ?", bookingID, userID, "event_reminder").First(&notification).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &notification, nil
}
func (r *notificationRepository) Create(notification *model.NotificationData) error {
	return r.db.Create(notification).Error
}
func (r *notificationRepository) GetLocation(id uuid.UUID) (*model.LocationModel, error) {
	var location model.LocationModel
	err := r.db.First(&location, "location_id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &location, nil
}
func (r *notificationRepository) DatabaseNow() (time.Time, error) {
	var now time.Time
	err := r.db.Raw("SELECT NOW()").Scan(&now).Error
	return now, err
}
