package handler

import (
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
	bySubject *model.UserModel
	byEmail   *model.UserModel
	updated   bool
}

func (s *stubUserRepo) GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error) {
	if s.bySubject != nil && s.bySubject.KeycloakUserID == keycloakUserID {
		return s.bySubject, nil
	}
	return nil, nil
}

func (s *stubUserRepo) GetByEmail(email string) (*model.UserModel, error) {
	if s.byEmail != nil && s.byEmail.Email == email {
		return s.byEmail, nil
	}
	return nil, nil
}

func (s *stubUserRepo) UpdateKeycloakUserID(userID uuid.UUID, keycloakUserID string) error {
	s.updated = true
	if s.byEmail != nil {
		s.byEmail.KeycloakUserID = keycloakUserID
	}
	return nil
}

func (s *stubUserRepo) Create(user *model.UserModel) error { return nil }

func (s *stubUserRepo) GetAll() ([]*model.UserModel, error) { return nil, nil }

type handlerRecommendationRepo struct {
	repository.RecommendationRepository
	err    error
	userID uuid.UUID
}

func (r *handlerRecommendationRepo) ListCandidates(id uuid.UUID, _ time.Time) ([]repository.RecommendationCandidate, error) {
	r.userID = id
	return nil, r.err
}

func TestRecommendationsHandlerResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name          string
		authenticated bool
		err           error
		wantStatus    int
	}{
		{"unauthenticated", false, nil, http.StatusUnauthorized},
		{"empty_recommendations", true, nil, http.StatusOK},
		{"repository_failure", true, errors.New("secret database details"), http.StatusInternalServerError},
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
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/recommendations", nil))
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

func TestRecommendationsHandlerRecoversStaleKeycloakUserIDFromEmail(t *testing.T) {
	user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "stale-subject", Email: "finn.betz@grossmeister.de"}
	repo := &stubUserRepo{byEmail: user}
	h := &RecommendationsHandler{userRepo: repo}

	principal := &middleware.Principal{Subject: "fresh-subject", Username: "finn.betz@grossmeister.de"}
	resolved, err := h.resolveUserForPrincipal(principal)
	if err != nil {
		t.Fatalf("resolveUserForPrincipal() error = %v", err)
	}
	if resolved == nil || resolved.UserID != user.UserID {
		t.Fatalf("resolveUserForPrincipal() = %#v, want user %s", resolved, user.UserID)
	}
	if !repo.updated {
		t.Fatal("expected stale Keycloak user ID to be repaired")
	}
	if user.KeycloakUserID != "fresh-subject" {
		t.Fatalf("stale keycloak user id not updated, got %q", user.KeycloakUserID)
	}
}
