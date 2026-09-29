package repository

import (
	"backend/internal/model"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// RecommendationCandidate carries the ticket count from the same query that
// checks availability. An event does not need to be loaded again for scoring.
type RecommendationCandidate struct {
	model.EventModel `gorm:"embedded"`
	ConfirmedTickets int64
}

type RecommendationRepository interface {
	ListCandidates(userID uuid.UUID, now time.Time) ([]RecommendationCandidate, error)
	ListPastEvents(userID uuid.UUID, now time.Time) ([]*model.EventModel, error)
	GetOrganizerRatings(organizerIDs []uuid.UUID) (map[uuid.UUID]float64, error)
}

type recommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) RecommendationRepository {
	return &recommendationRepository{db: db}
}

func (r *recommendationRepository) ListCandidates(userID uuid.UUID, now time.Time) ([]RecommendationCandidate, error) {
	var candidates []RecommendationCandidate
	// Confirmed bookings affect popularity; live reservations only affect capacity.
	counts := r.db.Table("bookings").
		Select("event_id, SUM(CASE WHEN status = ? THEN number_of_tickets ELSE 0 END) AS confirmed_tickets, SUM(number_of_tickets) AS occupied_tickets", model.BookingStatusConfirmed).
		Where("status = ? OR (status = ? AND expires_at > NOW() )", model.BookingStatusConfirmed, model.BookingStatusReserved).
		Group("event_id")
	bookedByUser := r.db.Table("bookings AS own").Select("1").
		Where("own.event_id = e.event_id AND own.user_id = ? AND own.status = ?", userID, model.BookingStatusConfirmed)
	err := r.db.Table("events AS e").
		Select("e.*, COALESCE(t.confirmed_tickets, 0) AS confirmed_tickets").
		Joins("LEFT JOIN (?) AS t ON t.event_id = e.event_id", counts).
		Where("e.status = ? AND e.start_time > ? AND e.capacity > 0", model.EventStatusPublished, now).
		Where("COALESCE(t.occupied_tickets, 0) < e.capacity").
		Where("NOT EXISTS (?)", bookedByUser).
		Scan(&candidates).Error
	return candidates, err
}

func (r *recommendationRepository) ListPastEvents(userID uuid.UUID, now time.Time) ([]*model.EventModel, error) {
	var events []*model.EventModel
	// One attended event counts once, even if the user made multiple bookings.
	err := r.db.Table("events AS e").Distinct("e.*").
		Joins("JOIN bookings AS b ON b.event_id = e.event_id").
		Where("b.user_id = ? AND b.status = ? AND e.end_time < ?", userID, model.BookingStatusConfirmed, now).
		Scan(&events).Error
	return events, err
}

func (r *recommendationRepository) GetOrganizerRatings(organizerIDs []uuid.UUID) (map[uuid.UUID]float64, error) {
	ratings := make(map[uuid.UUID]float64)
	if len(organizerIDs) == 0 {
		return ratings, nil
	}
	var rows []struct {
		OrganizerID uuid.UUID
		Rating      float64
	}
	err := r.db.Table("events AS e").Select("e.organizer_id, AVG(r.score) AS rating").
		Joins("JOIN bookings AS b ON b.event_id = e.event_id JOIN ratings AS r ON r.booking_id = b.booking_id").
		Where("r.is_visible = true AND e.organizer_id IN ? AND e.status = ?", organizerIDs, model.EventStatusCompleted).
		Group("e.organizer_id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	// No ratings is a normal cold-start case, represented by an absent map entry.
	for _, row := range rows {
		ratings[row.OrganizerID] = row.Rating
	}
	return ratings, nil
}
