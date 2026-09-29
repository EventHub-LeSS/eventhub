package handler

import (
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CreateBookingRequest struct {
	EventID         *uuid.UUID `json:"eventId" binding:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	NumberOfTickets int        `json:"numberOfTickets" binding:"required,gte=1" example:"2"`
}

// CreateBookingHandler godoc
// @Summary      Buchung anlegen
// @Description  Reserviert Tickets für ein veröffentlichtes Event. Die Reservierung verfällt nach Ablauf von expiresAt, solange sie nicht bestätigt wurde.
// @Tags         bookings
// @Accept       json
// @Produce      json
// @Param        booking body CreateBookingRequest true "Buchungsdaten"
// @Success      201 {object} model.BookingModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Event nicht veröffentlicht oder Kapazität überschritten"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings [post]
func CreateBookingHandler(bookingService *service.BookingService, userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFromContext(c)
		if !ok {
			writeProblem(c, http.StatusUnauthorized, "authentication is required")
			return
		}

		var req CreateBookingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			writeProblem(c, http.StatusBadRequest, err.Error())
			return
		}

		// The token subject is the Keycloak user id, bookings reference the API user id.
		user, err := userRepo.GetByKeycloakUserID(principal.Subject)
		if err != nil {
			writeProblem(c, http.StatusInternalServerError, "internal error")
			return
		}
		if user == nil {
			writeProblem(c, http.StatusForbidden, "user not found in API database, run sync first")
			return
		}

		created, err := bookingService.ReserveTickets(c.Request.Context(), *req.EventID, user.UserID, req.NumberOfTickets)
		if err != nil {
			writeBookingError(c, err)
			return
		}

		c.JSON(http.StatusCreated, created)
	}
}

func writeBookingError(c *gin.Context, err error) {
	var capacityErr *service.CapacityExceededError
	switch {
	case errors.Is(err, service.ErrInvalidTicketCount):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrEventNotPublished):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.As(err, &capacityErr):
		writeProblem(c, http.StatusConflict, err.Error())
	default:
		log.Printf("unexpected booking error: %V", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
