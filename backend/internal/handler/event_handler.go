package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var eventService *service.EventService

func SetEventService(s *service.EventService) {
	eventService = s
}

// EVENTHUB-75: Veranstaltung anlegen
func CreateEventHandler(c *gin.Context) {

}

// EVENTHUB-77: Veranstaltung als Entwurf speichern
func SaveEventAsDraftHandler(c *gin.Context) {

}

// EVENTHUB-78: Veranstaltung bearbeiten
func UpdateEventHandler(c *gin.Context) {
}

// EVENTHUB-76: Veranstaltung veröffentlichen
func PublishEventHandler(c *gin.Context) {

}

// EVENTHUB-82: Veranstaltung zurückziehen
func WithdrawEventHandler(c *gin.Context) {

}

// EVENTHUB-79: Eigene Veranstaltungen anzeigen
func ListOwnEventsHandler(c *gin.Context) {

}

// EVENTHUB-80: Verkaufte Tickets pro Veranstaltung anzeigen
func GetSoldTicketsHandler(c *gin.Context) {
	statistics, ok := getEventStatistics(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"eventId":     statistics.EventID,
		"soldTickets": statistics.SoldTickets,
	})
}

// EVENTHUB-81: Freie Plätze pro Veranstaltung anzeigen
func GetAvailableSeatsHandler(c *gin.Context) {
	statistics, ok := getEventStatistics(c)
	if !ok {
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"eventId":        statistics.EventID,
		"availableSeats": statistics.AvailableSeats,
	})
}

func getEventStatistics(c *gin.Context) (*model.EventStatistics, bool) {
	if eventService == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "event service is not configured",
		})
		return nil, false
	}

	eventID, err := uuid.Parse(c.Param("eventId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "eventId must be a valid UUID",
		})
		return nil, false
	}

	statistics, err := eventService.GetEventStatistics(eventID)
	if err != nil {
		if errors.Is(err, service.ErrEventNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "event not found",
			})
			return nil, false
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "could not load event statistics",
		})
		return nil, false
	}

	return statistics, true
}
