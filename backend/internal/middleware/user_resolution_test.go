package middleware

import (
	"backend/internal/model"
	"backend/internal/repository"
	"testing"

	"github.com/google/uuid"
)

type userResolutionRepoStub struct {
	repository.UserRepository
	bySubject *model.UserModel
	byEmail   *model.UserModel
	updated   bool
}

func (s *userResolutionRepoStub) GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error) {
	if s.bySubject != nil && s.bySubject.KeycloakUserID == keycloakUserID {
		return s.bySubject, nil
	}
	return nil, nil
}

func (s *userResolutionRepoStub) GetByEmail(email string) (*model.UserModel, error) {
	if s.byEmail != nil && s.byEmail.Email == email {
		return s.byEmail, nil
	}
	return nil, nil
}

func (s *userResolutionRepoStub) UpdateKeycloakUserID(userID uuid.UUID, keycloakUserID string) error {
	s.updated = true
	if s.byEmail != nil && s.byEmail.UserID == userID {
		s.byEmail.KeycloakUserID = keycloakUserID
	}
	return nil
}

func TestResolveUserForPrincipalRecoversStaleKeycloakUserIDFromEmail(t *testing.T) {
	user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "stale-subject", Email: "user@example.com"}
	repo := &userResolutionRepoStub{byEmail: user}

	principal := &Principal{Subject: "fresh-subject", Username: "user@example.com"}
	resolved, err := ResolveUserForPrincipal(repo, principal)
	if err != nil {
		t.Fatalf("ResolveUserForPrincipal() error = %v", err)
	}
	if resolved == nil || resolved.UserID != user.UserID {
		t.Fatalf("ResolveUserForPrincipal() = %#v, want user %s", resolved, user.UserID)
	}
	if !repo.updated {
		t.Fatal("expected stale Keycloak user ID to be repaired")
	}
	if user.KeycloakUserID != "fresh-subject" {
		t.Fatalf("stale keycloak user id not updated, got %q", user.KeycloakUserID)
	}
}

func TestResolveUserForPrincipal(t *testing.T) {
	user := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "subject"}
	for _, tc := range []struct {
		name      string
		repo      repository.UserRepository
		principal *Principal
		want      *model.UserModel
	}{
		{"missing_repository", nil, &Principal{Subject: "subject"}, nil},
		{"missing_principal", &userResolutionRepoStub{}, nil, nil},
		{"subject_match", &userResolutionRepoStub{bySubject: user}, &Principal{Subject: "subject"}, user},
		{"missing_username", &userResolutionRepoStub{}, &Principal{Subject: "subject"}, nil},
		{"unknown_user", &userResolutionRepoStub{}, &Principal{Subject: "subject", Username: "unknown@example.com"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := ResolveUserForPrincipal(tc.repo, tc.principal)
			if err != nil || resolved != tc.want {
				t.Fatalf("ResolveUserForPrincipal() = %v, %v; want %v, nil", resolved, err, tc.want)
			}
		})
	}
}
