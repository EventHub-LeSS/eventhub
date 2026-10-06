package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type EventHandler struct {
	eventService *service.EventService
}

func NewEventHandler(eventService *service.EventService) *EventHandler {
	return &EventHandler{eventService: eventService}
}

// managedOrganizationIDs returns the organizations in which the caller holds the event_manager
// role. It writes 401 or 403 and returns false if there are none.
func managedOrganizationIDs(c *gin.Context) ([]string, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return nil, false
	}

	managedOrgIDs := principal.OrganizationIDsWithRole(middleware.RoleEventManager)
	if len(managedOrgIDs) == 0 {
		writeProblem(c, http.StatusForbidden, "missing organization role")
		return nil, false
	}
	return managedOrgIDs, true
}

// EVENTHUB-75: Veranstaltung anlegen
func CreateEventHandler(c *gin.Context) {
}

// EVENTHUB-77: Veranstaltung als Entwurf speichern
// @Summary      Save event as draft
// @Description  Creates an event in status draft. Drafts are only visible to the organization that owns them and can be edited (PUT /events/{id}) and published (POST /events/{id}/publish) later. Requires the event_manager role in the owning organization. organizationId is the Keycloak organization ID or alias as returned by GET /users/me; it may be omitted when the caller manages events in exactly one organization.
// @Tags         events
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body   model.CreateDraftRequest true  "Draft event data"
// @Success      201 {object} model.EventModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      422 {object} model.ErrorResponse "Unknown categoryId or locationId"
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/draft [post]
func (h *EventHandler) SaveEventAsDraftHandler(c *gin.Context) {
	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	var req model.CreateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.eventService.CreateDraft(managedOrgIDs, req)
	if err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

// EVENTHUB-78: Veranstaltung bearbeiten
// @Summary      Update event
// @Description  Updates title, description, time span, location, category, price and capacity of an event. Requires the event_manager role in the organization that owns the event. Only events in status draft or published can be edited.
// @Tags         events
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id      path   string                   true  "Event ID"
// @Param        request body   model.UpdateEventRequest true  "Updated event data"
// @Success      200 {object} model.EventModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/{id} [put]
func (h *EventHandler) UpdateEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}

	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	var req model.UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.eventService.UpdateEvent(eventID, managedOrgIDs, req)
	if err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// EVENTHUB-76: Veranstaltung veröffentlichen
// @Summary      Publish event
// @Description  Publishes a draft event so that visitors can find and book it. Requires the event_manager role in the organization that owns the event. Only sufficiently complete events in status draft can be published. If the event is already published, returns 400 with detail "this event is already published".
// @Tags         events
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string                  true  "Event ID"
// @Success      200  {object} model.EventActionResponse
// @Failure      400  {object} model.ErrorResponse
// @Failure      401  {object} model.APIError
// @Failure      403  {object} model.ErrorResponse
// @Failure      404  {object} model.ErrorResponse
// @Failure      500  {object} model.ErrorResponse
// @Router       /events/{id}/publish [post]
func (h *EventHandler) PublishEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}

	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	if err := h.eventService.PublishEvent(eventID, managedOrgIDs); err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.EventActionResponse{Message: "event published"})
}

// EVENTHUB-82: Veranstaltung zurückziehen
// @Summary      Withdraw event
// @Description  Withdraws a published event so that it is no longer bookable. Requires the event_manager role in the organization that owns the event. Only events in status published can be withdrawn; the event status becomes cancelled.
// @Tags         events
// @Security     BearerAuth
// @Produce      json
// @Param        id   path   string                     true  "Event ID"
// @Success      200  {object} model.EventWithdrawnResponse
// @Failure      400  {object} model.ErrorResponse
// @Failure      401  {object} model.APIError
// @Failure      403  {object} model.ErrorResponse
// @Failure      404  {object} model.ErrorResponse
// @Failure      500  {object} model.ErrorResponse
// @Router       /events/{id}/withdraw [post]
func (h *EventHandler) WithdrawEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}

	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	if err := h.eventService.WithdrawEvent(eventID, managedOrgIDs); err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.EventWithdrawnResponse{Message: "event withdrawn"})
}

func writeEventActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeProblem(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrInvalidStatus):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrIncomplete):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotDraft):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotPublished):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrAlreadyPublished):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrOrganizationRequired):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrUnknownReference):
		writeProblem(c, http.StatusUnprocessableEntity, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}

// EVENTHUB-79: Eigene Veranstaltungen anzeigen
// EVENTHUB-77: eigene Entwürfe über ?status=draft
// @Summary      List own events
// @Description  Returns the events of all organizations in which the caller holds the event_manager role, ordered by start time. Use status=draft to list the own drafts. Drafts are never visible to other organizations or visitors.
// @Tags         events
// @Security     BearerAuth
// @Produce      json
// @Param        status query  string false "Only return events in this status" Enums(draft, published, cancelled, completed)
// @Success      200 {array}  model.EventModel "Own events; an empty array if there are none"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/self [get]
func (h *EventHandler) ListOwnEventsHandler(c *gin.Context) {
	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return
	}

	status := model.EventStatus(c.Query("status"))
	if status != "" && !model.IsValidEventStatus(status) {
		writeProblem(c, http.StatusBadRequest, "invalid status")
		return
	}

	events, err := h.eventService.ListOwnEvents(managedOrgIDs, status)
	if err != nil {
		writeEventActionError(c, err)
		return
	}
	if events == nil {
		events = []*model.EventModel{}
	}

	c.JSON(http.StatusOK, events)
}

