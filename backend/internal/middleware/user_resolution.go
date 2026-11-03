package middleware

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UserProfileSource loads the profile of a Keycloak user; it returns nil if the user does not exist.
type UserProfileSource interface {
	UserProfile(ctx context.Context, keycloakUserID string) (*model.UserModel, error)
}

// ResolveUserForPrincipal resolves the local API user for an authenticated principal.
// If needed, it falls back to the username as email and repairs a stale Keycloak user ID.
func ResolveUserForPrincipal(userRepo repository.UserRepository, principal *Principal) (*model.UserModel, error) {
	if userRepo == nil || principal == nil {
		return nil, nil
	}

	user, err := userRepo.GetByKeycloakUserID(principal.Subject)
	if err != nil || user != nil {
		return user, err
	}
	if principal.Username == "" {
		return nil, nil
	}

	user, err = userRepo.GetByEmail(principal.Username)
	if err != nil || user == nil {
		return user, err
	}
	if user.KeycloakUserID != principal.Subject {
		if err := userRepo.UpdateKeycloakUserID(user.UserID, principal.Subject); err != nil {
			return nil, err
		}
		user.KeycloakUserID = principal.Subject
	}
	return user, nil
}

// EnsureUserForPrincipal resolves the API user like ResolveUserForPrincipal and creates it from the
// Keycloak profile when the user registered in Keycloak but is not yet known to the API.
func EnsureUserForPrincipal(ctx context.Context, userRepo repository.UserRepository, profiles UserProfileSource, principal *Principal) (*model.UserModel, error) {
	user, err := ResolveUserForPrincipal(userRepo, principal)
	if err != nil || user != nil || userRepo == nil || profiles == nil || principal == nil {
		return user, err
	}

	profile, err := profiles.UserProfile(ctx, principal.Subject)
	if err != nil || profile == nil || profile.Email == "" {
		return nil, err
	}
	user = &model.UserModel{
		UserID:         uuid.New(),
		KeycloakUserID: principal.Subject,
		FirstName:      profile.FirstName,
		LastName:       profile.LastName,
		Email:          profile.Email,
	}
	if err := userRepo.Create(user); err != nil {
		// A concurrent first request may have created the user in the meantime.
		if existing, lookupErr := userRepo.GetByKeycloakUserID(principal.Subject); lookupErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	slog.Info("provisioned API user from Keycloak", "user_id", user.UserID, "keycloak_user_id", user.KeycloakUserID)
	return user, nil
}

// ProvisionAPIUser makes sure an authenticated Keycloak user also exists in the API database.
// Failures are only logged: handlers that need the API user report its absence themselves.
func ProvisionAPIUser(userRepo repository.UserRepository, profiles UserProfileSource) gin.HandlerFunc {
	return func(c *gin.Context) {
		if principal, ok := PrincipalFromContext(c); ok {
			if _, err := EnsureUserForPrincipal(c.Request.Context(), userRepo, profiles, principal); err != nil {
				slog.Error("API user provisioning failed", "keycloak_user_id", principal.Subject, "error", err)
			}
		}
		c.Next()
	}
}
