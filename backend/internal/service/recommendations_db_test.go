package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func insertRecommendationBooking(t *testing.T, db *gorm.DB, eventID, userID uuid.UUID, status string, tickets int, expires *time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec("INSERT INTO bookings (booking_id, event_id, user_id, status, number_of_tickets, expires_at) VALUES (?, ?, ?, ?, ?, ?)", id, eventID, userID, status, tickets, expires).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecommendationsDBCandidateFilters(t *testing.T) {
	db := setupTestDB(t)
	availableID, userID := seedEvent(t, db, 10, model.EventStatusPublished)
	var now time.Time
	if err := db.Raw("SELECT NOW()").Scan(&now).Error; err != nil {
		t.Fatal(err)
	}
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	want := map[uuid.UUID]int64{availableID: 0}
	for _, tc := range []struct {
		name                  string
		status                model.EventStatus
		bookingStatus         string
		tickets               int
		expires               *time.Time
		own, started, include bool
	}{
		{name: "draft", status: model.EventStatusDraft},
		{name: "cancelled", status: model.EventStatusCancelled},
		{name: "completed", status: model.EventStatusCompleted},
		{name: "already_started", status: model.EventStatusPublished, started: true},
		{name: "sold_out", status: model.EventStatusPublished, bookingStatus: "confirmed", tickets: 10},
		{name: "overbooked", status: model.EventStatusPublished, bookingStatus: "confirmed", tickets: 11},
		{name: "fully_reserved", status: model.EventStatusPublished, bookingStatus: "reserved", tickets: 10, expires: &future},
		{name: "expired_reservation", status: model.EventStatusPublished, bookingStatus: "reserved", tickets: 10, expires: &past, include: true},
		{name: "expiry_boundary", status: model.EventStatusPublished, bookingStatus: "reserved", tickets: 10, expires: &now, include: true},
		{name: "cancelled_booking", status: model.EventStatusPublished, bookingStatus: "cancelled", tickets: 10, own: true, include: true},
		{name: "failed_booking", status: model.EventStatusPublished, bookingStatus: "failed", tickets: 10, include: true},
		{name: "expired_booking", status: model.EventStatusPublished, bookingStatus: "expired", tickets: 10, include: true},
		{name: "already_booked", status: model.EventStatusPublished, bookingStatus: "confirmed", tickets: 1, own: true},
		{name: "partly_sold", status: model.EventStatusPublished, bookingStatus: "confirmed", tickets: 3, include: true},
		{name: "partly_reserved", status: model.EventStatusPublished, bookingStatus: "reserved", tickets: 3, expires: &future, include: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, otherUser := seedEvent(t, db, 10, tc.status)
			if tc.started {
				if err := db.Exec("UPDATE events SET start_time = ?, end_time = ? WHERE event_id = ?", now, future, id).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.bookingStatus != "" {
				if tc.own {
					otherUser = userID
				}
				insertRecommendationBooking(t, db, id, otherUser, tc.bookingStatus, tc.tickets, tc.expires)
			}
			if tc.include {
				want[id] = 0
				if tc.bookingStatus == "confirmed" {
					want[id] = int64(tc.tickets)
				}
			}
		})
	}
	// Confirmed tickets and live reservations jointly exhaust capacity.
	mixedID, otherUser := seedEvent(t, db, 10, model.EventStatusPublished)
	insertRecommendationBooking(t, db, mixedID, otherUser, "confirmed", 6, nil)
	insertRecommendationBooking(t, db, mixedID, otherUser, "reserved", 4, &future)
	repo := repository.NewRecommendationRepository(db)
	candidates, err := repo.ListCandidates(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != len(want) {
		t.Fatalf("got %d candidates, want %d", len(candidates), len(want))
	}
	for _, candidate := range candidates {
		tickets, ok := want[candidate.EventID]
		if !ok || candidate.ConfirmedTickets != tickets {
			t.Errorf("unexpected candidate %s with %d confirmed tickets", candidate.EventID, candidate.ConfirmedTickets)
		}
	}
}

func TestRecommendationsDBHistoryAndRatings(t *testing.T) {
	db := setupTestDB(t)
	pastID, userID := seedEvent(t, db, 100, model.EventStatusCompleted)
	futureID, _ := seedEvent(t, db, 100, model.EventStatusPublished)
	ignoredID, otherUser := seedEvent(t, db, 100, model.EventStatusCompleted)
	var now time.Time
	if err := db.Raw("SELECT NOW()").Scan(&now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE events SET start_time = ?, end_time = ? WHERE event_id IN ?", now.Add(-2*time.Hour), now.Add(-time.Hour), []uuid.UUID{pastID, ignoredID}).Error; err != nil {
		t.Fatal(err)
	}
	first := insertRecommendationBooking(t, db, pastID, userID, "confirmed", 1, nil)
	second := insertRecommendationBooking(t, db, pastID, userID, "confirmed", 2, nil)
	futureBooking := insertRecommendationBooking(t, db, futureID, userID, "confirmed", 1, nil)
	insertRecommendationBooking(t, db, ignoredID, userID, "cancelled", 1, nil)
	insertRecommendationBooking(t, db, ignoredID, otherUser, "confirmed", 1, nil)
	for bookingID, score := range map[uuid.UUID]int{first: 1, second: 5, futureBooking: 5} {
		if err := db.Exec("INSERT INTO ratings (rating_id, booking_id, score, text) VALUES (?, ?, ?, ?)", uuid.New(), bookingID, score, "test").Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := repository.NewRecommendationRepository(db)
	history, err := repo.ListPastEvents(userID)
	if err != nil || len(history) != 1 || history[0].EventID != pastID {
		t.Fatalf("history=%v, err=%v", history, err)
	}
	var pastEvent, futureEvent model.EventModel
	if err := db.First(&pastEvent, "event_id = ?", pastID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&futureEvent, "event_id = ?", futureID).Error; err != nil {
		t.Fatal(err)
	}
	ratings, err := repo.GetOrganizerRatings([]uuid.UUID{*pastEvent.OrganizerID, *futureEvent.OrganizerID})
	if err != nil || len(ratings) != 1 || ratings[*pastEvent.OrganizerID] != 3 {
		t.Fatalf("ratings=%v, err=%v", ratings, err)
	}
	if ratings, err := repo.GetOrganizerRatings(nil); err != nil || len(ratings) != 0 {
		t.Fatalf("empty organizers: ratings=%v, err=%v", ratings, err)
	}
}

type recommendationQueryCounter struct {
	logger.Interface
	count int
}

func (l *recommendationQueryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	l.count++
	l.Interface.Trace(ctx, begin, fc, err)
}

func TestRecommendationsDBRankingAndQueryCount(t *testing.T) {
	db := setupTestDB(t)
	pastID, userID := seedEvent(t, db, 100, model.EventStatusCompleted)
	preferredID, _ := seedEvent(t, db, 100, model.EventStatusPublished)
	popularID, otherUser := seedEvent(t, db, 100, model.EventStatusPublished)
	if err := db.Exec("UPDATE events SET start_time = NOW() - interval '2 days', end_time = NOW() - interval '1 day' WHERE event_id = ?", pastID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE events SET category_id = (SELECT category_id FROM events WHERE event_id = ?) WHERE event_id = ?", pastID, preferredID).Error; err != nil {
		t.Fatal(err)
	}
	insertRecommendationBooking(t, db, pastID, userID, "confirmed", 1, nil)
	insertRecommendationBooking(t, db, popularID, otherUser, "confirmed", 90, nil)
	counter := &recommendationQueryCounter{Interface: db.Logger}
	repo := repository.NewRecommendationRepository(db.Session(&gorm.Session{Logger: counter}))
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(userID)
	if err != nil || len(result) != 2 || result[0].EventID != preferredID || result[1].EventID != popularID {
		t.Fatalf("unexpected ranking: %v, err=%v", result, err)
	}
	if counter.count != 3 {
		t.Errorf("SQL queries=%d, want 3", counter.count)
	}
}

func TestRecommendationsDBPositiveCapacityConstraint(t *testing.T) {
	db := setupTestDB(t)
	id, _ := seedEvent(t, db, 1, model.EventStatusPublished)
	for _, capacity := range []int{0, -1} {
		if err := db.Exec("UPDATE events SET capacity = ? WHERE event_id = ?", capacity, id).Error; err == nil {
			t.Errorf("database accepted capacity %d", capacity)
		}
	}
}
