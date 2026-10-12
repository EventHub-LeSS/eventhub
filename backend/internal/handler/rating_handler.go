package handler

import (
	"backend/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type RatingHandler struct {
	ratingService *service.RatingService
}

func NewRatingHandler(ratingService *service.RatingService) *RatingHandler {
	return &RatingHandler{ratingService: ratingService}
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
