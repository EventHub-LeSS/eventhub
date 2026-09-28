package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RecommendationsHandler struct {
	recommendationsService *service.RecommendationsService
	userRepo               repository.UserRepository
}

func NewRecommendationsHandler(recommendationsService *service.RecommendationsService, userRepo repository.UserRepository) *RecommendationsHandler {
	return &RecommendationsHandler{recommendationsService: recommendationsService, userRepo: userRepo}
}

func (h *RecommendationsHandler) resolveUserForPrincipal(principal *middleware.Principal) (*model.UserModel, error) {
	return resolveUserByPrincipal(h.userRepo, principal)
}

func resolveUserByPrincipal(userRepo repository.UserRepository, principal *middleware.Principal) (*model.UserModel, error) {
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

// @Summary      Get event recommendations
// @Description  Returns future published events with free capacity, excluding the user's confirmed bookings. Active reservations count toward capacity. Ranking weights category affinity (50%), confirmed ticket popularity (40%), and organizer ratings (10%).
// @Tags         recommendations
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array} model.EventModel
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /recommendations [get]
func (h *RecommendationsHandler) GetEventRecommendations(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}

	user, err := h.resolveUserForPrincipal(principal)
	if err != nil {
		slog.Error("recommendation user lookup failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "could not resolve recommendation user")
		return
	}

	if user == nil {
		writeProblem(c, http.StatusInternalServerError, "user not found in API database, run sync first")
		return
	}

	events, err := h.recommendationsService.GetRecommendationsForUser(user.UserID)
	if err != nil {
		slog.Error("event recommendations failed", "user_id", user.UserID, "error", err)
		writeProblem(c, http.StatusInternalServerError, "could not load event recommendations")
		return
	}

	c.JSON(http.StatusOK, events)
}
