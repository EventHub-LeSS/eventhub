package handler

import (
	"backend/internal/middleware"
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

func (h *RecommendationsHandler) GetEventRecommendations(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "autentication is required")
		return
	}

	user, err := h.userRepo.GetByKeycloakUserID(principal.Subject)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, err.Error())
		return
	}

	if user == nil {
		writeProblem(c, http.StatusInternalServerError, "user not found")
		return
	}

	events, err := h.recommendationsService.GetRecommendationsForUser(user.UserID)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, events)
}
