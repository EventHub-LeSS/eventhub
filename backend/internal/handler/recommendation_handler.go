package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
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
// @Description  Returns a ranked list of recommended events for the authenticated user based on their historical bookings, category affinity, organizer affinity, and event popularity.
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
		writeProblem(c, http.StatusUnauthorized, "autentication is required")
		return
	}

	user, err := h.resolveUserForPrincipal(principal)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, err.Error())
		return
	}

	if user == nil {
		writeProblem(c, http.StatusInternalServerError, "user not found in API database, run sync first")
		return
	}

	events, err := h.recommendationsService.GetRecommendationsForUser(user.UserID)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, events)
}
