package repository

import (
	"backend/internal/model"
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type NotificationRepository interface {
	ListForUser(context.Context, uuid.UUID, int, int) ([]model.NotificationData, int64, error)
	MarkRead(context.Context, uuid.UUID, uuid.UUID) (*model.NotificationData, error)
	RefreshReminder(uuid.UUID, uuid.UUID) error
	GetReminder(uuid.UUID, uuid.UUID) (*model.NotificationData, error)
	Create(*model.NotificationData) error
	GetLocation(uuid.UUID) (*model.LocationModel, error)
	DatabaseNow() (time.Time, error)
}
type notificationRepository struct{ db *gorm.DB }

func NewNotificationRepository(db *gorm.DB) NotificationRepository {
	return &notificationRepository{db: db}
}
func (r *notificationRepository) ListForUser(ctx context.Context, userID uuid.UUID, page, limit int) (notifications []model.NotificationData, total int64, err error) {
	if page < 1 || limit < 1 || limit > 100 || page-1 > math.MaxInt/limit {
		return nil, 0, errors.New("invalid notification pagination")
	}
	notifications = make([]model.NotificationData, 0)
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.NotificationData{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
			return err
		}
		var ids []uuid.UUID
		if err := tx.Model(&model.NotificationData{}).Where("user_id = ?", userID).
			Order("created_at DESC, notification_id DESC").Offset((page-1)*limit).Limit(limit).
			Pluck("notification_id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		repo := &notificationRepository{db: tx}
		if err := repo.refreshReminders(userID, ids); err != nil {
			return err
		}
		return tx.Where("user_id = ? AND notification_id IN ?", userID, ids).
			Order("created_at DESC, notification_id DESC").Find(&notifications).Error
	})
	return
}

// refreshReminders refreshes only the requested page, avoiding unbounded work.
// Simulation has no background sender; its stored content and planned time are
// synchronized before display/re-simulation. CreatedAt anchors short-notice times
// so repeated reads do not continually move scheduledAt forward.
func (r *notificationRepository) refreshReminders(userID uuid.UUID, ids []uuid.UUID) error {
	return r.db.Exec(
		"UPDATE notification_data AS n SET subject = fresh.subject, content = fresh.content, scheduled_at = fresh.scheduled_at, read_at = NULL, updated_at = clock_timestamp() FROM ("+
			"SELECT note.notification_id, 'Erinnerung: ' || e.title AS subject, "+
			"e.title || ' beginnt am ' || to_char(e.start_time AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') || ' in ' || l.name || ', ' || l.city || ' (' || l.postal_code || ' ' || l.street || COALESCE(' ' || l.house_number, '') || '). Gebuchte Tickets: ' || b.number_of_tickets::text || '.' AS content, "+
			"GREATEST(e.start_time - interval '24 hours', note.created_at) AS scheduled_at "+
			"FROM notification_data AS note JOIN bookings AS b ON b.booking_id = note.booking_id AND b.user_id = note.user_id "+
			"JOIN events AS e ON e.event_id = b.event_id JOIN locations AS l ON l.location_id = e.location_id "+
			"WHERE note.user_id = ? AND note.notification_id IN ? AND note.occasion = 'event_reminder' AND note.simulated = TRUE"+
			") AS fresh WHERE n.notification_id = fresh.notification_id AND n.user_id = ? "+
			"AND (n.subject, n.content, n.scheduled_at) IS DISTINCT FROM (fresh.subject, fresh.content, fresh.scheduled_at)",
		userID, ids, userID).Error
}

func (r *notificationRepository) RefreshReminder(id, userID uuid.UUID) error {
	return r.refreshReminders(userID, []uuid.UUID{id})
}

func (r *notificationRepository) MarkRead(ctx context.Context, id, userID uuid.UUID) (notification *model.NotificationData, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &notificationRepository{db: tx}
		if err := repo.RefreshReminder(id, userID); err != nil {
			return err
		}
		var result model.NotificationData
		update := tx.Raw("UPDATE notification_data SET updated_at = CASE WHEN read_at IS NULL THEN clock_timestamp() ELSE updated_at END, read_at = COALESCE(read_at, clock_timestamp()) WHERE notification_id = ? AND user_id = ? RETURNING *", id, userID).Scan(&result)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected > 0 {
			notification = &result
		}
		return nil
	})
	return
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
	err := r.db.Raw("SELECT clock_timestamp()").Scan(&now).Error
	return now, err
}
