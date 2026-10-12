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
