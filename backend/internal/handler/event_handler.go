package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type EventHandler struct {
	eventService *service.EventService
}

func NewEventHandler(eventService *service.EventService) *EventHandler {
	return &EventHandler{eventService: eventService}
}

// EVENTHUB-75: Veranstaltung anlegen
func CreateEventHandler(c *gin.Context) {

}

// EVENTHUB-77: Veranstaltung als Entwurf speichern
func SaveEventAsDraftHandler(c *gin.Context) {

}

// EVENTHUB-78: Veranstaltung bearbeiten
func (h *EventHandler) UpdateEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid event id")
		return
	}

	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}
	if principal.ActiveOrganization == nil {
		writeProblem(c, http.StatusForbidden, "no active organization")
		return
	}
	if !principal.HasOrganizationRole(middleware.RoleEventManager) &&
		!principal.HasOrganizationRole(middleware.RoleOrganizationAdmin) {
		writeProblem(c, http.StatusForbidden, "missing organization role")
		return
	}

	var req model.UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.eventService.UpdateEvent(eventID, principal.ActiveOrganization.ID, req)
	if err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, updated)
}

// EVENTHUB-76: Veranstaltung veröffentlichen
func PublishEventHandler(c *gin.Context) {

}

// EVENTHUB-82: Veranstaltung zurückziehen
func WithdrawEventHandler(c *gin.Context) {

}

func writeProblem(c *gin.Context, status int, detail string) {
	c.JSON(status, model.ErrorResponse{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}

func writeEventActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrForbidden):
		writeProblem(c, http.StatusForbidden, err.Error())
	case errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrIncomplete),
		errors.Is(err, service.ErrInvalidPrice), errors.Is(err, service.ErrStartInPast):
		writeProblem(c, http.StatusBadRequest, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}

// EVENTHUB-79: Eigene Veranstaltungen anzeigen
func ListOwnEventsHandler(c *gin.Context) {

}

// EVENTHUB-80: Verkaufte Tickets pro Veranstaltung anzeigen
func GetSoldTicketsHandler(c *gin.Context) {
}

// EVENTHUB-81: Freie Plätze pro Veranstaltung anzeigen
func GetAvailableSeatsHandler(c *gin.Context) {

}
