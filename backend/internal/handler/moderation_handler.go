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

type EventModerationRequest struct {
	Reason string `json:"reason" binding:"required" minLength:"1" maxLength:"1000"`
}

// ListEventsHandler godoc
// @Summary      List events for moderation
// @Description  All event statuses and full descriptions, including withdrawn events. Requires admin or moderator. Newest first; organizationId is the local organization UUID.
// @Tags         moderation
// @Security     BearerAuth
// @Produce      json
// @Param        status          query string false "Event status" Enums(draft,published,cancelled,completed)
// @Param        organizationId  query string false "Organization UUID"
// @Param        page            query int false "Page" default(1) minimum(1)
// @Param        limit           query int false "Page size" default(50) minimum(1) maximum(100)
// @Success      200 {object} model.ModeratedEventPage
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      500 {object} model.ErrorResponse
// @Router       /moderation/events [get]
func (h *ModerationHandler) ListEventsHandler(c *gin.Context) {
	filter := model.ModeratedEventFilter{Status: model.EventStatus(c.Query("status"))}
	if filter.Status != "" && !model.IsValidEventStatus(filter.Status) {
		writeProblem(c, http.StatusBadRequest, "invalid event status")
		return
	}
	if value := c.Query("organizationId"); value != "" {
		organizationID, err := uuid.Parse(value)
		if err != nil {
			writeProblem(c, http.StatusBadRequest, "invalid organization id")
			return
		}
		filter.OrganizationID = &organizationID
	}
	page, limit, err := parsePageLimit(c)
	if err == nil && page-1 > math.MaxInt/limit {
		err = errors.New("page is too large")
	}
	if err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.moderationService.ListEvents(filter, page, limit)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to list events for moderation", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load events")
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetEventHandler godoc
// @Summary      Inspect an event for moderation
// @Description  Full event content in any status. Requires admin or moderator.
// @Tags         moderation
// @Security     BearerAuth
// @Produce      json
// @Param        eventId path string true "Event UUID"
// @Success      200 {object} model.ModeratedEvent
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /moderation/events/{eventId} [get]
func (h *ModerationHandler) GetEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}
	result, err := h.moderationService.GetEvent(eventID)
	writeEventModerationResult(c, result, err)
}

// EditEventContentHandler godoc
// @Summary      Correct problematic event text
// @Description  Requires admin or moderator and a reason of 1-1000 characters after trimming. Supply title (3-200 characters after trimming), description (0-5000 characters; empty clears it), or both. Other event fields and status stay unchanged. Works in any status. Changes, previous values, reason and actor are recorded atomically. An unchanged request returns changed=false without an audit entry.
// @Tags         moderation
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        eventId path string true "Event UUID"
// @Param        request body model.ModerateEventContentRequest true "Content correction and reason"
// @Success      200 {object} model.EventModerationResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /moderation/events/{eventId}/content [patch]
func (h *ModerationHandler) EditEventContentHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}
	var req model.ModerateEventContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "content and reason are required")
		return
	}
	if req.Title == nil && req.Description == nil {
		writeProblem(c, http.StatusBadRequest, "title or description is required")
		return
	}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if length := utf8.RuneCountInString(title); length < 3 || length > 200 {
			writeProblem(c, http.StatusBadRequest, "title must contain 3 to 200 characters")
			return
		}
		req.Title = &title
	}
	if req.Description != nil && utf8.RuneCountInString(*req.Description) > 5000 {
		writeProblem(c, http.StatusBadRequest, "description must contain at most 5000 characters")
		return
	}
	if !validateModerationReason(c, req.Reason) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	result, err := h.moderationService.EditEventContent(c.Request.Context(), eventID, req)
	writeEventModerationResult(c, result, err)
}

// WithdrawEventHandler godoc
// @Summary      Withdraw a problematic event
// @Description  Requires admin or moderator and a reason of 1-1000 characters after trimming. Draft and published events become cancelled and disappear from public lists, details and recommendations. New bookings and payment completion are rejected; existing bookings and payments remain in the history and tickets are invalid. No automatic refunds. Completed events return 409. Already cancelled events return changed=false without another audit entry. The status change, reason and actor are recorded atomically.
// @Tags         moderation
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        eventId path string true "Event UUID"
// @Param        request body EventModerationRequest true "Moderation reason"
// @Success      200 {object} model.EventModerationResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /moderation/events/{eventId}/withdraw [post]
func (h *ModerationHandler) WithdrawEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}
	var req EventModerationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "reason is required")
		return
	}
	if !validateModerationReason(c, req.Reason) {
		return
	}
	result, err := h.moderationService.WithdrawEvent(c.Request.Context(), eventID, strings.TrimSpace(req.Reason))
	writeEventModerationResult(c, result, err)
}

func validateModerationReason(c *gin.Context, reason string) bool {
	length := utf8.RuneCountInString(strings.TrimSpace(reason))
	if length < 1 || length > maxModerationReasonLength {
		writeProblem(c, http.StatusBadRequest, "reason must contain 1 to 1000 characters")
		return false
	}
	return true
}

func writeEventModerationResult(c *gin.Context, result any, err error) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, result)
	case errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrInvalidStatus):
		writeProblem(c, http.StatusConflict, "completed events cannot be withdrawn")
	case errors.Is(err, audit.ErrUnavailable), errors.Is(err, audit.ErrMissingContext):
		writeProblem(c, http.StatusInternalServerError, "the audit log is unavailable, the event was not changed")
	default:
		slog.ErrorContext(c.Request.Context(), "event moderation failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "event moderation failed")
	}
}

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
