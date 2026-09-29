package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type stubUserRepo struct {
	repository.UserRepository
	bySubject *model.UserModel
}

func (s *stubUserRepo) GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error) {
	if s.bySubject != nil && s.bySubject.KeycloakUserID == keycloakUserID {
		return s.bySubject, nil
	}
	return nil, nil
}

type handlerRecommendationRepo struct {
	repository.RecommendationRepository
	err        error
	userID     uuid.UUID
	candidates []repository.RecommendationCandidate
}

func (r *handlerRecommendationRepo) ListCandidates(id uuid.UUID, _ time.Time) ([]repository.RecommendationCandidate, error) {
	r.userID = id
	return r.candidates, r.err
}

func (r *handlerRecommendationRepo) ListPastEvents(uuid.UUID, time.Time) ([]*model.EventModel, error) {
	return nil, nil
}

func (r *handlerRecommendationRepo) GetOrganizerRatings([]uuid.UUID) (map[uuid.UUID]float64, error) {
	return nil, nil
}

func TestRecommendationsHandlerResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name          string
		query         string
		authenticated bool
		err           error
		wantStatus    int
	}{
		{"unauthenticated", "", false, nil, http.StatusUnauthorized},
		{"unauthenticated_with_invalid_limit", "?limit=invalid", false, nil, http.StatusUnauthorized},
		{"empty_recommendations", "", true, nil, http.StatusOK},
		{"empty_recommendations_with_limit", "?limit=1", true, nil, http.StatusOK},
		{"repository_failure", "", true, errors.New("secret database details"), http.StatusInternalServerError},
		{"repository_failure_with_limit", "?limit=1", true, errors.New("secret database details"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "subject"}
			repo := &handlerRecommendationRepo{err: tc.err}
			h := NewRecommendationsHandler(service.NewRecommendationsService(repo), &stubUserRepo{bySubject: user})
			router := gin.New()
			if tc.authenticated {
				router.Use(func(c *gin.Context) {
					middleware.SetPrincipalForRequest(c, &middleware.Principal{Subject: "subject"})
					c.Next()
				})
			}
			router.GET("/recommendations", h.GetEventRecommendations)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/recommendations"+tc.query, nil))
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", response.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && strings.TrimSpace(response.Body.String()) != "[]" {
				t.Errorf("want empty JSON array, got %s", response.Body.String())
			}
			if tc.authenticated && repo.userID != user.UserID {
				t.Error("service did not receive authenticated API user ID")
			}
			if !tc.authenticated && repo.userID != uuid.Nil {
				t.Error("unauthenticated request reached repository")
			}
			if strings.Contains(response.Body.String(), "secret database details") {
				t.Error("internal error leaked into response")
			}
		})
	}
}

func TestRecommendationsHandlerLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	second := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	third := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	wantIDs := []uuid.UUID{first, second, third}
	for _, tc := range []struct {
		name       string
		query      string
		wantCount  int
		wantStatus int
	}{
		{"omitted", "", 3, http.StatusOK},
		{"one", "?limit=1", 1, http.StatusOK},
		{"two", "?limit=2", 2, http.StatusOK},
		{"exact_count", "?limit=3", 3, http.StatusOK},
		{"larger_than_result", "?limit=10", 3, http.StatusOK},
		{"empty", "?limit=", 0, http.StatusBadRequest},
		{"without_value", "?limit", 0, http.StatusBadRequest},
		{"non_numeric", "?limit=abc", 0, http.StatusBadRequest},
		{"zero", "?limit=0", 0, http.StatusBadRequest},
		{"negative", "?limit=-1", 0, http.StatusBadRequest},
		{"fractional", "?limit=1.5", 0, http.StatusBadRequest},
		{"overflow", "?limit=999999999999999999999999999999", 0, http.StatusBadRequest},
		{"whitespace", "?limit=%20", 0, http.StatusBadRequest},
		{"repeated", "?limit=1&limit=2", 0, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "subject"}
			// Reverse ranking order verifies that truncation preserves score and tie ordering.
			repo := &handlerRecommendationRepo{candidates: []repository.RecommendationCandidate{
				{EventModel: model.EventModel{EventID: third, Capacity: 100}},
				{EventModel: model.EventModel{EventID: second, Capacity: 100}, ConfirmedTickets: 50},
				{EventModel: model.EventModel{EventID: first, Capacity: 100}, ConfirmedTickets: 50},
			}}
			h := NewRecommendationsHandler(service.NewRecommendationsService(repo), &stubUserRepo{bySubject: user})
			router := gin.New()
			router.Use(func(c *gin.Context) {
				middleware.SetPrincipalForRequest(c, &middleware.Principal{Subject: "subject"})
				c.Next()
			})
			router.GET("/recommendations", h.GetEventRecommendations)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/recommendations"+tc.query, nil))
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d; body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
			if tc.wantStatus == http.StatusBadRequest {
				if repo.userID != uuid.Nil {
					t.Error("invalid limit reached repository")
				}
				var problem model.ErrorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
					t.Fatalf("decode problem: %v", err)
				}
				if problem.Status != http.StatusBadRequest || problem.Detail != "limit must be a single positive integer" {
					t.Errorf("unexpected problem: %+v", problem)
				}
				return
			}
			var events []model.EventModel
			if err := json.Unmarshal(response.Body.Bytes(), &events); err != nil {
				t.Fatalf("decode recommendations: %v", err)
			}
			if len(events) != tc.wantCount {
				t.Fatalf("result count=%d, want %d", len(events), tc.wantCount)
			}
			for i, event := range events {
				if event.EventID != wantIDs[i] {
					t.Errorf("position %d: got %s, want %s", i, event.EventID, wantIDs[i])
				}
			}
		})
	}
}
