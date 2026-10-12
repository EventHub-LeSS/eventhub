package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// resolveAPIUser resolves the API user of an authenticated principal. Tokens carry the Keycloak
// user ID, while bookings and other API data reference the API user ID. It writes the problem
// response itself and reports false when the request cannot continue.
func resolveAPIUser(c *gin.Context, userRepo repository.UserRepository, principal *middleware.Principal) (*model.UserModel, bool) {
	user, err := middleware.ResolveUserForPrincipal(userRepo, principal)
	if err != nil {
		slog.Error("API user lookup failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
		return nil, false
	}
	if user == nil {
		writeProblem(c, http.StatusForbidden, "user not found in API database, run sync first")
		return nil, false
	}
	return user, true
}
