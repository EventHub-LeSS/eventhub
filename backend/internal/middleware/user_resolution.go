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

// DefaultGlobalRoleAssigner grants a Keycloak user their default global role, e.g. the visitor
// role every account starts with (EVENTHUB-299). The assigner itself decides which roles
// disqualify a user from the grant.
type DefaultGlobalRoleAssigner interface {
	EnsureDefaultGlobalRole(ctx context.Context, keycloakUserID string) error
}

// EnsureUserForPrincipal resolves the API user like ResolveUserForPrincipal and creates it from the
// Keycloak profile when the user registered in Keycloak but is not yet known to the API. Every user
// whose token carries no global role receives the default global role from the assigner: freshly
// provisioned users as well as accounts the startup sync created in the API database before their
// first request. Users whose token already carries a global role are skipped without a Keycloak
// lookup. A failed grant is only logged, the startup sync and the seed grant the role to users
// without any global role.
func EnsureUserForPrincipal(ctx context.Context, userRepo repository.UserRepository, profiles UserProfileSource, roles DefaultGlobalRoleAssigner, principal *Principal) (*model.UserModel, error) {
	user, err := ResolveUserForPrincipal(userRepo, principal)
	if err != nil || userRepo == nil || profiles == nil || principal == nil {
		return user, err
	}
	if user == nil {
		profile, profileErr := profiles.UserProfile(ctx, principal.Subject)
		if profileErr != nil || profile == nil || profile.Email == "" {
			return nil, profileErr
		}
		user = &model.UserModel{
			UserID:         uuid.New(),
			KeycloakUserID: principal.Subject,
			FirstName:      profile.FirstName,
			LastName:       profile.LastName,
			Email:          profile.Email,
		}
		if createErr := userRepo.Create(user); createErr != nil {
			// A concurrent first request may have created the user in the meantime.
			if existing, lookupErr := userRepo.GetByKeycloakUserID(principal.Subject); lookupErr == nil && existing != nil {
				user = existing
			} else {
				return nil, createErr
			}
		} else {
			slog.Info("provisioned API user from Keycloak", "user_id", user.UserID, "keycloak_user_id", user.KeycloakUserID)
		}
	}
	// The token is the cheap gate: a roleless token means the account was provisioned before it
	// held a role (startup sync) or its grant failed earlier, so the assigner runs until the
	// account converges on the default role and the next token carries it.
	if roles != nil && len(principal.GlobalRoles) == 0 {
		if grantErr := roles.EnsureDefaultGlobalRole(ctx, principal.Subject); grantErr != nil {
			slog.Warn("default global role grant failed", "keycloak_user_id", principal.Subject, "error", grantErr)
		}
	}
	return user, nil
}

// ProvisionAPIUser makes sure an authenticated Keycloak user also exists in the API database and
// starts with their default global role (EVENTHUB-299). Failures are only logged: handlers that
// need the API user report its absence themselves.
func ProvisionAPIUser(userRepo repository.UserRepository, profiles UserProfileSource, roles DefaultGlobalRoleAssigner) gin.HandlerFunc {
	return func(c *gin.Context) {
		if principal, ok := PrincipalFromContext(c); ok {
			if _, err := EnsureUserForPrincipal(c.Request.Context(), userRepo, profiles, roles, principal); err != nil {
				slog.Error("API user provisioning failed", "keycloak_user_id", principal.Subject, "error", err)
			}
		}
		c.Next()
	}
}
