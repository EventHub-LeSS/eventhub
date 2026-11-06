package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"time"
	_ "time/tzdata" // Europe/Berlin for cancellation notices on hosts without a time zone database

	"github.com/google/uuid"
)

var ErrReminderUnavailable = errors.New("reminder requires a confirmed booking of an upcoming published event")

// OccasionEventCancelled marks the notice a ticket holder receives when an event is withdrawn.
const OccasionEventCancelled = "event_cancelled"

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
		scheduled := event.StartTime.Add(-24 * time.Hour)
		if scheduled.Before(now) {
			scheduled = now
		}
		notification = &model.NotificationData{
			NotificationID: uuid.New(), UserID: &userID, BookingID: &bookingID,
			Occasion: "event_reminder", Subject: "Erinnerung: " + event.Title,
			Content: fmt.Sprintf("%s beginnt am %s in %s. Gebuchte Tickets: %d.",
				event.Title, event.StartTime.Format(time.RFC3339), location.Address(), booking.NumberOfTickets),
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

// EVENTHUB-291: notifyEventCancelled tells every ticket holder of a withdrawn event about the
// cancellation and returns the number of notified users. It must run in the withdraw transaction
// after the event row is locked: payment confirmation and reminder simulation lock the event first
// as well, so no booking is confirmed and no reminder is created while ticket holders are read.
// Each user receives one notice covering all of their confirmed bookings. A republished event
// (EVENTHUB-290) can be withdrawn again; every withdrawal adds new notices and keeps the earlier
// ones. The lock and the status check published → cancelled ensure that one withdrawal notifies
// each user only once. Events that have already begun notify nobody and keep their reminders;
// otherwise the reminders of the event are deleted, because they would still announce the event.
func notifyEventCancelled(tx repository.Tx, event *model.EventModel) (int, error) {
	now, err := tx.Notifications.DatabaseNow()
	if err != nil {
		return 0, err
	}
	if !event.StartTime.After(now) {
		return 0, nil
	}
	if err := tx.Notifications.DeleteEventReminders(event.EventID); err != nil {
		return 0, err
	}
	holders, err := tx.Bookings.ListConfirmedTicketHolders(event.EventID)
	if err != nil {
		return 0, err
	}
	if len(holders) == 0 {
		return 0, nil
	}
	venue := ""
	if event.LocationID != nil {
		location, err := tx.Notifications.GetLocation(*event.LocationID)
		if err != nil {
			return 0, err
		}
		if location != nil {
			name := location.Street
			if location.AddressAddition != nil {
				name = *location.AddressAddition
			}
			venue = fmt.Sprintf(" in %s, %s", name, location.City)
		}
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return 0, err
	}
	start := event.StartTime.In(berlin).Format("02.01.2006, 15:04") + " Uhr"
	notifications := make([]model.NotificationData, 0, len(holders))
	for _, holder := range holders {
		userID, eventID, scheduled := holder.UserID, event.EventID, now
		notifications = append(notifications, model.NotificationData{
			NotificationID: uuid.New(), UserID: &userID, EventID: &eventID,
			Occasion: OccasionEventCancelled, Subject: "Veranstaltung abgesagt: " + event.Title,
			Content: fmt.Sprintf("%s am %s%s wurde vom Veranstalter abgesagt. Betroffene Tickets: %d. "+
				"Der Veranstalter meldet sich bei dir wegen der Erstattung.", event.Title, start, venue, holder.Tickets),
			ScheduledAt: &scheduled, CreatedAt: now, UpdatedAt: now,
		})
	}
	if err := tx.Notifications.CreateAll(notifications); err != nil {
		return 0, err
	}
	return len(notifications), nil
}
