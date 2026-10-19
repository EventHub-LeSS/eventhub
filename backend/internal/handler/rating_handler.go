package handler

import (
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RatingHandler struct {
	ratingService *service.RatingService
	userRepo      repository.UserRepository
}

func NewRatingHandler(ratingService *service.RatingService, userRepo repository.UserRepository) *RatingHandler {
	return &RatingHandler{ratingService: ratingService, userRepo: userRepo}
}

type CreateEventRatingRequest struct {
	Score int    `json:"score" binding:"required,gte=1,lte=5" example:"5"`
	Text  string `json:"text" example:"Great event!"`
}

// @Summary      Rate an event
// @Description  Creates one rating per authenticated visitor and event. The visitor must have a confirmed booking for this event. Text is optional; score must be between 1 and 5.
// @Tags         events
// @Accept       json
// @Produce      json
// @Param        eventId path string true "Event UUID"
// @Param        rating body CreateEventRatingRequest true "Rating"
// @Success      201 {object} model.RatingModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /events/{eventId}/ratings [post]
func (h *RatingHandler) CreateEventRatingHandler(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}

	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "eventId must be a valid UUID")
		return
	}

	var req CreateEventRatingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	user, ok := resolveAPIUser(c, h.userRepo, principal)
	if !ok {
		return
	}

	rating, err := h.ratingService.CreateEventRating(c.Request.Context(), user.UserID, eventID, req.Score, req.Text)
	switch {
	case errors.Is(err, service.ErrInvalidRating):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrRatingBookingRequired):
		writeProblem(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrEventAlreadyRated):
		writeProblem(c, http.StatusConflict, err.Error())
	case err != nil:
		slog.Error("create event rating failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	default:
		c.JSON(http.StatusCreated, rating)
	}
}

// EVENTHUB-238: Bewertungsdurchschnitt für Veranstaltungen.
// @Summary      Get event rating summary
// @Description  Public average score and review count. Only visible ratings with scores 1–5 linked to confirmed bookings of this event count; ticket quantities do not weight reviews. No valid ratings returns averageRating=null and ratingCount=0. Published events and ended completed events expose summaries; drafts and cancelled events return 404. Averages are not rounded.
// @Tags         events
// @Produce      json
// @Param        eventId path string true "Event UUID"
// @Success      200 {object} model.EventRatingSummary
// @Failure      400 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/{eventId}/rating-summary [get]
func (h *RatingHandler) GetEventRatingSummaryHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "eventId must be a valid UUID")
		return
	}
	summary, err := h.ratingService.GetEventRatingSummary(eventID)
	if err != nil {
		writeEventActionError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}

// EVENTHUB-239: Bewertungsdurchschnitt für Organisator/Veranstalter.
// @Summary      Get organizer rating summary
// @Description  Public average of unrounded event averages, with equal weight per rated event. Includes only this organization's published or completed events with endTime <= database time. Uses the same visible, score 1–5, confirmed-booking ratings as event summaries. Unrated events do not contribute. No rated events returns averageRating=null and ratedEventCount=0. Recommendations retain their existing calculation.
// @Tags         organizations
// @Produce      json
// @Param        organizationID path string true "Organization database UUID (not Keycloak ID or alias)"
// @Success      200 {object} model.OrganizerRatingSummary
// @Failure      400 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/rating-summary [get]
func (h *RatingHandler) GetOrganizerRatingSummaryHandler(c *gin.Context) {
	organizationID, err := uuid.Parse(c.Param("organizationID"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "organizationID must be a valid UUID")
		return
	}
	summary, err := h.ratingService.GetOrganizerRatingSummary(organizationID)
	if err != nil {
		writeEventActionError(c, err)
		return
	}
	c.JSON(http.StatusOK, summary)
}
