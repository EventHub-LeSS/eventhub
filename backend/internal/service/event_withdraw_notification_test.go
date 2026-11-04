package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// EVENTHUB-291: withdrawing an event notifies its ticket holders.

type fakeNotificationRepo struct {
	repository.NotificationRepository
	now              time.Time
	location         *model.LocationModel
	created          []model.NotificationData
	createErr        error
	deletedReminders []uuid.UUID
}

func (f *fakeNotificationRepo) DatabaseNow() (time.Time, error) {
	if f.now.IsZero() {
		return time.Now(), nil
	}
	return f.now, nil
}

func (f *fakeNotificationRepo) GetLocation(uuid.UUID) (*model.LocationModel, error) {
	return f.location, nil
}

func (f *fakeNotificationRepo) CreateAll(notifications []model.NotificationData) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, notifications...)
	return nil
}

func (f *fakeNotificationRepo) DeleteEventReminders(eventID uuid.UUID) error {
	f.deletedReminders = append(f.deletedReminders, eventID)
	return nil
}

type fakeTicketHolderRepo struct {
	repository.BookingRepository
	holders []model.TicketHolder
	err     error
}

func (f *fakeTicketHolderRepo) ListConfirmedTicketHolders(uuid.UUID) ([]model.TicketHolder, error) {
	return f.holders, f.err
}

type withdrawFixture struct {
	svc           *EventService
	events        *fakeEventRepo
	notifications *fakeNotificationRepo
	bookings      *fakeTicketHolderRepo
	audit         *fakeAuditRepo
}

var withdrawNow = time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)

func newWithdrawFixture(event *model.EventModel, holders ...model.TicketHolder) *withdrawFixture {
	f := &withdrawFixture{
		events: &fakeEventRepo{event: event},
		notifications: &fakeNotificationRepo{
			now:      withdrawNow,
			location: &model.LocationModel{Name: "Stadthalle", City: "Berlin"},
		},
		bookings: &fakeTicketHolderRepo{holders: holders},
		audit:    &fakeAuditRepo{},
	}
	orgs := &fakeOrgRepo{orgs: map[uuid.UUID]*model.OrganizationModel{orgB.OrganizationID: orgB}}
	tx := &fakeTransactor{
		events: f.events, orgs: orgs, audit: f.audit,
		notifications: f.notifications, bookings: f.bookings,
	}
	f.svc = NewEventService(f.events, orgs, tx)
	return f
}

func (f *withdrawFixture) withdraw(event *model.EventModel) (int, error) {
	return f.svc.WithdrawEvent(auditCtx(audit.EventCancelled), event.EventID, []string{"kc-org-b"})
}

func upcomingEvent(start time.Time) *model.EventModel {
	event := eventOwnedBy(orgB, model.EventStatusPublished)
	event.StartTime, event.EndTime = start, start.Add(3*time.Hour)
	return event
}

func TestWithdrawEvent_NotifiesEachTicketHolderOnce(t *testing.T) {
	event := upcomingEvent(time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC))
	anna, ben := uuid.New(), uuid.New()
	f := newWithdrawFixture(event, model.TicketHolder{UserID: anna, Tickets: 3}, model.TicketHolder{UserID: ben, Tickets: 1})

	notified, err := f.withdraw(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notified != 2 || len(f.notifications.created) != 2 {
		t.Fatalf("notified = %d, created = %d, want 2", notified, len(f.notifications.created))
	}
	if f.events.saved == nil || f.events.saved.Status != model.EventStatusCancelled {
		t.Fatal("event not cancelled")
	}
	if len(f.audit.entries) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(f.audit.entries))
	}
	if len(f.notifications.deletedReminders) != 1 || f.notifications.deletedReminders[0] != event.EventID {
		t.Fatalf("reminders of the event not deleted: %v", f.notifications.deletedReminders)
	}

	want := map[uuid.UUID]string{
		anna: "Altes Konzert am 01.12.2026, 20:00 Uhr in Stadthalle, Berlin wurde vom Veranstalter abgesagt. " +
			"Betroffene Tickets: 3. Der Veranstalter meldet sich bei dir wegen der Erstattung.",
		ben: "Altes Konzert am 01.12.2026, 20:00 Uhr in Stadthalle, Berlin wurde vom Veranstalter abgesagt. " +
			"Betroffene Tickets: 1. Der Veranstalter meldet sich bei dir wegen der Erstattung.",
	}
	ids := map[uuid.UUID]bool{}
	for _, n := range f.notifications.created {
		if n.UserID == nil {
			t.Fatal("notification without recipient")
		}
		if n.Content != want[*n.UserID] {
			t.Errorf("content = %q, want %q", n.Content, want[*n.UserID])
		}
		if n.Subject != "Veranstaltung abgesagt: Altes Konzert" || n.Occasion != OccasionEventCancelled {
			t.Errorf("subject/occasion = %q/%q", n.Subject, n.Occasion)
		}
		if n.EventID == nil || *n.EventID != event.EventID || n.BookingID != nil {
			t.Errorf("event/booking reference = %v/%v", n.EventID, n.BookingID)
		}
		if n.Simulated || n.ReadAt != nil || n.ScheduledAt == nil || !n.ScheduledAt.Equal(withdrawNow) ||
			!n.CreatedAt.Equal(withdrawNow) || !n.UpdatedAt.Equal(withdrawNow) {
			t.Errorf("unexpected delivery fields: %+v", n)
		}
		if n.NotificationID == uuid.Nil || ids[n.NotificationID] {
			t.Error("notification IDs must be set and unique")
		}
		ids[n.NotificationID] = true
	}
	if *f.notifications.created[0].UserID == *f.notifications.created[1].UserID {
		t.Fatal("a user was notified twice")
	}
}

