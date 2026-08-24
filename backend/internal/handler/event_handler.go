package handler

import (
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
		return
	}

	// TODO: replace with Keycloak/auth middleware
	userID, err := uuid.Parse(c.GetHeader("X-User-ID"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid user id"})
		return
	}

	var req model.UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := h.eventService.UpdateEvent(eventID, userID, req)
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

func writeEventActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrEventNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrIncomplete),
		errors.Is(err, service.ErrInvalidPrice), errors.Is(err, service.ErrStartInPast):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
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
