package handler

import (
	"testing"

	"backend/internal/middleware"
	"backend/internal/model"

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
