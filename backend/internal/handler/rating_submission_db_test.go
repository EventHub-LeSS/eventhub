package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/internal/testdb"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newRatingSubmissionRouter(db *gorm.DB, principal *middleware.Principal) http.Handler {
	gin.SetMode(gin.TestMode)
	h := NewRatingHandler(service.NewRatingService(repository.NewRatingRepository(db),
		repository.NewEventRepository(db), repository.NewOrganizationRepository(db)), repository.NewUserRepository(db))

	r := gin.New()
	if principal != nil {
		r.Use(func(c *gin.Context) {
			middleware.SetPrincipalForRequest(c, principal)
			c.Next()
		})
	}
	r.POST("/api/v1/events/:eventId/ratings", h.CreateEventRatingHandler)
	return r
}

func seedRatingBooking(t *testing.T, db *gorm.DB, userID, eventID uuid.UUID, status string) uuid.UUID {
	t.Helper()
	bookingID := uuid.New()
	if err := db.Exec(
		"INSERT INTO bookings (booking_id, user_id, event_id, number_of_tickets, status) VALUES (?, ?, ?, 1, ?)",
		bookingID, userID, eventID, status,
	).Error; err != nil {
		t.Fatalf("seed booking: %v", err)
	}
	return bookingID
}

func postEventRating(t *testing.T, router http.Handler, eventID any, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/events/%v/ratings", eventID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestCreateEventRatingRequiresConfirmedOwnedBooking(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "rating-submit", model.EventStatusPublished)
	userID := seedBookingUser(t, db, "kc-rating-submit")
	otherUserID := seedBookingUser(t, db, "kc-rating-submit-other")
	principal := &middleware.Principal{Subject: "kc-rating-submit"}
	router := newRatingSubmissionRouter(db, principal)

	noBookingEvent := seedEventForOrg(t, db, "rating-submit-no-booking", model.EventStatusPublished)
	foreignBookingEvent := seedEventForOrg(t, db, "rating-submit-foreign", model.EventStatusPublished)
	reservedEvent := seedEventForOrg(t, db, "rating-submit-reserved", model.EventStatusPublished)
	cancelledEvent := seedEventForOrg(t, db, "rating-submit-cancelled", model.EventStatusPublished)
	failedEvent := seedEventForOrg(t, db, "rating-submit-failed", model.EventStatusPublished)
	expiredEvent := seedEventForOrg(t, db, "rating-submit-expired", model.EventStatusPublished)
	for _, tc := range []struct {
		name    string
		userID  uuid.UUID
		eventID uuid.UUID
		status  string
	}{
		{"no booking", userID, noBookingEvent.eventID, ""},
		{"foreign booking", otherUserID, foreignBookingEvent.eventID, "confirmed"},
		{"reserved booking", userID, reservedEvent.eventID, "reserved"},
		{"cancelled booking", userID, cancelledEvent.eventID, "cancelled"},
		{"failed booking", userID, failedEvent.eventID, "failed"},
		{"expired booking", userID, expiredEvent.eventID, "expired"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status != "" {
				seedRatingBooking(t, db, tc.userID, tc.eventID, tc.status)
			}
			before := countRatings(t, db)
			rec := postEventRating(t, router, tc.eventID, `{"score":4}`)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			var problem model.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Detail == "" {
				t.Fatalf("missing understandable permission detail: %s", rec.Body.String())
			}
			if got := countRatings(t, db); got != before {
				t.Fatalf("unauthorized request persisted rating: before=%d after=%d", before, got)
			}
		})
	}

	seedRatingBooking(t, db, userID, event.eventID, "confirmed")
	rec := postEventRating(t, router, event.eventID, `{"score":5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("valid booking: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created model.RatingModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created rating: %v: %s", err, rec.Body.String())
	}
	if created.Score != 5 || created.Text != "" || created.BookingID == nil {
		t.Fatalf("created rating did not preserve score, optional text, or booking: %+v", created)
	}
	var persisted struct {
		UserID  uuid.UUID
		EventID uuid.UUID
	}
	if err := db.Table("ratings AS r").Select("b.user_id, b.event_id").
		Joins("JOIN bookings AS b ON b.booking_id = r.booking_id").
		Where("r.rating_id = ?", created.RatingID).Scan(&persisted).Error; err != nil {
		t.Fatalf("load rating association: %v", err)
	}
	if persisted.UserID != userID || persisted.EventID != event.eventID {
		t.Fatalf("rating linked to user/event %s/%s, want %s/%s", persisted.UserID, persisted.EventID, userID, event.eventID)
	}
}

func TestCreateEventRatingRejectsInvalidAndDuplicateRatings(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "rating-submit-duplicate", model.EventStatusPublished)
	userID := seedBookingUser(t, db, "kc-rating-submit-duplicate")
	principal := &middleware.Principal{Subject: "kc-rating-submit-duplicate"}
	router := newRatingSubmissionRouter(db, principal)
	seedRatingBooking(t, db, userID, event.eventID, "confirmed")

	for _, body := range []string{`{"score":0}`, `{"score":6}`, `{"score":"5"}`, `{}`} {
		rec := postEventRating(t, router, event.eventID, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status=%d body=%s", body, rec.Code, rec.Body.String())
		}
	}
	if rec := postEventRating(t, router, "invalid-event-id", `{"score":4}`); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid event ID: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := countRatings(t, db); got != 0 {
		t.Fatalf("invalid request persisted %d ratings", got)
	}

	rec := postEventRating(t, router, event.eventID, `{"score":4,"text":"gut"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first rating: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created model.RatingModel
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.Text != "gut" || created.Score != 4 {
		t.Fatalf("rating text/score were not saved: %v %+v", err, created)
	}
	seedRatingBooking(t, db, userID, event.eventID, "confirmed")
	rec = postEventRating(t, router, event.eventID, `{"score":3}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate rating: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := countRatings(t, db); got != 1 {
		t.Fatalf("duplicate request persisted another rating: count=%d", got)
	}
}

func TestCreateEventRatingRequiresAuthentication(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "rating-submit-auth", model.EventStatusPublished)
	rec := postEventRating(t, newRatingSubmissionRouter(db, nil), event.eventID, `{"score":4}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreateEventRatingConcurrentDuplicateSubmissions(t *testing.T) {
	db := testdb.Open(t)
	event := seedEventForOrg(t, db, "rating-submit-concurrent", model.EventStatusPublished)
	userID := seedBookingUser(t, db, "kc-rating-submit-concurrent")
	principal := &middleware.Principal{Subject: "kc-rating-submit-concurrent"}
	router := newRatingSubmissionRouter(db, principal)
	seedRatingBooking(t, db, userID, event.eventID, "confirmed")
	seedRatingBooking(t, db, userID, event.eventID, "confirmed")

	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := postEventRating(t, router, event.eventID, `{"score":5}`)
			statuses <- rec.Code
		}()
	}
	wg.Wait()
	close(statuses)
	created, conflicts := 0, 0
	for status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		default:
			t.Errorf("unexpected concurrent response status %d", status)
		}
	}
	if created != 1 || conflicts != 1 || countRatings(t, db) != 1 {
		t.Fatalf("created=%d conflicts=%d ratings=%d, want one of each response and one stored review",
			created, conflicts, countRatings(t, db))
	}
}

func countRatings(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var count int64
	if err := db.Table("ratings").Count(&count).Error; err != nil {
		t.Fatalf("count ratings: %v", err)
	}
	return count
}
