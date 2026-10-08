package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

type recommendationRepoStub struct {
	candidates                          []repository.RecommendationCandidate
	history                             []*model.EventModel
	ratings                             map[uuid.UUID]float64
	candidateErr, historyErr, ratingErr error
	calls                               [3]int
	userIDs                             []uuid.UUID
	organizerIDs                        []uuid.UUID
}

func (r *recommendationRepoStub) ListCandidates(userID uuid.UUID) ([]repository.RecommendationCandidate, error) {
	r.calls[0]++
	r.userIDs = append(r.userIDs, userID)
	return r.candidates, r.candidateErr
}

func (r *recommendationRepoStub) ListPastEvents(userID uuid.UUID) ([]*model.EventModel, error) {
	r.calls[1]++
	r.userIDs = append(r.userIDs, userID)
	return r.history, r.historyErr
}

func (r *recommendationRepoStub) GetOrganizerRatings(ids []uuid.UUID) (map[uuid.UUID]float64, error) {
	r.calls[2]++
	r.organizerIDs = ids
	return r.ratings, r.ratingErr
}

func recommendationCandidate(category *uuid.UUID, tickets int64) repository.RecommendationCandidate {
	return repository.RecommendationCandidate{
		EventModel:       model.EventModel{EventID: uuid.New(), Capacity: 100, CategoryID: category},
		ConfirmedTickets: tickets,
	}
}

func TestRecommendationsCategoryUsesUUIDValues(t *testing.T) {
	category, other := uuid.New(), uuid.New()
	categoryCopy := category
	preferred, popular := recommendationCandidate(&category, 0), recommendationCandidate(&other, 90)
	repo := &recommendationRepoStub{
		candidates: []repository.RecommendationCandidate{popular, preferred},
		history:    []*model.EventModel{{EventID: uuid.New(), CategoryID: &categoryCopy}},
	}
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
	if err != nil || len(result) != 2 {
		t.Fatalf("result=%v, err=%v", result, err)
	}
	if result[0].EventID != preferred.EventID {
		t.Fatal("category score 0.5 must outrank popularity score 0.36")
	}
}

func TestRecommendationsHistoryCountsUniqueEvents(t *testing.T) {
	category := uuid.New()
	historical := &model.EventModel{EventID: uuid.New(), CategoryID: &category}
	// Two distinct historical events: affinity=1/2, not 2/3. Nil entries
	// are ignored; an event whose category was deleted still counts in total.
	preferred, popular := recommendationCandidate(&category, 0), recommendationCandidate(nil, 70)
	repo := &recommendationRepoStub{
		candidates: []repository.RecommendationCandidate{preferred, popular},
		history:    []*model.EventModel{nil, historical, historical, {EventID: uuid.New()}},
	}
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
	if err != nil || len(result) != 2 || result[0].EventID != popular.EventID {
		t.Fatalf("popularity 0.28 should outrank category 0.25: result=%v, err=%v", result, err)
	}
}

func TestRecommendationsColdStartWeights(t *testing.T) {
	organizer := uuid.New()
	bestOrganizer, popular, unrated := recommendationCandidate(nil, 0), recommendationCandidate(nil, 20), recommendationCandidate(nil, 0)
	bestOrganizer.OrganizerID = &organizer
	repo := &recommendationRepoStub{
		candidates: []repository.RecommendationCandidate{unrated, popular, bestOrganizer},
		ratings:    map[uuid.UUID]float64{organizer: 5},
	}
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
	if err != nil || len(result) != 3 {
		t.Fatalf("result=%v, err=%v", result, err)
	}
	want := []uuid.UUID{bestOrganizer.EventID, popular.EventID, unrated.EventID}
	for i, event := range result {
		if event.EventID != want[i] {
			t.Errorf("position %d: got %s, want %s", i, event.EventID, want[i])
		}
	}
}

func TestRecommendationsRejectInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		for _, tickets := range []int64{0, 1} {
			invalid, valid := recommendationCandidate(nil, tickets), recommendationCandidate(nil, 0)
			invalid.Capacity = capacity
			repo := &recommendationRepoStub{candidates: []repository.RecommendationCandidate{invalid, valid}}
			result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
			if err != nil || len(result) != 1 || result[0].EventID != valid.EventID {
				t.Fatalf("capacity=%d tickets=%d: result=%v, err=%v", capacity, tickets, result, err)
			}
		}
	}
}

func TestRecommendationsUnitScore(t *testing.T) {
	for _, tc := range []struct{ input, want float64 }{
		{-1, 0}, {0, 0}, {0.25, 0.25}, {1, 1}, {2, 1},
		{math.NaN(), 0}, {math.Inf(1), 0}, {math.Inf(-1), 0},
	} {
		if got := recommendationUnitScore(tc.input); got != tc.want {
			t.Errorf("score(%v)=%v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestRecommendationsDeterministicTies(t *testing.T) {
	a, b, later := recommendationCandidate(nil, 0), recommendationCandidate(nil, 0), recommendationCandidate(nil, 0)
	a.EventID = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	b.EventID = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	a.StartTime = time.Now()
	b.StartTime = a.StartTime
	later.StartTime = a.StartTime.Add(time.Hour)
	for _, candidates := range [][]repository.RecommendationCandidate{{later, b, a}, {b, a, later}} {
		result, err := NewRecommendationsService(&recommendationRepoStub{candidates: candidates}).GetRecommendationsForUser(uuid.New())
		if err != nil || len(result) != 3 || result[0].EventID != a.EventID || result[1].EventID != b.EventID || result[2].EventID != later.EventID {
			t.Fatalf("unexpected tie order: %v, err=%v", result, err)
		}
	}
}

func TestRecommendationsBatchQueries(t *testing.T) {
	organizer, userID := uuid.New(), uuid.New()
	repo := &recommendationRepoStub{}
	for i := 0; i < 100; i++ {
		candidate := recommendationCandidate(nil, 1)
		candidate.OrganizerID = &organizer
		repo.candidates = append(repo.candidates, candidate)
	}
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(userID)
	if err != nil || len(result) != 100 {
		t.Fatalf("result length=%d, err=%v", len(result), err)
	}
	if repo.calls != [3]int{1, 1, 1} || !reflect.DeepEqual(repo.organizerIDs, []uuid.UUID{organizer}) {
		t.Errorf("calls=%v, organizers=%v", repo.calls, repo.organizerIDs)
	}
	if !reflect.DeepEqual(repo.userIDs, []uuid.UUID{userID, userID}) {
		t.Errorf("inconsistent user: %v", repo.userIDs)
	}
}

func TestRecommendationsPropagateRepositoryErrors(t *testing.T) {
	want := errors.New("database unavailable")
	for _, stage := range []string{"candidates", "history", "ratings"} {
		t.Run(stage, func(t *testing.T) {
			repo := &recommendationRepoStub{candidates: []repository.RecommendationCandidate{recommendationCandidate(nil, 0)}}
			switch stage {
			case "candidates":
				repo.candidateErr = want
			case "history":
				repo.historyErr = want
			case "ratings":
				repo.ratingErr = want
			}
			result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
			if !errors.Is(err, want) || result != nil {
				t.Fatalf("result=%v, err=%v; want wrapped error and no partial ranking", result, err)
			}
		})
	}
}

func TestRecommendationsEmptyCandidates(t *testing.T) {
	repo := &recommendationRepoStub{}
	result, err := NewRecommendationsService(repo).GetRecommendationsForUser(uuid.New())
	if err != nil || result == nil || len(result) != 0 || repo.calls != [3]int{1, 0, 0} {
		t.Fatalf("result=%v, err=%v, calls=%v", result, err, repo.calls)
	}
}
