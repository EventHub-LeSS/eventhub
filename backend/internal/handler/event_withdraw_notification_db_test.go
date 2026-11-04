package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"backend/internal/testdb"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EVENTHUB-291: withdrawing an event notifies its ticket holders. These tests run the real
// handlers, services and repositories against Postgres; see testdb.Open for how to enable them.

func createTicketBooking(t *testing.T, db *gorm.DB, user *uuid.UUID, event uuid.UUID, status model.BookingStatus, tickets int) uuid.UUID {
	t.Helper()
	booking := model.BookingModel{BookingID: uuid.New(), UserID: user, EventID: &event, Status: status, NumberOfTickets: tickets}
	if status == model.BookingStatusReserved {
		expires := time.Now().Add(time.Hour)
		booking.ExpiresAt = &expires
	}
	if err := db.Create(&booking).Error; err != nil {
		t.Fatal(err)
	}
	return booking.BookingID
}

func createReminder(t *testing.T, db *gorm.DB, user, booking uuid.UUID) uuid.UUID {
	t.Helper()
	note := model.NotificationData{
		NotificationID: uuid.New(), UserID: &user, BookingID: &booking, Occasion: "event_reminder",
		Subject: "Erinnerung", Content: "Erinnerung", Simulated: true,
	}
	if err := db.Create(&note).Error; err != nil {
		t.Fatal(err)
	}
	return note.NotificationID
}

func cancellationNotices(t *testing.T, db *gorm.DB, event uuid.UUID) map[uuid.UUID]model.NotificationData {
	t.Helper()
	var notes []model.NotificationData
	if err := db.Where("event_id = ? AND occasion = ?", event, service.OccasionEventCancelled).Find(&notes).Error; err != nil {
		t.Fatal(err)
	}
	byUser := map[uuid.UUID]model.NotificationData{}
	for _, note := range notes {
		if note.UserID == nil {
			t.Fatal("cancellation notice without recipient")
		}
		if _, dup := byUser[*note.UserID]; dup {
			t.Fatalf("user %s was notified twice", *note.UserID)
		}
		byUser[*note.UserID] = note
	}
	return byUser
}

func notificationExists(t *testing.T, db *gorm.DB, id uuid.UUID) bool {
	t.Helper()
	var count int64
	if err := db.Model(&model.NotificationData{}).Where("notification_id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count == 1
}

func withdrawResponse(t *testing.T, rec *httptest.ResponseRecorder) model.EventWithdrawnResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response model.EventWithdrawnResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func eventStatus(t *testing.T, db *gorm.DB, event uuid.UUID) model.EventStatus {
	t.Helper()
	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", event).Error; err != nil {
		t.Fatal(err)
	}
	return stored.Status
}

