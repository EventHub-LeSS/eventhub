package middleware

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"testing"

	"github.com/google/uuid"
)

type userResolutionRepoStub struct {
	repository.UserRepository
	bySubject *model.UserModel
	byEmail   *model.UserModel
	updated   bool
	created   *model.UserModel
}

func (s *userResolutionRepoStub) Create(user *model.UserModel) error {
	s.created = user
	return nil
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

type userProfileStub struct {
	profile *model.UserModel
	calls   int
}

func (s *userProfileStub) UserProfile(_ context.Context, keycloakUserID string) (*model.UserModel, error) {
	s.calls++
	if s.profile != nil && s.profile.KeycloakUserID == keycloakUserID {
		return s.profile, nil
	}
	return nil, nil
}

func TestEnsureUserForPrincipalProvisionsNewKeycloakUser(t *testing.T) {
	repo := &userResolutionRepoStub{}
	profiles := &userProfileStub{profile: &model.UserModel{KeycloakUserID: "new-subject", FirstName: "New", LastName: "User", Email: "new@example.com"}}
	principal := &Principal{Subject: "new-subject", Username: "new@example.com"}

	resolved, err := EnsureUserForPrincipal(context.Background(), repo, profiles, principal)
	if err != nil {
		t.Fatalf("EnsureUserForPrincipal() error = %v", err)
	}
	if resolved == nil || repo.created != resolved {
		t.Fatalf("EnsureUserForPrincipal() = %#v, want provisioned user", resolved)
	}
	if resolved.UserID == uuid.Nil || resolved.KeycloakUserID != "new-subject" || resolved.Email != "new@example.com" ||
		resolved.FirstName != "New" || resolved.LastName != "User" {
		t.Fatalf("provisioned user = %#v", resolved)
	}
}

func TestEnsureUserForPrincipalSkipsKnownAndUnknownUsers(t *testing.T) {
	known := &model.UserModel{UserID: uuid.New(), KeycloakUserID: "subject"}
	repo := &userResolutionRepoStub{bySubject: known}
	profiles := &userProfileStub{}
	resolved, err := EnsureUserForPrincipal(context.Background(), repo, profiles, &Principal{Subject: "subject"})
	if err != nil || resolved != known || profiles.calls != 0 || repo.created != nil {
		t.Fatalf("known user: got %v, %v, %d profile calls, created %v", resolved, err, profiles.calls, repo.created)
	}

	repo = &userResolutionRepoStub{}
	resolved, err = EnsureUserForPrincipal(context.Background(), repo, profiles, &Principal{Subject: "missing"})
	if err != nil || resolved != nil || repo.created != nil {
		t.Fatalf("user missing in Keycloak: got %v, %v, created %v", resolved, err, repo.created)
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
