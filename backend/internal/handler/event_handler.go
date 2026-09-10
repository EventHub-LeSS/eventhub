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
	managedOrgIDs := organizationIDsWithRole(principal, middleware.RoleEventManager)
	if len(managedOrgIDs) == 0 {
		writeProblem(c, http.StatusForbidden, "missing organization role")
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

// organizationIDsWithRole returns the Keycloak IDs of all organizations in which
// the principal holds the given role.
func organizationIDsWithRole(principal *middleware.Principal, role middleware.OrganizationRole) []string {
	ids := make([]string, 0, len(principal.Organizations))
	for _, org := range principal.Organizations {
		if _, ok := org.Roles[role]; ok {
			ids = append(ids, org.ID)
		}
	}
	return ids
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