func TestWithdrawEventHandler_NotifiesTicketHoldersDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	other := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)

	anna := seedBookingUser(t, db, "withdraw-anna")
	ben := seedBookingUser(t, db, "withdraw-ben")
	reserving := seedBookingUser(t, db, "withdraw-reserving")
	cancelling := seedBookingUser(t, db, "withdraw-cancelling")

	annaFirst := createTicketBooking(t, db, &anna, event.eventID, model.BookingStatusConfirmed, 2)
	createTicketBooking(t, db, &anna, event.eventID, model.BookingStatusConfirmed, 3)
	createTicketBooking(t, db, &anna, event.eventID, model.BookingStatusCancelled, 4)
	createTicketBooking(t, db, &ben, event.eventID, model.BookingStatusConfirmed, 1)
	createTicketBooking(t, db, &reserving, event.eventID, model.BookingStatusReserved, 1)
	cancelledBooking := createTicketBooking(t, db, &cancelling, event.eventID, model.BookingStatusCancelled, 1)
	createTicketBooking(t, db, nil, event.eventID, model.BookingStatusConfirmed, 1)
	otherBooking := createTicketBooking(t, db, &anna, other.eventID, model.BookingStatusConfirmed, 1)

	annaReminder := createReminder(t, db, anna, annaFirst)
	staleReminder := createReminder(t, db, cancelling, cancelledBooking)
	otherReminder := createReminder(t, db, anna, otherBooking)

	response := withdrawResponse(t, postWithdraw(t, newEventRouter(db, principalManaging("kc-org-b")), event.eventID.String()))
	if response.Message != "event withdrawn" || response.NotifiedUsers != 2 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if status := eventStatus(t, db, event.eventID); status != model.EventStatusCancelled {
		t.Fatalf("status not persisted: %s", status)
	}

	notices := cancellationNotices(t, db, event.eventID)
	if len(notices) != 2 {
		t.Fatalf("expected notices for anna and ben, got %d", len(notices))
	}
	var stored model.EventModel
	if err := db.First(&stored, "event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	start := stored.StartTime.In(berlin).Format("02.01.2006, 15:04") + " Uhr"
	for user, tickets := range map[uuid.UUID]string{anna: "5", ben: "1"} {
		note, ok := notices[user]
		if !ok {
			t.Fatalf("user %s was not notified", user)
		}
		want := "Altes Konzert am " + start + " in loc-" + event.locationID.String() + ", Bonn wurde vom Veranstalter abgesagt. " +
			"Betroffene Tickets: " + tickets + ". Der Veranstalter meldet sich bei dir wegen der Erstattung."
		if note.Content != want || note.Subject != "Veranstaltung abgesagt: Altes Konzert" {
			t.Errorf("notice = %q / %q, want %q", note.Subject, note.Content, want)
		}
		if note.BookingID != nil || note.Simulated || note.ReadAt != nil || note.ScheduledAt == nil || !note.ScheduledAt.Equal(note.CreatedAt) {
			t.Errorf("unexpected notice fields: %+v", note)
		}
	}

	if notificationExists(t, db, annaReminder) || notificationExists(t, db, staleReminder) {
		t.Error("reminders of the withdrawn event must be deleted")
	}
	if !notificationExists(t, db, otherReminder) {
		t.Error("reminders of other events must be kept")
	}
	if len(cancellationNotices(t, db, other.eventID)) != 0 {
		t.Error("other events must not be announced as cancelled")
	}

	// The notice appears in the recipient's own notification list with the event reference.
	page := notificationPage(t, newNotificationDBRouter(db, "withdraw-anna"), "")
	var found bool
	for _, item := range page.Items {
		if item.Occasion == service.OccasionEventCancelled {
			found = item.EventID != nil && *item.EventID == event.eventID && item.ReadAt == nil
		}
	}
	if !found {
		t.Fatalf("cancellation notice missing from list: %+v", page.Items)
	}
	for _, subject := range []string{"withdraw-reserving", "withdraw-cancelling"} {
		for _, item := range notificationPage(t, newNotificationDBRouter(db, subject), "").Items {
			if item.Occasion == service.OccasionEventCancelled {
				t.Errorf("%s holds no confirmed ticket but was notified", subject)
			}
		}
	}
}

func TestWithdrawEventHandler_BegunEventNotifiesNobodyDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET start_time = NOW() - interval '1 hour', end_time = NOW() + interval '1 hour' WHERE event_id = ?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	user := seedBookingUser(t, db, "withdraw-begun")
	booking := createTicketBooking(t, db, &user, event.eventID, model.BookingStatusConfirmed, 2)
	reminder := createReminder(t, db, user, booking)

	response := withdrawResponse(t, postWithdraw(t, newEventRouter(db, principalManaging("kc-org-b")), event.eventID.String()))
	if response.NotifiedUsers != 0 {
		t.Fatalf("notifiedUsers = %d, want 0", response.NotifiedUsers)
	}
	if status := eventStatus(t, db, event.eventID); status != model.EventStatusCancelled {
		t.Fatalf("begun event must still be withdrawn, status %s", status)
	}
	if len(cancellationNotices(t, db, event.eventID)) != 0 {
		t.Error("begun event must not notify")
	}
	if !notificationExists(t, db, reminder) {
		t.Error("reminders of a begun event must be kept")
	}
}

func TestWithdrawEventHandler_NotificationFailureRollsBackDatabase(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "kc-org-b", model.EventStatusPublished)
	user := seedBookingUser(t, db, "withdraw-rollback")
	booking := createTicketBooking(t, db, &user, event.eventID, model.BookingStatusConfirmed, 1)
	reminder := createReminder(t, db, user, booking)
	// An existing notice violates the unique index, so saving the notices fails.
	existing := model.NotificationData{
		NotificationID: uuid.New(), UserID: &user, EventID: &event.eventID,
		Occasion: service.OccasionEventCancelled, Subject: "alt", Content: "alt",
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	rec := postWithdraw(t, newEventRouter(db, principalManaging("kc-org-b")), event.eventID.String())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "idx_notification_event_cancelled") {
		t.Error("database details leaked")
	}
	if status := eventStatus(t, db, event.eventID); status != model.EventStatusPublished {
		t.Errorf("failed withdraw changed status to %s", status)
	}
	if !notificationExists(t, db, reminder) {
		t.Error("failed withdraw deleted reminders")
	}
	if notices := cancellationNotices(t, db, event.eventID); len(notices) != 1 || notices[user].NotificationID != existing.NotificationID {
		t.Errorf("failed withdraw changed notices: %+v", notices)
	}
	var audits int64
	if err := db.Model(&model.AuditLogModel{}).Where("resource_id = ?", event.eventID.String()).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Errorf("failed withdraw recorded %d audit entries", audits)
	}
}
