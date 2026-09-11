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

func organizationFromContext(c *gin.Context) (string, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return "", false
	}
	if principal.ActiveOrganization == nil {
		writeProblem(c, http.StatusForbidden, "no active organization")
		return "", false
	}
	if !principal.HasOrganizationRole(middleware.RoleEventManager) {
		writeProblem(c, http.StatusForbidden, "missing organization role")
		return "", false
	}
	return principal.ActiveOrganization.ID, true
}

func eventsOrEmpty(events []*model.EventModel) []*model.EventModel {
	if events == nil {
		return []*model.EventModel{}
	}
	return events
}

// EVENTHUB-75: Veranstaltung anlegen
func CreateEventHandler(c *gin.Context) {

}

// EVENTHUB-77: Veranstaltung als Entwurf speichern
func (h *EventHandler) SaveEventAsDraftHandler(c *gin.Context) {
	keycloakOrgID, ok := organizationFromContext(c)
	if !ok {
		return
	}

	var req model.CreateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.eventService.CreateDraft(keycloakOrgID, req)
	if err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusCreated, created)
}

func (h *EventHandler) ListOwnDraftsHandler(c *gin.Context) {
	keycloakOrgID, ok := organizationFromContext(c)
	if !ok {
		return
	}

	drafts, err := h.eventService.ListOwnEventsByStatus(keycloakOrgID, model.EventStatusDraft)
	if err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, eventsOrEmpty(drafts))
}

// EVENTHUB-78: Veranstaltung bearbeiten
func UpdateEventHandler(c *gin.Context) {
}

// EVENTHUB-76: Veranstaltung veröffentlichen
func (h *EventHandler) PublishEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invaild event id")
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

	if !principal.HasOrganizationRole(middleware.RoleEventManager) {
		writeProblem(c, http.StatusForbidden, "missing organization role")
		return
	}

	if err := h.eventService.PublishEvent(eventID, principal.ActiveOrganization.ID); err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "event published"})
}

// EVENTHUB-82: Veranstaltung zurückziehen
func (h *EventHandler) WithdrawEventHandler(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invaild event id")
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

	if !principal.HasOrganizationRole(middleware.RoleEventManager) {
		writeProblem(c, http.StatusForbidden, "missing organization role")
		return
	}

	if err := h.eventService.WithdrawEvent(eventID, principal.ActiveOrganization.ID); err != nil {
		writeEventActionError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "event withdrawn"})
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
	case errors.Is(err, service.ErrInvalidStatus):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrIncomplete):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotDraft):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrNotPublished):
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
