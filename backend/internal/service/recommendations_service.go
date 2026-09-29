package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"fmt"
	"math"
	"sort"

	"github.com/google/uuid"
)

type RecommendationsService struct {
	repo repository.RecommendationRepository
}

func NewRecommendationsService(repo repository.RecommendationRepository) *RecommendationsService {
	return &RecommendationsService{repo: repo}
}

func (s *RecommendationsService) GetRecommendationsForUser(userID uuid.UUID) ([]*model.EventModel, error) {
	candidates, err := s.repo.ListCandidates(userID)
	if err != nil {
		return nil, fmt.Errorf("load recommendation candidates: %w", err)
	}
	result := make([]*model.EventModel, 0, len(candidates))
	if len(candidates) == 0 {
		return result, nil
	}

	history, err := s.repo.ListPastEvents(userID)
	if err != nil {
		return nil, fmt.Errorf("load recommendation history: %w", err)
	}
	categoryCounts := make(map[uuid.UUID]int)
	seenEvents := make(map[uuid.UUID]bool)
	total := 0
	for _, event := range history {
		if event == nil || seenEvents[event.EventID] {
			continue
		}
		seenEvents[event.EventID] = true
		total++
		if event.CategoryID != nil {
			categoryCounts[*event.CategoryID]++
		}
	}

	organizerIDs := make([]uuid.UUID, 0)
	seenOrganizers := make(map[uuid.UUID]bool)
	for _, candidate := range candidates {
		if id := candidate.OrganizerID; id != nil && !seenOrganizers[*id] {
			seenOrganizers[*id] = true
			organizerIDs = append(organizerIDs, *id)
		}
	}
	ratings, err := s.repo.GetOrganizerRatings(organizerIDs)
	if err != nil {
		return nil, fmt.Errorf("load recommendation organizer ratings: %w", err)
	}

	type scoredEvent struct {
		event *model.EventModel
		score float64
	}

	scoredEvents := make([]scoredEvent, 0, len(candidates))
	for i := range candidates {
		candidate := &candidates[i]
		event := &candidate.EventModel
		// Defensive guard even if invalid legacy data reaches the service.
		if event.Capacity <= 0 {
			continue
		}
		categoryAffinity := 0.0
		if event.CategoryID != nil && total > 0 {
			categoryAffinity = float64(categoryCounts[*event.CategoryID]) / float64(total)
		}
		organizerScore := 0.0
		if event.OrganizerID != nil {
			organizerScore = recommendationUnitScore((ratings[*event.OrganizerID] - 1) / 4)
		}
		popularity := recommendationUnitScore(float64(candidate.ConfirmedTickets) / float64(event.Capacity))
		score := 0.5*categoryAffinity + 0.4*popularity + 0.1*organizerScore
		scoredEvents = append(scoredEvents, scoredEvent{event: event, score: score})
	}

	// Deterministic ties: sooner events first, then UUID, independent of DB order.
	sort.Slice(scoredEvents, func(i, j int) bool {
		a, b := scoredEvents[i], scoredEvents[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if !a.event.StartTime.Equal(b.event.StartTime) {
			return a.event.StartTime.Before(b.event.StartTime)
		}
		return a.event.EventID.String() < b.event.EventID.String()
	})

	for _, item := range scoredEvents {
		result = append(result, item.event)
	}

	return result, nil
}

func recommendationUnitScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return math.Max(0, math.Min(1, score))
}