// EVENTHUB-80: Verkaufte Tickets pro Veranstaltung anzeigen
// @Summary      Get sold tickets
// @Description  Returns the number of confirmed tickets sold for an event. Requires the event_manager role in the organization that owns the event.
// @Tags         events
// @Security     BearerAuth
// @Produce      json
// @Param        eventId path string true "Event ID"
// @Success      200 {object} model.SoldTicketsResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/{eventId}/sold-tickets [get]
func (h *EventHandler) GetSoldTicketsHandler(c *gin.Context) {
	statistics, ok := h.getEventStatistics(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, model.SoldTicketsResponse{
		EventID:     statistics.EventID,
		SoldTickets: statistics.SoldTickets,
	})
}

// EVENTHUB-81: Freie Plätze pro Veranstaltung anzeigen
// @Summary      Get available seats
// @Description  Returns the remaining available seats for an event. Requires the event_manager role in the organization that owns the event.
// @Tags         events
// @Security     BearerAuth
// @Produce      json
// @Param        eventId path string true "Event ID"
// @Success      200 {object} model.AvailableSeatsResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /events/{eventId}/available-seats [get]
func (h *EventHandler) GetAvailableSeatsHandler(c *gin.Context) {
	statistics, ok := h.getEventStatistics(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, model.AvailableSeatsResponse{
		EventID:        statistics.EventID,
		AvailableSeats: statistics.AvailableSeats,
	})
}

func (h *EventHandler) getEventStatistics(c *gin.Context) (*model.EventStatistics, bool) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "eventId must be a valid UUID")
		return nil, false
	}

	managedOrgIDs, ok := managedOrganizationIDs(c)
	if !ok {
		return nil, false
	}

	statistics, err := h.eventService.GetEventStatistics(
		eventID,
		managedOrgIDs,
	)
	if err != nil {
		writeEventActionError(c, err)
		return nil, false
	}

	return statistics, true
}

// ListPublishedEventsHandler handles EVENTHUB-206, EVENTHUB-207 and EVENTHUB-208.
// @Param location query string false "Case-insensitive substring of city or venue name; trimmed, empty means no filter, maximum 200 characters. Wildcards are treated literally."
// @Param categoryId query string false "Exact category UUID; trimmed, empty means no category filter. Combined with location using AND."
// @Summary List published events
// @Description Public list of published events with their category and location. Optional location searches city or venue name; categoryId selects an exact category. Both filters are combined using AND. Omit a filter or pass an empty value to reset it. Unknown categories and searches without matches return 200 with []. Invalid category UUIDs or location values exceeding 200 characters after trimming return 400. Events missing a category or location are omitted. Sorted by start time and event ID.
// @Tags events
// @Produce json
// @Success 200 {array} model.PublishedEventResponse
// @Failure 400 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /events [get]
func (h *EventHandler) ListPublishedEventsHandler(c *gin.Context) {
	location := strings.TrimSpace(c.Query("location"))
	if utf8.RuneCountInString(location) > 200 {
		writeProblem(c, http.StatusBadRequest, "location must not exceed 200 characters")
		return
	}
	filter := model.PublishedEventFilter{Location: location}
	if category := strings.TrimSpace(c.Query("categoryId")); category != "" {
		categoryID, err := uuid.Parse(category)
		if err != nil {
			writeProblem(c, http.StatusBadRequest, "categoryId must be a valid UUID")
			return
		}
		filter.CategoryID = &categoryID
	}
	events, err := h.eventService.ListPublishedEvents(filter)
	if err != nil {
		writeProblem(c, http.StatusInternalServerError, "internal error")
		return
	}
	c.JSON(http.StatusOK, events)
}

// GetPublishedEventDetailsHandler handles EVENTHUB-211.
// @Summary Get published event details
// @Description Public event details with description, category, location, price, capacity, availableSeats and bookable. Available seats account for confirmed tickets and unexpired reservations. Sold-out published events remain visible with bookable=false. Booking via POST /bookings requires authentication and checks availability again. Unpublished events and events missing category or location return 404.
// @Tags events
// @Produce json
// @Param eventId path string true "Event UUID"
// @Success 200 {object} model.PublishedEventDetailsResponse
// @Failure 400 {object} model.ErrorResponse
// @Failure 404 {object} model.ErrorResponse
// @Failure 500 {object} model.ErrorResponse
// @Router /events/{eventId} [get]
func (h *EventHandler) GetPublishedEventDetailsHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "eventId must be a valid UUID")
		return
	}
	details, err := h.eventService.GetPublishedEventDetails(eventID)
	if err != nil {
		writeEventActionError(c, err)
		return
	}
	c.JSON(http.StatusOK, details)
}
