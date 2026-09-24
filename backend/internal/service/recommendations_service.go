package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"sort"

	"github.com/google/uuid"
)

type RecommendationsService struct {
	eventRepo   repository.EventRepository
	bookingRepo repository.BookingRepository
}

func NewRecommendationsService(eventRepo repository.EventRepository, bookingRepo repository.BookingRepository) *RecommendationsService {
	return &RecommendationsService{eventRepo: eventRepo, bookingRepo: bookingRepo}
}

func (s *RecommendationsService) GetRecommendationsForUser(userID uuid.UUID) ([]*model.EventModel, error) {
	events, err := s.eventRepo.GetAvailableEvents()
	if err != nil {
		return nil, err
	}

	type scoredEvent struct {
		event *model.EventModel
		score float64
	}

	scoredEvents := make([]scoredEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		score := s.scoreEventForUser(userID, *event)
		scoredEvents = append(scoredEvents, scoredEvent{event: event, score: score})
	}

	sort.Slice(scoredEvents, func(i, j int) bool {
		return scoredEvents[i].score > scoredEvents[j].score
	})

	result := make([]*model.EventModel, 0, len(scoredEvents))
	for _, item := range scoredEvents {
		result = append(result, item.event)
	}

	return result, nil
}

func (s *RecommendationsService) scoreEventForUser(userID uuid.UUID, event model.EventModel) float64 {
	categoryAffinity := s.computeCategoryAffinity(userID, event.CategoryID)
	popularityScore := s.computePopularityScore(event.EventID)
	organizerScore := s.computeOrganizerScore(event.OrganizerID)

	return 0.5*categoryAffinity + 0.4*popularityScore + 0.1*organizerScore
}

func (s *RecommendationsService) computeOrganizerScore(OrganizerID *uuid.UUID) float64 {
	rating, err := s.bookingRepo.GetAvgRatingByOrganizer(OrganizerID)

	if err != nil {
		return 0
	}
	return (float64(rating) - 1) / 4
}

func (s *RecommendationsService) computePopularityScore(EventID uuid.UUID) float64 {
	booked, err := s.eventRepo.GetConfirmedTicketCount(EventID)

	if err != nil {
		return 0
	}

	event, err := s.eventRepo.GetEventByID(EventID)

	if err != nil {
		return 0
	}

	return float64(booked) / float64(event.Capacity)

}

func (s *RecommendationsService) computeCategoryAffinity(userID uuid.UUID, CategoryID *uuid.UUID) float64 {
	events, err := s.bookingRepo.ListEventsByUser(userID)

	if err != nil || len(events) == 0 {
		return 0
	}

	total := len(events)
	sameCategory := 0
	for _, event := range events {
		if event.CategoryID == CategoryID {
			sameCategory++
		}
	}
	return float64(sameCategory) / float64(total)

}
