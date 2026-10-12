package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"time"

	"github.com/google/uuid"
)

type RatingService struct {
	ratingRepo repository.RatingRepository
	eventRepo  repository.EventRepository
}

func NewRatingService(ratingRepo repository.RatingRepository, eventRepo repository.EventRepository) *RatingService {
	return &RatingService{ratingRepo: ratingRepo, eventRepo: eventRepo}
}

func (s *RatingService) GetEventRatingSummary(eventID uuid.UUID) (*model.EventRatingSummary, error) {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return nil, err
	}
	// Published events expose summaries like their public details. Completed
	// events expose summaries once ended; drafts and cancellations remain private.
	if event == nil || (event.Status != model.EventStatusPublished &&
		(event.Status != model.EventStatusCompleted || event.EndTime.After(time.Now()))) {
		return nil, ErrEventNotFound
	}
	average, count, err := s.ratingRepo.GetEventRatingSummary(eventID)
	if err != nil {
		return nil, err
	}
	return &model.EventRatingSummary{EventID: eventID, AverageRating: average, RatingCount: count}, nil
}
