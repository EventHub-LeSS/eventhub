package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB-236: Meine Tickets und Buchungshistorie
type OwnBookingHandler struct {
	ownBookingService *service.OwnBookingService
	userRepo          repository.UserRepository
}

func NewOwnBookingHandler(ownBookingService *service.OwnBookingService, userRepo repository.UserRepository) *OwnBookingHandler {
	return &OwnBookingHandler{ownBookingService: ownBookingService, userRepo: userRepo}
}

// ListOwnTicketsHandler godoc
// @Summary      Meine Tickets
// @Description  Liefert die gültigen Tickets des Aufrufers: nur Buchungen mit ticketsValid true, also bezahlt, bestätigt und für ein veröffentlichtes oder abgeschlossenes Event. Kommende Events zuerst nach Startzeit aufsteigend, danach vergangene nach Startzeit absteigend; ein Event gilt als kommend, bis es nach der Datenbankuhr geendet hat. Jeder Eintrag enthält bookingId für GET /me/bookings/{bookingId} und ratingScore, den Score, mit dem der Aufrufer das Event bewertet hat, oder null, wenn noch nicht bewertet; die Bewertung zählt pro Nutzer und Event. Ticketliste, Buchungshistorie, Ticketdetails und GET /bookings/{bookingId}/payment wenden dieselbe Gültigkeitsregel an. Ein leeres Array, wenn es keine gültigen Tickets gibt.
// @Tags         bookings
// @Produce      json
// @Success      200 {array}  model.OwnBookingResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse "Nutzer nicht in der API-Datenbank synchronisiert"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /me/tickets [get]
func (h *OwnBookingHandler) ListOwnTicketsHandler(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	tickets, err := h.ownBookingService.ListOwnTickets(user.UserID)
	if err != nil {
		writeOwnBookingError(c, err)
		return
	}
	c.JSON(http.StatusOK, tickets)
}

// ListOwnBookingsHandler godoc
// @Summary      Meine Buchungshistorie
// @Description  Liefert alle Buchungen des Aufrufers in jedem Status, die neueste zuerst, mit Event, Ort, Ticketanzahl, Buchungs- und Zahlungsstatus, failureReason und ticketsValid. bookingStatus ist der gespeicherte Status, nur eine nach der Datenbankuhr abgelaufene Reservierung wird als expired angezeigt. GET /bookings/{bookingId}/payment zeigt dagegen den gespeicherten Status reserved, bei offener Zahlung bis zum Cleanup der Zahlungen nach höchstens etwa einer Minute; ohne Zahlung liefert es 404. status filtert auf den angezeigten Status, expired findet also gespeicherte und abgeleitete abgelaufene Buchungen. Ein leeres Array, wenn es keine Buchungen gibt.
// @Tags         bookings
// @Produce      json
// @Param        status query string false "Nur Buchungen mit diesem angezeigten Status" Enums(reserved, confirmed, cancelled, failed, expired)
// @Success      200 {array}  model.OwnBookingResponse
// @Failure      400 {object} model.ErrorResponse "Ungültiger status"
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse "Nutzer nicht in der API-Datenbank synchronisiert"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /me/bookings [get]
func (h *OwnBookingHandler) ListOwnBookingsHandler(c *gin.Context) {
	if _, ok := middleware.PrincipalFromContext(c); !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}
	status := model.BookingStatus(c.Query("status"))
	if status != "" && !model.IsValidBookingStatus(status) {
		writeProblem(c, http.StatusBadRequest, "invalid status")
		return
	}
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	bookings, err := h.ownBookingService.ListOwnBookings(user.UserID, status)
	if err != nil {
		writeOwnBookingError(c, err)
		return
	}
	c.JSON(http.StatusOK, bookings)
}

// GetOwnBookingHandler godoc
// @Summary      Ticketdetails einer eigenen Buchung
// @Description  Liefert eine Buchung des Aufrufers in jedem Status mit Event, Ort, Ticketanzahl, Buchungsstatus, Zahlungsstatus, Betrag, failureReason und ticketsValid. Ungültige, stornierte oder abgelaufene Buchungen sowie Tickets abgesagter Events sind mit ticketsValid false markiert. ticketsValid, paymentStatus, failureReason und message entsprechen GET /bookings/{bookingId}/payment, weil beide dieselbe Regel und dieselben Texte nutzen. bookingStatus zeigt eine nach der Datenbankuhr abgelaufene Reservierung als expired, GET /bookings/{bookingId}/payment den gespeicherten Status, bei offener Zahlung bis zum Cleanup nach höchstens etwa einer Minute. Unbekannte Buchungen und Buchungen anderer Nutzer liefern beide 404.
// @Tags         bookings
// @Produce      json
// @Param        bookingId path string true "Booking ID"
// @Success      200 {object} model.OwnBookingResponse
// @Failure      400 {object} model.ErrorResponse "Ungültige Booking ID"
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse "Nutzer nicht in der API-Datenbank synchronisiert"
// @Failure      404 {object} model.ErrorResponse "Buchung unbekannt oder von einem anderen Nutzer"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /me/bookings/{bookingId} [get]
func (h *OwnBookingHandler) GetOwnBookingHandler(c *gin.Context) {
	if _, ok := middleware.PrincipalFromContext(c); !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}
	bookingID, err := uuid.Parse(c.Param("bookingId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid booking id")
		return
	}
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	booking, err := h.ownBookingService.GetOwnBooking(bookingID, user.UserID)
	if err != nil {
		writeOwnBookingError(c, err)
		return
	}
	c.JSON(http.StatusOK, booking)
}

// resolveUser resolves the API user of the caller from the token, never from the request. It writes
// the problem response itself and reports false when the request cannot continue.
func (h *OwnBookingHandler) resolveUser(c *gin.Context) (*model.UserModel, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return nil, false
	}
	return resolveAPIUser(c, h.userRepo, principal)
}

// writeOwnBookingError hides database errors from the caller; they are logged on the server.
func writeOwnBookingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBookingNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	default:
		slog.Error("unexpected own booking error", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
