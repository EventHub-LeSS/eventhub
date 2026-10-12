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
	"github.com/shopspring/decimal"
)

// maxSimulatePaymentBody bounds the simulate request body, which is a few dozen bytes. Decoding
// a decimal grows superlinearly with its digits, so the limit also bounds the cost of amount.
const maxSimulatePaymentBody = 4 << 10

// EVENTHUB-235: Zahlungsvorgang
type SimulatePaymentRequest struct {
	// Outcome the simulated payment provider reports; no real payment provider is connected.
	Outcome model.SimulatedPaymentOutcome `json:"outcome" binding:"required,oneof=success failure" example:"success"`
	// Amount shown to the user before completing the payment, as JSON number or string; it must
	// equal the stored amount.
	Amount *decimal.Decimal `json:"amount" binding:"required,gte=0" swaggertype:"number" example:"40"`
}

type PaymentHandler struct {
	paymentService *service.PaymentService
	userRepo       repository.UserRepository
}

func NewPaymentHandler(paymentService *service.PaymentService, userRepo repository.UserRepository) *PaymentHandler {
	return &PaymentHandler{paymentService: paymentService, userRepo: userRepo}
}

// StartPaymentHandler godoc
// @Summary      Zahlungsvorgang starten
// @Description  Überführt eine Reservierung des Aufrufers in einen Zahlungsvorgang. Der Betrag ergibt sich aus Ticketanzahl mal Veranstaltungspreis, wird gespeichert und vor Abschluss angezeigt; Beträge vom Client werden ignoriert. Ein erneuter Aufruf liefert den offenen Zahlungsvorgang unverändert zurück (200). Die Zahlung muss vor expiresAt abgeschlossen werden. Ist die Reservierung abgelaufen oder das Event zurückgezogen, wird eine offene Zahlung dabei als fehlgeschlagen gespeichert und 409 geliefert. Buchungen anderer Nutzer liefern 404.
// @Tags         payments
// @Produce      json
// @Param        bookingId path string true "Booking ID"
// @Success      200 {object} model.PaymentResponse "Offener Zahlungsvorgang existierte bereits"
// @Success      201 {object} model.PaymentResponse "Zahlungsvorgang angelegt"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Buchung nicht reserviert, Reservierung abgelaufen, Event nicht mehr verfügbar oder Zahlung bereits abgeschlossen"
// @Failure      422 {object} model.ErrorResponse "Betrag außerhalb des zulässigen Bereichs"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings/{bookingId}/payment [post]
func (h *PaymentHandler) StartPaymentHandler(c *gin.Context) {
	bookingID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	payment, created, err := h.paymentService.StartPayment(c.Request.Context(), bookingID, user.UserID)
	if err != nil {
		writePaymentError(c, err)
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, payment)
}

// SimulatePaymentHandler godoc
// @Summary      Zahlung simulieren
// @Description  Schließt den offenen Zahlungsvorgang mit einem simulierten Ergebnis ab; es ist kein echter Zahlungsdienstleister angebunden. amount muss dem gespeicherten Betrag entsprechen, sonst wird nichts geändert (422). Bei success wird die Buchung verbindlich bestätigt (bookingStatus confirmed, ticketsValid true). Bei failure, abgelaufener Reservierung oder zurückgezogenem Event schlägt die Zahlung fehl (status failed, failureReason, message), die Buchung erhält keinen Ticketanspruch und die Tickets werden freigegeben. Das Ergebnis wird innerhalb der Transaktion aus dem gespeicherten Zahlungsstatus gelesen. Der Body darf höchstens 4 KiB groß sein (413).
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        bookingId path string true "Booking ID"
// @Param        payment body SimulatePaymentRequest true "Simuliertes Ergebnis und angezeigter Betrag"
// @Success      200 {object} model.PaymentResponse "Ergebnis des Zahlungsvorgangs, erfolgreich oder fehlgeschlagen"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Buchung nicht gefunden oder kein Zahlungsvorgang gestartet"
// @Failure      409 {object} model.ErrorResponse "Zahlung bereits abgeschlossen oder Buchung nicht reserviert"
// @Failure      413 {object} model.ErrorResponse "Request Body größer als 4 KiB"
// @Failure      422 {object} model.ErrorResponse "Betrag weicht vom gespeicherten Betrag ab"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings/{bookingId}/payment/simulate [post]
func (h *PaymentHandler) SimulatePaymentHandler(c *gin.Context) {
	bookingID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSimulatePaymentBody)
	var req SimulatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeProblem(c, http.StatusRequestEntityTooLarge, "request body must not exceed 4 KiB")
			return
		}
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	payment, err := h.paymentService.SimulatePayment(c.Request.Context(), bookingID, user.UserID, req.Outcome, *req.Amount)
	if err != nil {
		writePaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, payment)
}

// GetPaymentHandler godoc
// @Summary      Zahlungsvorgang abrufen
// @Description  Liefert den gespeicherten Zahlungsvorgang einer Buchung des Aufrufers: Betrag, Zahlungsstatus, Buchungsstatus und eine verständliche Meldung. ticketsValid ist nur bei bezahlter und bestätigter Buchung true, deren Event stattfindet oder stattgefunden hat. Buchung, Zahlung, Event-Status und Datenbankuhr stammen aus einer Abfrage. Offene Zahlungen abgelaufener Reservierungen schließt ein Hintergrundjob jede Minute als fehlgeschlagen ab.
// @Tags         payments
// @Produce      json
// @Param        bookingId path string true "Booking ID"
// @Success      200 {object} model.PaymentResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Buchung nicht gefunden oder kein Zahlungsvorgang gestartet"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /bookings/{bookingId}/payment [get]
func (h *PaymentHandler) GetPaymentHandler(c *gin.Context) {
	bookingID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	payment, err := h.paymentService.GetPayment(c.Request.Context(), bookingID, user.UserID)
	if err != nil {
		writePaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, payment)
}

// resolveRequest parses the booking ID and resolves the API user of the caller. It writes the
// problem response itself and reports false when the request cannot continue.
func (h *PaymentHandler) resolveRequest(c *gin.Context) (uuid.UUID, *model.UserModel, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return uuid.Nil, nil, false
	}

	bookingID, err := uuid.Parse(c.Param("bookingId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid booking id")
		return uuid.Nil, nil, false
	}

	user, ok := resolveAPIUser(c, h.userRepo, principal)
	return bookingID, user, ok
}

func writePaymentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidPaymentOutcome):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrBookingNotFound), errors.Is(err, service.ErrPaymentNotStarted):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrBookingNotPayable),
		errors.Is(err, service.ErrReservationExpired),
		errors.Is(err, service.ErrEventUnavailable),
		errors.Is(err, service.ErrPaymentAlreadyProcessed):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrPaymentAmountMismatch),
		errors.Is(err, service.ErrInvalidPaymentAmount),
		errors.Is(err, service.ErrInvalidTicketCount):
		writeProblem(c, http.StatusUnprocessableEntity, err.Error())
	default:
		slog.Error("unexpected payment error", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
