package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidRating         = errors.New("rating score must be between 1 and 5")
	ErrRatingBookingRequired = errors.New("a confirmed booking for this event is required to rate it")
	ErrEventAlreadyRated     = errors.New("this event has already been rated by the visitor")
	ErrEventNotRateable      = errors.New("only events that have taken place can be rated")
)

type RatingService struct {
	ratingRepo repository.RatingRepository
	eventRepo  repository.EventRepository
	orgRepo    repository.OrganizationRepository
}

func NewRatingService(ratingRepo repository.RatingRepository, eventRepo repository.EventRepository, orgRepo repository.OrganizationRepository) *RatingService {
	return &RatingService{ratingRepo: ratingRepo, eventRepo: eventRepo, orgRepo: orgRepo}
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

func (s *RatingService) GetOrganizerRatingSummary(organizationID uuid.UUID) (*model.OrganizerRatingSummary, error) {
	org, err := s.orgRepo.GetByID(organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil {
		return nil, ErrOrganizationNotFound
	}
	average, count, err := s.ratingRepo.GetOrganizerRatingSummary(organizationID)
	if err != nil {
		return nil, err
	}
	return &model.OrganizerRatingSummary{OrganizationID: organizationID, AverageRating: average, RatedEventCount: count}, nil
}

func (s *RatingService) CreateEventRating(ctx context.Context, userID, eventID uuid.UUID, score int, text string) (*model.RatingModel, error) {
	if score < 1 || score > 5 {
		return nil, ErrInvalidRating
	}
	rating, err := s.ratingRepo.CreateEventRating(ctx, userID, eventID, score, text)
	switch {
	case errors.Is(err, repository.ErrRatingEventNotFound):
		return nil, ErrEventNotFound
	case errors.Is(err, repository.ErrRatingEventNotEnded):
		return nil, ErrEventNotRateable
	case errors.Is(err, repository.ErrNoConfirmedRatingBooking):
		return nil, ErrRatingBookingRequired
	case errors.Is(err, repository.ErrRatingAlreadyExists):
		return nil, ErrEventAlreadyRated
	case err != nil:
		return nil, err
	default:
		return rating, nil
	}
}
