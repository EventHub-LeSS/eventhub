package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var ErrReminderUnavailable = errors.New("reminder requires a confirmed booking of an upcoming published event")

type NotificationService struct {
	notifications repository.NotificationRepository
	tx            repository.Transactor
}

func NewNotificationService(notifications repository.NotificationRepository, tx repository.Transactor) *NotificationService {
	return &NotificationService{notifications: notifications, tx: tx}
}

var ErrNotificationNotFound = errors.New("notification not found")

func (s *NotificationService) ListForUser(ctx context.Context, userID uuid.UUID, page, limit int) (*model.NotificationPage, error) {
	notifications, total, err := s.notifications.ListForUser(ctx, userID, page, limit)
	if err != nil {
		return nil, err
	}
	if notifications == nil {
		notifications = make([]model.NotificationData, 0)
	}
	return &model.NotificationPage{Items: notifications, Page: page, Limit: limit, Total: total}, nil
}

func (s *NotificationService) MarkRead(ctx context.Context, id, userID uuid.UUID) (*model.NotificationData, error) {
	notification, err := s.notifications.MarkRead(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if notification == nil {
		return nil, ErrNotificationNotFound
	}
	return notification, nil
}

// SimulateReminder uses the same event-before-booking lock order as payment confirmation.
// The booking lock and unique index ensure repeated or concurrent requests create only one reminder.
func (s *NotificationService) SimulateReminder(ctx context.Context, bookingID, userID uuid.UUID) (notification *model.NotificationData, created bool, err error) {
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		initial, err := tx.Bookings.GetBookingByID(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(initial, userID) {
			return ErrBookingNotFound
		}
		if initial.EventID == nil {
			return ErrReminderUnavailable
		}
		event, err := tx.Events.LockEvent(*initial.EventID)
		if err != nil {
			return err
		}
		booking, err := tx.Bookings.LockBooking(bookingID)
		if err != nil {
			return err
		}
		if !ownsBooking(booking, userID) {
			return ErrBookingNotFound
		}
		now, err := tx.Notifications.DatabaseNow()
		if err != nil {
			return err
		}
		if event == nil || booking.EventID == nil || *booking.EventID != event.EventID ||
			booking.Status != model.BookingStatusConfirmed || event.Status != model.EventStatusPublished ||
			!event.StartTime.After(now) || event.LocationID == nil {
			return ErrReminderUnavailable
		}
		location, err := tx.Notifications.GetLocation(*event.LocationID)
		if err != nil {
			return err
		}
		if location == nil {
			return ErrReminderUnavailable
		}
		existing, err := tx.Notifications.GetReminder(bookingID, userID)
		if err != nil {
			return err
		}
		if existing != nil {
			if err := tx.Notifications.RefreshReminder(existing.NotificationID, userID); err != nil {
				return err
			}
			notification, err = tx.Notifications.GetReminder(bookingID, userID)
			if err != nil {
				return err
			}
			return nil
		}
		street := location.Street
		if location.HouseNumber != nil {
			street += " " + *location.HouseNumber
		}
		scheduled := event.StartTime.Add(-24 * time.Hour)
		if scheduled.Before(now) {
			scheduled = now
		}
		notification = &model.NotificationData{
			NotificationID: uuid.New(), UserID: &userID, BookingID: &bookingID,
			Occasion: "event_reminder", Subject: "Erinnerung: " + event.Title,
			Content: fmt.Sprintf("%s beginnt am %s in %s, %s (%s %s). Gebuchte Tickets: %d.",
				event.Title, event.StartTime.Format(time.RFC3339), location.Name, location.City,
				location.PostalCode, street, booking.NumberOfTickets),
			ScheduledAt: &scheduled, Simulated: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Notifications.Create(notification); err != nil {
			return err
		}
		created = true
		return nil
	})
	return
}
