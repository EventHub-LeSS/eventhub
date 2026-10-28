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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func notificationPage(t *testing.T, router http.Handler, query string) model.NotificationPage {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notifications"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var page model.NotificationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Items == nil {
		t.Fatal("empty pages must contain items: []")
	}
	return page
}
func markNotificationRead(t *testing.T, router http.Handler, id uuid.UUID, status int) model.NotificationData {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/notifications/"+id.String()+"/read", nil))
	if rec.Code != status {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var note model.NotificationData
	if status == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &note); err != nil {
			t.Fatal(err)
		}
	}
	return note
}
func TestNotificationsPaginationValidation(t *testing.T) {
	user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "page-user"}
	router := notificationRouter(NewNotificationHandler(service.NewNotificationService(&notificationListStub{}, nil), &stubUserRepo{bySubject: user}), &middleware.Principal{Subject: "page-user"})
	for _, query := range []string{"?page=0", "?page=-1", "?page=x", "?limit=0", "?limit=101", "?limit=x", "?page=999999999999999999999999999999", "?page=" + strconv.Itoa(int(^uint(0)>>1)) + "&limit=100"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/notifications"+query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("query=%s status=%d", query, rec.Code)
		}
	}
	page := notificationPage(t, router, "")
	if page.Page != 1 || page.Limit != 50 || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("default page=%+v", page)
	}
}
func TestNotificationsPaginationAndReadDatabase(t *testing.T) {
	db := testdb.Open(t)
	user := seedBookingUser(t, db, "notification-page")
	other := seedBookingUser(t, db, "notification-page-other")
	router := newNotificationDBRouter(db, "notification-page")
	empty := notificationPage(t, router, "")
	if empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatal("expected empty page")
	}
	instant := time.Now().UTC().Truncate(time.Microsecond)
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
	}
	for _, id := range ids {
		note := model.NotificationData{NotificationID: id, UserID: &user, Subject: "test", Content: "test", CreatedAt: instant}
		if err := db.Create(&note).Error; err != nil {
			t.Fatal(err)
		}
	}
	foreign := model.NotificationData{NotificationID: uuid.New(), UserID: &other, Subject: "private", Content: "private"}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	first := notificationPage(t, router, "?page=1&limit=2&userId="+other.String())
	second := notificationPage(t, router, "?page=2&limit=2")
	beyond := notificationPage(t, router, "?page=3&limit=2")
	if first.Total != 3 || first.Page != 1 || first.Limit != 2 || len(first.Items) != 2 || first.Items[0].NotificationID != ids[0] ||
		first.Items[1].NotificationID != ids[1] || second.Total != 3 || len(second.Items) != 1 || second.Items[0].NotificationID != ids[2] ||
		beyond.Total != 3 || len(beyond.Items) != 0 {
		t.Fatalf("pages=%+v %+v %+v", first, second, beyond)
	}
	if first.Items[0].ReadAt != nil {
		t.Fatal("new notification must be unread")
	}
	read := markNotificationRead(t, router, ids[0], http.StatusOK)
	repeated := markNotificationRead(t, router, ids[0], http.StatusOK)
	if read.ReadAt == nil || repeated.ReadAt == nil || !read.ReadAt.Equal(*repeated.ReadAt) || !read.UpdatedAt.Equal(repeated.UpdatedAt) {
		t.Fatalf("read must be idempotent: %+v %+v", read, repeated)
	}
	first = notificationPage(t, router, "?limit=2")
	if first.Items[0].ReadAt == nil {
		t.Fatal("read status not persisted")
	}
	markNotificationRead(t, router, foreign.NotificationID, http.StatusNotFound)
	markNotificationRead(t, router, uuid.New(), http.StatusNotFound)
	stored := model.NotificationData{}
	if err := db.First(&stored, "notification_id=?", foreign.NotificationID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ReadAt != nil {
		t.Fatal("foreign notification changed")
	}
}
func TestSimulatedReminderRefreshDatabase(t *testing.T) {
	db := testdb.Open(t)
	user := seedBookingUser(t, db, "notification-refresh")
	other := seedBookingUser(t, db, "notification-refresh-other")
	event := seedEventForOrg(t, db, "notification-refresh", model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET start_time=NOW()+interval '3 days',end_time=NOW()+interval '4 days' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	booking := createNotificationBooking(t, db, user, event.eventID, model.BookingStatusConfirmed)
	otherBooking := createNotificationBooking(t, db, other, event.eventID, model.BookingStatusConfirmed)
	router := newNotificationDBRouter(db, "notification-refresh")
	originalRec := requestReminder(router, booking)
	if originalRec.Code != http.StatusCreated {
		t.Fatalf("create=%s", originalRec.Body.String())
	}
	var original model.NotificationData
	if err := json.Unmarshal(originalRec.Body.Bytes(), &original); err != nil {
		t.Fatal(err)
	}
	otherRouter := newNotificationDBRouter(db, "notification-refresh-other")
	otherRec := requestReminder(otherRouter, otherBooking)
	if otherRec.Code != http.StatusCreated {
		t.Fatalf("other=%s", otherRec.Body.String())
	}
	var foreign model.NotificationData
	if err := json.Unmarshal(otherRec.Body.Bytes(), &foreign); err != nil {
		t.Fatal(err)
	}
	markNotificationRead(t, router, original.NotificationID, http.StatusOK)
	stable := notificationPage(t, router, "")
	if stable.Items[0].ReadAt == nil || !stable.Items[0].ScheduledAt.Equal(*original.ScheduledAt) {
		t.Fatal("unchanged reminder changed")
	}
	if err := db.Exec("UPDATE events SET title='Neuer Titel',start_time=start_time+interval '2 days',end_time=end_time+interval '2 days',location='Neue Halle, Neue Straße 17, 10115 Berlin' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	updated := notificationPage(t, router, "").Items[0]
	var current model.EventModel
	if err := db.First(&current, "event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.NotificationID != original.NotificationID || updated.ReadAt != nil || !updated.CreatedAt.Equal(original.CreatedAt) ||
		updated.ScheduledAt == nil || !updated.ScheduledAt.Equal(current.StartTime.Add(-24*time.Hour)) ||
		!strings.Contains(updated.Subject, "Neuer Titel") || !strings.Contains(updated.Content, "Neue Halle") ||
		!strings.Contains(updated.Content, "Berlin") || !strings.Contains(updated.Content, "Neue Straße 17") ||
		!strings.Contains(updated.Content, current.StartTime.Format(time.RFC3339)) {
		t.Fatalf("refresh=%+v", updated)
	}
	var stored model.NotificationData
	if err := db.First(&stored, "notification_id=?", original.NotificationID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Content != updated.Content || !stored.ScheduledAt.Equal(*updated.ScheduledAt) {
		t.Fatal("refresh not persisted")
	}
	stored = model.NotificationData{}
	if err := db.First(&stored, "notification_id=?", foreign.NotificationID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Content != foreign.Content {
		t.Fatal("refresh touched another user's reminder")
	}
	// Editing only the location must refresh too.
	markNotificationRead(t, router, original.NotificationID, http.StatusOK)
	if err := db.Exec("UPDATE events SET location='Umbenannte Halle, Neue Straße 17, 10115 Berlin' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	moved := notificationPage(t, router, "").Items[0]
	if moved.ReadAt != nil || !strings.Contains(moved.Content, "Umbenannte Halle") {
		t.Fatalf("location refresh=%+v", moved)
	}
	// Re-simulation must refresh without requiring a list request first.
	if err := db.Exec("UPDATE events SET start_time=start_time-interval '1 day',end_time=end_time-interval '1 day',title='Früherer Beginn' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	rec := requestReminder(router, booking)
	var refreshed model.NotificationData
	if err := json.Unmarshal(rec.Body.Bytes(), &refreshed); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || refreshed.NotificationID != original.NotificationID || !strings.Contains(refreshed.Subject, "Früherer Beginn") ||
		!refreshed.ScheduledAt.Equal(updated.ScheduledAt.Add(-24*time.Hour)) {
		t.Fatalf("repeat refresh=%s", rec.Body.String())
	}
	// A near-term event gets a stable plan anchored at original creation, not a moving "now".
	if err := db.Exec("UPDATE events SET start_time=NOW()+interval '2 hours',end_time=NOW()+interval '3 hours' WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	short := notificationPage(t, router, "").Items[0]
	again := notificationPage(t, router, "").Items[0]
	if !short.ScheduledAt.Equal(original.CreatedAt) || !again.ScheduledAt.Equal(*short.ScheduledAt) {
		t.Fatal("short-notice schedule drifted")
	}
	// An event without a location keeps the last reminder text instead of failing the list.
	if err := db.Exec("UPDATE events SET location=NULL WHERE event_id=?", event.eventID).Error; err != nil {
		t.Fatal(err)
	}
	if kept := notificationPage(t, router, "").Items[0]; kept.Content != again.Content {
		t.Fatalf("reminder without location=%+v", kept)
	}
}

type notificationReadStub struct {
	repository.NotificationRepository
	note *model.NotificationData
	err  error
}

func (s *notificationReadStub) MarkRead(context.Context, uuid.UUID, uuid.UUID) (*model.NotificationData, error) {
	return s.note, s.err
}
func TestNotificationReadErrors(t *testing.T) {
	user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "read-user"}
	for _, tc := range []struct {
		name, id  string
		principal *middleware.Principal
		repo      *notificationReadStub
		status    int
	}{
		{"anonymous", uuid.NewString(), nil, &notificationReadStub{}, 401},
		{"invalid", "invalid", &middleware.Principal{Subject: "read-user"}, &notificationReadStub{}, 400},
		{"missing", uuid.NewString(), &middleware.Principal{Subject: "read-user"}, &notificationReadStub{}, 404},
		{"database", uuid.NewString(), &middleware.Principal{Subject: "read-user"}, &notificationReadStub{err: errors.New("private SQL")}, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := notificationRouter(NewNotificationHandler(service.NewNotificationService(tc.repo, nil), &stubUserRepo{bySubject: user}), tc.principal)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/notifications/"+tc.id+"/read", nil))
			if rec.Code != tc.status || strings.Contains(rec.Body.String(), "private SQL") {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
