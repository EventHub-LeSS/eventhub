package handler

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/service"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ModerationHandler struct {
	moderationService *service.ModerationService
}

func NewModerationHandler(moderationService *service.ModerationService) *ModerationHandler {
	return &ModerationHandler{moderationService: moderationService}
}

// RatingVisibilityRequest hides (hidden=true) or shows (hidden=false) a rating.
type RatingVisibilityRequest struct {
	Hidden *bool  `json:"hidden" binding:"required" example:"true"`
	Reason string `json:"reason" binding:"required" minLength:"1" maxLength:"1000" example:"Beleidigender Inhalt"`
}

const maxModerationReasonLength = 1000

// ListRatingsHandler lists ratings for moderation.
// @Summary      List ratings for moderation
// @Description  Returns a page of all ratings including their text, newest first, with event and organization. Requires the global role admin or moderator. Unlike the public summaries, hidden ratings and ratings of any booking status are included; hidden filters by visibility, eventId by rated event. eventId, eventTitle, organizationId and organizationName are null if the rating no longer belongs to an event or organization. Hidden ratings are released with PUT /moderation/ratings/{ratingId}/visibility and hidden=false.
// @Tags         moderation
// @Security     BearerAuth
// @Produce      json
// @Param        hidden   query  bool    false  "Only hidden (true) or only visible (false) ratings"
// @Param        eventId  query  string  false  "Only ratings of this event"
// @Param        page     query  int     false  "Page number (1-indexed)" default(1) minimum(1)
// @Param        limit    query  int     false  "Page size (1-100)" default(50) minimum(1) maximum(100)
// @Success      200  {object} model.ModeratedRatingPage
// @Failure      400  {object} model.ErrorResponse
// @Failure      401  {object} model.APIError
// @Failure      403  {object} model.APIError
// @Failure      500  {object} model.ErrorResponse
// @Router       /moderation/ratings [get]
func (h *ModerationHandler) ListRatingsHandler(c *gin.Context) {
	var filter model.ModeratedRatingFilter
	switch c.Query("hidden") {
	case "":
	case "true", "false":
		hidden := c.Query("hidden") == "true"
		filter.Hidden = &hidden
	default:
		writeProblem(c, http.StatusBadRequest, "hidden must be true or false")
		return
	}
	if value := c.Query("eventId"); value != "" {
		eventID, err := uuid.Parse(value)
		if err != nil {
			writeProblem(c, http.StatusBadRequest, "invalid event id")
			return
		}
		filter.EventID = &eventID
	}
	page, limit, err := parsePageLimit(c)
	if err == nil && page-1 > math.MaxInt/limit {
		err = errors.New("page is too large")
	}
	if err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	result, err := h.moderationService.ListRatings(filter, page, limit)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to list ratings for moderation", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load ratings")
		return
	}
	c.JSON(http.StatusOK, result)
}

// SetRatingVisibilityHandler hides or shows a rating.
// @Summary      Hide or release a rating
// @Description  Hides (hidden=true) a rating or releases it again (hidden=false), which withdraws an earlier hiding. Requires the global role admin or moderator. Hidden ratings no longer count towards rating summaries and recommendations. The reason (1-1000 characters after trimming) is required and is stored with the acting user in the audit log of the organization that owns the rated event (organization_id "platform" if the rating no longer belongs to an organization). Requesting the current visibility returns changed=false and records nothing.
// @Tags         moderation
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        ratingId  path  string                   true  "Rating ID"
// @Param        request   body  RatingVisibilityRequest  true  "Visibility and reason"
// @Success      200  {object} model.RatingVisibilityResponse
// @Failure      400  {object} model.ErrorResponse
// @Failure      401  {object} model.APIError
// @Failure      403  {object} model.APIError
// @Failure      404  {object} model.ErrorResponse
// @Failure      500  {object} model.ErrorResponse
// @Router       /moderation/ratings/{ratingId}/visibility [put]
func (h *ModerationHandler) SetRatingVisibilityHandler(c *gin.Context) {
	ratingID, err := uuid.Parse(c.Param("ratingId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid rating id")
		return
	}
	var req RatingVisibilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "hidden and reason are required")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxModerationReasonLength {
		writeProblem(c, http.StatusBadRequest, "reason must contain 1 to 1000 characters")
		return
	}

	result, err := h.moderationService.SetRatingVisibility(c.Request.Context(), ratingID, *req.Hidden, reason)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, service.ErrRatingNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, audit.ErrUnavailable), errors.Is(err, audit.ErrMissingContext):
		writeProblem(c, http.StatusInternalServerError, "the audit log is unavailable, the visibility was not changed")
	default:
		writeProblem(c, http.StatusInternalServerError, "failed to change the visibility of the rating")
	}
}
