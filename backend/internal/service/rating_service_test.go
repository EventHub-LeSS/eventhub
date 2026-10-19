package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type ratingCreateStub struct {
	repository.RatingRepository
	rating  *model.RatingModel
	err     error
	called  bool
	userID  uuid.UUID
	eventID uuid.UUID
	score   int
	text    string
}

func (s *ratingCreateStub) CreateEventRating(_ context.Context, userID, eventID uuid.UUID, score int, text string) (*model.RatingModel, error) {
	s.called = true
	s.userID, s.eventID, s.score, s.text = userID, eventID, score, text
	return s.rating, s.err
}

func TestCreateEventRatingValidatesAndDelegates(t *testing.T) {
	rating := &model.RatingModel{RatingID: uuid.New(), Score: 5}
	repo := &ratingCreateStub{rating: rating}
	service := NewRatingService(repo, nil, nil)
	userID, eventID := uuid.New(), uuid.New()
	for _, score := range []int{1, 5} {
		repo.called = false
		created, err := service.CreateEventRating(context.Background(), userID, eventID, score, "")
		if err != nil || created != rating {
			t.Fatalf("score %d: created=%+v err=%v", score, created, err)
		}
		if !repo.called || repo.userID != userID || repo.eventID != eventID || repo.score != score || repo.text != "" {
			t.Fatalf("repository received unexpected score %d request: %+v", score, repo)
		}
	}

	for _, score := range []int{0, 6} {
		repo.called = false
		if _, err := service.CreateEventRating(context.Background(), userID, eventID, score, "text"); !errors.Is(err, ErrInvalidRating) {
			t.Errorf("score %d: err=%v, want ErrInvalidRating", score, err)
		}
		if repo.called {
			t.Errorf("score %d reached repository", score)
		}
	}
}

func TestCreateEventRatingMapsRepositoryErrors(t *testing.T) {
	for _, tc := range []struct {
		repoErr error
		wantErr error
	}{
		{repository.ErrRatingEventNotFound, ErrEventNotFound},
		{repository.ErrRatingEventNotEnded, ErrEventNotRateable},
		{repository.ErrNoConfirmedRatingBooking, ErrRatingBookingRequired},
		{repository.ErrRatingAlreadyExists, ErrEventAlreadyRated},
	} {
		repo := &ratingCreateStub{err: tc.repoErr}
		_, err := NewRatingService(repo, nil, nil).CreateEventRating(context.Background(), uuid.New(), uuid.New(), 3, "")
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("err=%v, want %v", err, tc.wantErr)
		}
	}
}
