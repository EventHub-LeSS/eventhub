package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CreateBookingRequest struct {
	EventID         *uuid.UUID `json:"eventId" example:"550e8400-e29b-41d4-a716-446655440000"`
	NumberOfTickets int        `json:"numberOfTickets" example:"2"`
}

// CreateBookingHandler godoc
// @Summary      Buchung anlegen
// @Description  Legt eine neue Buchung für ein Event an
// @Tags         bookings
// @Accept       json
// @Produce      json
// @Param        booking body CreateBookingRequest true "Buchungsdaten"
// @Success      201 {object} model.BookingModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings [post]
func CreateBookingHandler(bookingService *service.BookingService) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFromContext(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, model.APIError{
				Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
			})
			return
		}
		userID, err := uuid.Parse(principal.Subject)
		if err != nil {
			c.JSON(http.StatusUnauthorized, model.APIError{
				Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Invalid user id in token"},
			})
			return
		}

		var req CreateBookingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{
				Type:   "about:blank",
				Title:  http.StatusText(http.StatusBadRequest),
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
			return
		}
		booking := model.BookingModel{
			EventID:         req.EventID,
			NumberOfTickets: req.NumberOfTickets,
			UserID:          &userID,
		}

		created, err := bookingService.CreateBooking(&booking)
		if err != nil {
			if isBookingBusinessError(err.Error()) {
				c.JSON(http.StatusBadRequest, model.ErrorResponse{
					Type:   "about:blank",
					Title:  http.StatusText(http.StatusBadRequest),
					Status: http.StatusBadRequest,
					Detail: err.Error(),
				})
				return
			}
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Type:   "about:blank",
				Title:  http.StatusText(http.StatusInternalServerError),
				Status: http.StatusInternalServerError,
				Detail: "internal server error",
			})
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

var bookingBusinessErrors = []string{
	"event_id is required",
	"number of tickets must be at least 1",
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
