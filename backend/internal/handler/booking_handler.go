package handler

import (
	"backend/internal/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CancelBooking godoc
// @Summary      Buchung stornieren
// @Description  Storniert eine Buchung des Aufrufers und gibt deren Tickets frei. Eine Reservierung ohne Zahlung ist ein Warenkorb-Item und wird für sich storniert. Eine Buchung mit Zahlung wird mit dem ganzen Zahlungsvorgang storniert: Während die Zahlung offen ist, wird der gesamte Checkout storniert — es wurde nichts abgebucht — und nachdem sie bezahlt wurde, wird der Zeilenbetrag der Buchung zurückerstattet; der Zahlungsvorgang gilt als erstattet, sobald alle seine Buchungen storniert sind. Eine bezahlte Buchung einer vom Veranstalter abgesagten Veranstaltung ist nicht stornierbar (409), die Erstattung übernimmt der Veranstalter. Wiederholtes Stornieren gelingt ohne weitere Erstattung.
// @Tags         bookings
// @Produce      json
// @Param        bookingId path string true "Booking ID"
// @Success      200 {object} model.BookingModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Buchung in diesem Status nicht stornierbar oder Veranstaltung vom Veranstalter abgesagt"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings/{bookingId}/cancel [post]
func CancelBooking(bookingService *service.BookingService, userRepo repository.UserRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := middleware.PrincipalFromContext(c)
		if !ok {
			writeProblem(c, http.StatusUnauthorized, "authentication is required")
			return
		}
		bookingID, err := uuid.Parse(c.Param("bookingId"))
		if err != nil {
			writeProblem(c, http.StatusBadRequest, "invalid booking id")
			return
		}
		user, ok := resolveAPIUser(c, userRepo, principal)
		if !ok {
			return
		}
		booking, err := bookingService.CancelBooking(c.Request.Context(), bookingID, user.UserID)
		if err != nil {
			writeBookingError(c, err)
			return
		}
		c.JSON(http.StatusOK, booking)
	}
}

func writeBookingError(c *gin.Context, err error) {
	var capacityErr *service.CapacityExceededError
	switch {
	case errors.Is(err, service.ErrInvalidTicketCount):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrEventNotFound), errors.Is(err, service.ErrBookingNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrEventNotPublished), errors.Is(err, service.ErrBookingNotCancellable):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.As(err, &capacityErr):
		writeProblem(c, http.StatusConflict, err.Error())
	default:
		slog.Error("unexpected booking error", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
