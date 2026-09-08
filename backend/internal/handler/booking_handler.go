package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func CreateBookingHandler(bookingService *service.BookingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var booking model.BookingModel
		if err := c.ShouldBindJSON(&booking); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		created, err := bookingService.CreateBooking(&booking)
		if err != nil {
			if isBookingBusinessError(err.Error()) {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

var bookingBusinessErrors = []string{
	"event_id is required",
	"event not found",
	"event is not available for booking",
	"no available seats left",
}

func isBookingBusinessError(msg string) bool {
	for _, e := range bookingBusinessErrors {
		if strings.Contains(msg, e) {
			return true
		}
	}
	return false
}
