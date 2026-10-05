package middleware

import (
	"backend/internal/model"
	"backend/internal/repository"
)

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
