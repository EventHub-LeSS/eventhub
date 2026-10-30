package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type notificationListStub struct {
	repository.NotificationRepository
	userID uuid.UUID
	err    error
}

func (s *notificationListStub) ListForUser(_ context.Context, id uuid.UUID, _, _ int) ([]model.NotificationData, int64, error) {
	s.userID = id
	return nil, 0, s.err
}
func notificationRouter(h *NotificationHandler, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if principal != nil {
			middleware.SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	router.GET("/api/v1/notifications", h.ListNotificationsHandler)
	router.PATCH("/api/v1/notifications/:notificationId/read", h.MarkNotificationReadHandler)
	router.POST("/api/v1/bookings/:bookingId/reminder", h.SimulateReminderHandler)
	return router
}
func TestNotificationsAuthenticationAndErrors(t *testing.T) {
	id := uuid.New()
	user := &model.UserModel{UserID: id, KeycloakUserID: "notification-user"}
	for _, tc := range []struct {
		name      string
		principal *middleware.Principal
		user      *model.UserModel
		err       error
		status    int
	}{
		{"anonymous", nil, nil, nil, 401},
		{"unsynced user", &middleware.Principal{Subject: "missing"}, nil, nil, 403},
		{"empty", &middleware.Principal{Subject: "notification-user"}, user, nil, 200},
		{"database failure", &middleware.Principal{Subject: "notification-user"}, user, errors.New("secret SQL"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &notificationListStub{err: tc.err}
			router := notificationRouter(NewNotificationHandler(service.NewNotificationService(repo, nil), &stubUserRepo{bySubject: tc.user}), tc.principal)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notifications?userId="+uuid.NewString(), nil))
			if rec.Code != tc.status {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret SQL") {
				t.Fatal("database details leaked")
			}
			if tc.status == 200 && (!strings.Contains(rec.Body.String(), "\"items\":[]") || repo.userID != id) {
				t.Fatal("empty response or user resolution incorrect")
			}
			if tc.name == "anonymous" {
				rec = httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+uuid.NewString()+"/reminder", nil))
				if rec.Code != 401 {
					t.Fatalf("reminder status=%d", rec.Code)
				}
			}
		})
	}
	router := notificationRouter(NewNotificationHandler(service.NewNotificationService(nil, nil), &stubUserRepo{bySubject: user}), &middleware.Principal{Subject: "notification-user"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bookings/invalid/reminder", nil))
	if rec.Code != 400 {
		t.Fatalf("invalid ID status=%d", rec.Code)
	}
}
func newNotificationDBRouter(db *gorm.DB, subject string) http.Handler {
	return notificationRouter(NewNotificationHandler(service.NewNotificationService(repository.NewNotificationRepository(db), repository.NewTransactor(db)), repository.NewUserRepository(db)), &middleware.Principal{Subject: subject})
}
func createNotificationBooking(t *testing.T, db *gorm.DB, user, event uuid.UUID, status model.BookingStatus) uuid.UUID {
	t.Helper()
	booking := model.BookingModel{BookingID: uuid.New(), UserID: &user, EventID: &event, Status: status, NumberOfTickets: 2}
	if status == model.BookingStatusReserved {
		expires := time.Now().Add(time.Hour)
		booking.ExpiresAt = &expires
	}
	if err := db.Create(&booking).Error; err != nil {
		t.Fatal(err)
	}
	return booking.BookingID
}
func requestReminder(router http.Handler, id uuid.UUID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/bookings/"+id.String()+"/reminder", nil))
	return rec
}
func TestNotificationsDatabaseLifecycle(t *testing.T) {
	db := testdb.Open(t)
	owner := seedBookingUser(t, db, "notification-owner")
	other := seedBookingUser(t, db, "notification-other")
	event := seedEventForOrg(t, db, "notifications", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET start_time=NOW()+interval '2 days',end_time=NOW()+interval '3 days' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	booking := createNotificationBooking(t, db, owner, event.eventID, model.BookingStatusConfirmed)
	router := newNotificationDBRouter(db, "notification-owner")
	rec := requestReminder(router, booking)
	if rec.Code != 201 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var notification model.NotificationData
	if err := json.Unmarshal(rec.Body.Bytes(), &notification); err != nil {
		t.Fatal(err)
	}
	var storedEvent model.EventModel
	if err := db.First(&storedEvent, "event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if notification.UserID == nil || *notification.UserID != owner || notification.BookingID == nil || *notification.BookingID != booking ||
		notification.Occasion != "event_reminder" || !notification.Simulated || notification.CreatedAt.IsZero() || notification.ScheduledAt == nil ||
		!notification.ScheduledAt.Equal(storedEvent.StartTime.Add(-24*time.Hour)) ||
		!strings.Contains(notification.Content, storedEvent.Title) ||
		!strings.Contains(notification.Content, " in "+event.location+".") || !strings.Contains(notification.Content, storedEvent.StartTime.Format(time.RFC3339)) {
		t.Fatalf("incorrect reminder: %+v", notification)
	}
	rec = requestReminder(router, booking)
	var repeated model.NotificationData
	if err := json.Unmarshal(rec.Body.Bytes(), &repeated); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || repeated.NotificationID != notification.NotificationID {
		t.Fatalf("repeat=%s", rec.Body.String())
	}
	foreign := createNotificationBooking(t, db, other, event.eventID, model.BookingStatusConfirmed)
	if rec = requestReminder(router, foreign); rec.Code != 404 {
		t.Fatalf("foreign status=%d", rec.Code)
	}
	if rec = requestReminder(router, uuid.New()); rec.Code != 404 {
		t.Fatalf("unknown status=%d", rec.Code)
	}
	// Store a newer generic notification belonging to another user; it must never appear.
	foreignNote := model.NotificationData{NotificationID: uuid.New(), UserID: &other, Subject: "private", Content: "private", Occasion: "general"}
	if err := db.Create(&foreignNote).Error; err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notifications?userId="+other.String(), nil))
	var notifications model.NotificationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &notifications); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(notifications.Items) != 1 || notifications.Items[0].NotificationID != notification.NotificationID {
		t.Fatalf("list=%s", rec.Body.String())
	}
	newer := model.NotificationData{NotificationID: uuid.New(), UserID: &owner, Subject: "new", Content: "new", CreatedAt: notification.CreatedAt.Add(time.Second)}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil))
	if err := json.Unmarshal(rec.Body.Bytes(), &notifications); err != nil {
		t.Fatal(err)
	}
	if len(notifications.Items) != 2 || notifications.Items[0].NotificationID != newer.NotificationID {
		t.Fatalf("ordering=%s", rec.Body.String())
	}
}
func TestReminderEligibilityDatabase(t *testing.T) {
	db := testdb.Open(t)
	owner := seedBookingUser(t, db, "reminder-owner")
	router := newNotificationDBRouter(db, "reminder-owner")
	for _, status := range []model.BookingStatus{model.BookingStatusReserved, model.BookingStatusCancelled, model.BookingStatusFailed, model.BookingStatusExpired} {
		event := seedEventForOrg(t, db, "eligibility", model.EventStatusPublished)
		id := createNotificationBooking(t, db, owner, event.eventID, status)
		if rec := requestReminder(router, id); rec.Code != 409 {
			t.Fatalf("booking=%s status=%d body=%s", status, rec.Code, rec.Body.String())
		}
	}
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusCancelled, model.EventStatusCompleted} {
		event := seedEventForOrg(t, db, "eligibility", status)
		id := createNotificationBooking(t, db, owner, event.eventID, model.BookingStatusConfirmed)
		if rec := requestReminder(router, id); rec.Code != 409 {
			t.Fatalf("event=%s status=%d", status, rec.Code)
		}
	}
	event := seedEventForOrg(t, db, "eligibility", model.EventStatusPublished)
	id := createNotificationBooking(t, db, owner, event.eventID, model.BookingStatusConfirmed)
	if err := db.Exec("UPDATE events SET start_time=NOW()-interval '2 days',end_time=NOW()-interval '1 day' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if rec := requestReminder(router, id); rec.Code != 409 {
		t.Fatalf("past status=%d", rec.Code)
	}
	if err := db.Exec("UPDATE events SET start_time=NOW()+interval '2 hours',end_time=NOW()+interval '3 hours' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	rec := requestReminder(router, id)
	var note model.NotificationData
	if err := json.Unmarshal(rec.Body.Bytes(), &note); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 201 || note.ScheduledAt == nil || !note.ScheduledAt.Equal(note.CreatedAt) {
		t.Fatalf("short notice=%s", rec.Body.String())
	}
	// Existing reminders do not bypass checks after cancellation.
	if err := db.Exec("UPDATE bookings SET status='cancelled' WHERE booking_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if rec := requestReminder(router, id); rec.Code != 409 {
		t.Fatalf("cancelled repeat status=%d", rec.Code)
	}
	var count int64
	if err := db.Model(&model.NotificationData{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("invalid bookings created reminders: %d", count)
	}
}
func TestReminderConcurrentRequestsDatabase(t *testing.T) {
	db := testdb.Open(t)
	owner := seedBookingUser(t, db, "parallel-reminder")
	event := seedEventForOrg(t, db, "parallel", model.EventStatusPublished)
	booking := createNotificationBooking(t, db, owner, event.eventID, model.BookingStatusConfirmed)
	router := newNotificationDBRouter(db, "parallel-reminder")
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); responses <- requestReminder(router, booking) }()
	}
	wg.Wait()
	close(responses)
	created := 0
	var id uuid.UUID
	for rec := range responses {
		if rec.Code == 201 {
			created++
		} else if rec.Code != 200 {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var note model.NotificationData
		if err := json.Unmarshal(rec.Body.Bytes(), &note); err != nil {
			t.Fatal(err)
		}
		if id == uuid.Nil {
			id = note.NotificationID
		} else if id != note.NotificationID {
			t.Fatal("duplicate reminders")
		}
	}
	var count int64
	if err := db.Model(&model.NotificationData{}).Where("booking_id=?", booking).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if created != 1 || count != 1 {
		t.Fatalf("created=%d stored=%d", created, count)
	}
}