func TestWithdrawEvent_FormatsStartInBerlinTime(t *testing.T) {
	tests := []struct {
		name  string
		start time.Time
		want  string
	}{
		{"winter time", time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC), "am 01.12.2026, 20:00 Uhr"},
		{"summer time", time.Date(2027, 7, 1, 18, 5, 0, 0, time.UTC), "am 01.07.2027, 20:05 Uhr"},
		{"next day in Berlin", time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC), "am 01.01.2027, 00:30 Uhr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := upcomingEvent(tt.start)
			f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 2})
			if _, err := f.withdraw(event); err != nil {
				t.Fatal(err)
			}
			if got := f.notifications.created[0].Content; !strings.Contains(got, tt.want) {
				t.Errorf("content = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

func TestWithdrawEvent_OmitsMissingLocation(t *testing.T) {
	want := "Altes Konzert am 01.12.2026, 20:00 Uhr wurde vom Veranstalter abgesagt. " +
		"Betroffene Tickets: 2. Der Veranstalter meldet sich bei dir wegen der Erstattung."
	for name, prepare := range map[string]func(*model.EventModel, *withdrawFixture){
		"event without location": func(e *model.EventModel, _ *withdrawFixture) { e.LocationID = nil },
		"deleted location":       func(_ *model.EventModel, f *withdrawFixture) { f.notifications.location = nil },
	} {
		t.Run(name, func(t *testing.T) {
			event := upcomingEvent(time.Date(2026, 12, 1, 19, 0, 0, 0, time.UTC))
			f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 2})
			prepare(event, f)
			if _, err := f.withdraw(event); err != nil {
				t.Fatal(err)
			}
			if got := f.notifications.created[0].Content; got != want {
				t.Errorf("content = %q, want %q", got, want)
			}
		})
	}
}

func TestWithdrawEvent_BegunEventNotifiesNobody(t *testing.T) {
	for name, start := range map[string]time.Time{
		"starts now":   withdrawNow,
		"already over": withdrawNow.Add(-48 * time.Hour),
	} {
		t.Run(name, func(t *testing.T) {
			event := upcomingEvent(start)
			f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 2})

			notified, err := f.withdraw(event)
			if err != nil {
				t.Fatal(err)
			}
			if notified != 0 || len(f.notifications.created) != 0 {
				t.Fatalf("notified = %d, created = %d, want none", notified, len(f.notifications.created))
			}
			if len(f.notifications.deletedReminders) != 0 {
				t.Fatal("reminders of a begun event must be kept")
			}
			if f.events.saved == nil || f.events.saved.Status != model.EventStatusCancelled || len(f.audit.entries) != 1 {
				t.Fatal("begun event must still be withdrawn and audited")
			}
		})
	}
}

func TestWithdrawEvent_WithoutTicketHoldersDeletesRemindersOnly(t *testing.T) {
	event := upcomingEvent(withdrawNow.Add(24 * time.Hour))
	f := newWithdrawFixture(event)

	notified, err := f.withdraw(event)
	if err != nil {
		t.Fatal(err)
	}
	if notified != 0 || len(f.notifications.created) != 0 {
		t.Fatalf("notified = %d, created = %d, want none", notified, len(f.notifications.created))
	}
	if len(f.notifications.deletedReminders) != 1 {
		t.Fatal("reminders of an upcoming event must be deleted")
	}
}

func TestWithdrawEvent_NotificationFailureAbortsWithdraw(t *testing.T) {
	for name, breakRepo := range map[string]func(*withdrawFixture){
		"ticket holders unavailable": func(f *withdrawFixture) { f.bookings.err = errors.New("db down") },
		"notifications not saved":    func(f *withdrawFixture) { f.notifications.createErr = errors.New("db down") },
	} {
		t.Run(name, func(t *testing.T) {
			event := upcomingEvent(withdrawNow.Add(24 * time.Hour))
			f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 1})
			breakRepo(f)

			notified, err := f.withdraw(event)
			if err == nil {
				t.Fatal("expected the error so that the transaction is rolled back")
			}
			if notified != 0 || len(f.audit.entries) != 0 {
				t.Fatalf("failed withdraw reported %d notified users and %d audit entries", notified, len(f.audit.entries))
			}
		})
	}
}

func TestWithdrawEvent_RejectedWithdrawNotifiesNobody(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := upcomingEvent(withdrawNow.Add(24 * time.Hour))
			event.Status = status
			f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 1})

			if _, err := f.withdraw(event); !errors.Is(err, ErrNotPublished) {
				t.Fatalf("expected ErrNotPublished, got %v", err)
			}
			if len(f.notifications.created) != 0 || len(f.notifications.deletedReminders) != 0 {
				t.Fatal("rejected withdraw touched notifications")
			}
		})
	}
	t.Run("foreign organization", func(t *testing.T) {
		event := upcomingEvent(withdrawNow.Add(24 * time.Hour))
		f := newWithdrawFixture(event, model.TicketHolder{UserID: uuid.New(), Tickets: 1})

		if _, err := f.svc.WithdrawEvent(auditCtx(audit.EventCancelled), event.EventID, []string{"kc-org-a"}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
		if len(f.notifications.created) != 0 || len(f.notifications.deletedReminders) != 0 {
			t.Fatal("rejected withdraw touched notifications")
		}
	})
}
