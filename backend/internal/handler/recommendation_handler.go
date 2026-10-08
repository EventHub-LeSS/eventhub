package handler

import (
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RecommendationsHandler struct {
	recommendationsService *service.RecommendationsService
	userRepo               repository.UserRepository
}

func NewRecommendationsHandler(recommendationsService *service.RecommendationsService, userRepo repository.UserRepository) *RecommendationsHandler {
	return &RecommendationsHandler{recommendationsService: recommendationsService, userRepo: userRepo}
}

// @Summary      Get event recommendations
// @Description  Returns future published events with free capacity, excluding the user's confirmed bookings. Active reservations count toward capacity. Ranking weights category affinity (50%), confirmed ticket popularity (40%), and organizer ratings (10%).
// @Tags         recommendations
// @Security     BearerAuth
// @Produce      json
// @Param        limit query int false "Maximum number of recommendations after ranking; omitted returns all recommendations" minimum(1)
// @Success      200 {array} model.EventModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /recommendations [get]
func (h *RecommendationsHandler) GetEventRecommendations(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}

	limit := 0
	if values := c.Request.URL.Query()["limit"]; len(values) > 0 {
		var err error
		limit, err = strconv.Atoi(values[0])
		if len(values) != 1 || err != nil || limit <= 0 {
			writeProblem(c, http.StatusBadRequest, "limit must be a single positive integer")
			return
		}
	}

	user, err := middleware.ResolveUserForPrincipal(h.userRepo, principal)
	if err != nil {
		slog.Error("recommendation user lookup failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "could not resolve recommendation user")
		return
	}

	if user == nil {
		writeProblem(c, http.StatusForbidden, "user not found in API database, run sync first")
		return
	}

	events, err := h.recommendationsService.GetRecommendationsForUser(user.UserID)
	if err != nil {
		slog.Error("event recommendations failed", "user_id", user.UserID, "error", err)
		writeProblem(c, http.StatusInternalServerError, "could not load event recommendations")
		return
	}

	// Limit only after the service has ranked all eligible events.
	if limit > 0 && limit < len(events) {
		events = events[:limit]
	}

	c.JSON(http.StatusOK, events)
}
