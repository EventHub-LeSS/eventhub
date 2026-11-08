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
	if err := s.requirePublicRatedEvent(eventID); err != nil {
		return nil, err
	}
	average, count, err := s.ratingRepo.GetEventRatingSummary(eventID)
	if err != nil {
		return nil, err
	}
	return &model.EventRatingSummary{EventID: eventID, AverageRating: average, RatingCount: count}, nil
}

// ListEventRatings returns one page of the ratings that GetEventRatingSummary counts.
func (s *RatingService) ListEventRatings(eventID uuid.UUID, page, limit int) (*model.EventRatingPage, error) {
	if err := s.requirePublicRatedEvent(eventID); err != nil {
		return nil, err
	}
	items, total, err := s.ratingRepo.ListEventRatings(eventID, page, limit)
	if err != nil {
		return nil, err
	}
	return &model.EventRatingPage{Items: items, Page: page, Limit: limit, Total: total}, nil
}

// requirePublicRatedEvent returns ErrEventNotFound unless the event's ratings are public.
func (s *RatingService) requirePublicRatedEvent(eventID uuid.UUID) error {
	event, err := s.eventRepo.GetEventByID(eventID)
	if err != nil {
		return err
	}
	// Published events expose ratings like their public details. Completed
	// events expose ratings once ended; drafts and cancellations remain private.
	if event == nil || (event.Status != model.EventStatusPublished &&
		(event.Status != model.EventStatusCompleted || event.EndTime.After(time.Now()))) {
		return ErrEventNotFound
	}
	// Events of organizations that an admin has not approved yet stay private.
	if event.OrganizerID != nil {
		org, err := s.orgRepo.GetByID(*event.OrganizerID)
		if err != nil {
			return err
		}
		if org == nil || !org.IsActive {
			return ErrEventNotFound
		}
	}
	return nil
}

func (s *RatingService) GetOrganizerRatingSummary(organizationID uuid.UUID) (*model.OrganizerRatingSummary, error) {
	org, err := s.orgRepo.GetByID(organizationID)
	if err != nil {
		return nil, err
	}
	if org == nil || !org.IsActive {
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
