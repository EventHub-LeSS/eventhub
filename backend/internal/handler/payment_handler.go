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

// EVENTHUB-235: Zahlungsvorgang; 282: Zahlung eines ganzen Warenkorb-Checkouts
type SimulatePaymentRequest struct {
	// Outcome the simulated payment provider reports; no real payment provider is connected.
	Outcome model.SimulatedPaymentOutcome `json:"outcome" binding:"required,oneof=success failure" example:"success"`
	// Amount shown to the user before completing the payment, as JSON number or string; it must
	// equal the stored amount.
	Amount *decimal.Decimal `json:"amount" binding:"required,gte=0" swaggertype:"number" example:"40"`
	// Payment method of the simulated payment provider. Optional: without it the outcome alone
	// decides and no payment method is recorded.
	PaymentMethod *model.PaymentMethod `json:"paymentMethod" binding:"omitempty,oneof=credit_card direct_debit paypal" enums:"credit_card,direct_debit,paypal" example:"credit_card"`
	// Card data; required to pay with paymentMethod credit_card.
	CreditCard *model.CreditCardDetails `json:"creditCard"`
	// Account data and SEPA mandate; required to pay with paymentMethod direct_debit.
	DirectDebit *model.DirectDebitDetails `json:"directDebit"`
	// PayPal account of PayPal Express; optional with paymentMethod paypal.
	PayPal *model.PayPalDetails `json:"paypal"`
}

// paymentMethodInput returns the payment method of the request, or nil when the request names no
// payment method and sends no payment details.
func (req *SimulatePaymentRequest) paymentMethodInput() *model.PaymentMethodInput {
	if req.PaymentMethod == nil && req.CreditCard == nil && req.DirectDebit == nil && req.PayPal == nil {
		return nil
	}
	in := &model.PaymentMethodInput{CreditCard: req.CreditCard, DirectDebit: req.DirectDebit, PayPal: req.PayPal}
	if req.PaymentMethod != nil {
		in.Method = *req.PaymentMethod
	}
	return in
}

type PaymentHandler struct {
	paymentService *service.PaymentService
	userRepo       repository.UserRepository
}

func NewPaymentHandler(paymentService *service.PaymentService, userRepo repository.UserRepository) *PaymentHandler {
	return &PaymentHandler{paymentService: paymentService, userRepo: userRepo}
}

// GetPaymentHandler godoc
// @Summary      Zahlungsvorgang abrufen
// @Description  Liefert den gespeicherten Zahlungsvorgang des Aufrufers: den Summenbetrag, den Zahlungsstatus und jedes Item mit Event, Ticketanzahl, Zeilenbetrag, Buchungsstatus und Ticketgültigkeit. Der Zahlungsvorgang deckt alle Buchungen eines Warenkorb-Checkouts ab und schließt sie zusammen ab. ticketsValid ist nur true, wenn die Zahlung bezahlt, alle Buchungen bestätigt sind und die Events stattfinden oder stattgefunden haben. Zahlung, Buchungen, Event-Status und Datenbankuhr stammen aus einer Abfrage. Offene Zahlungen abgelaufener Reservierungen schließt ein Hintergrundjob jede Minute als fehlgeschlagen ab. Zahlungsvorgänge anderer Nutzer liefern 404.
// @Tags         payments
// @Produce      json
// @Param        paymentId path string true "Payment ID"
// @Success      200 {object} model.PaymentResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Zahlungsvorgang nicht gefunden"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /payments/{paymentId} [get]
func (h *PaymentHandler) GetPaymentHandler(c *gin.Context) {
	paymentID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	payment, err := h.paymentService.GetPayment(c.Request.Context(), paymentID, user.UserID)
	if err != nil {
		writePaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, payment)
}

// SimulatePaymentHandler godoc
// @Summary      Zahlung simulieren
// @Description  Schließt den offenen Zahlungsvorgang des Aufrufers mit einem simulierten Ergebnis ab; es ist kein echter Zahlungsdienstleister angebunden. amount muss dem gespeicherten Summenbetrag entsprechen, sonst wird nichts geändert (422). Bei success werden alle Buchungen des Zahlungsvorgangs verbindlich bestätigt (bookingStatus confirmed, ticketsValid true). Bei failure, abgelaufener Reservierung oder nicht mehr verfügbarem Event schlägt die Zahlung fehl (status failed, failureReason, message), keine Buchung erhält einen Ticketanspruch und alle Tickets werden freigegeben — der Zahlungsvorgang gilt für alle Buchungen zusammen oder für keine. Das Ergebnis wird innerhalb der Transaktion aus dem gespeicherten Zahlungsstatus gelesen. Der Body darf höchstens 4 KiB groß sein (413). Zahlungsmethoden: paymentMethod credit_card (creditCard mit cardholder, number, expiry MM/YY, cvc; Visa, Mastercard, American Express; Luhn-Prüfung), direct_debit (directDebit mit accountHolder, iban aus dem SEPA-Raum mit gültiger Prüfziffer, mandateAccepted true) oder paypal (PayPal Express, paypal.email optional). Bei success sind creditCard bzw. directDebit Pflicht, bei failure optional; nur die Daten der gewählten Methode dürfen gesendet werden. Ungültige Zahlungsdaten liefern 400, ohne dass sich etwas ändert. Der simulierte Zahlungsanbieter lehnt seine Testdaten ab (status failed, failureReason declined): Karte 4000 0000 0000 0002, IBAN DE62 3704 0044 0532 0130 01, PayPal paypal-declined@example.com; alle anderen gültigen Daten werden belastet. Gespeichert werden nur paymentMethod, das maskierte Konto (paymentAccount), cardBrand und die Referenz des Anbieters (transactionId, bei Lastschrift die Mandatsreferenz) — nie Kartennummer, CVC oder IBAN im Klartext. Ohne paymentMethod entscheidet allein outcome und es wird keine Zahlungsmethode gespeichert.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Param        paymentId path string true "Payment ID"
// @Param        payment body SimulatePaymentRequest true "Simuliertes Ergebnis, angezeigter Betrag und Zahlungsmethode"
// @Success      200 {object} model.PaymentResponse "Ergebnis des Zahlungsvorgangs, erfolgreich oder fehlgeschlagen"
// @Failure      400 {object} model.ErrorResponse "Ungültiger Request oder ungültige Zahlungsdaten"
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Zahlungsvorgang nicht gefunden"
// @Failure      409 {object} model.ErrorResponse "Zahlung bereits abgeschlossen oder Buchung nicht reserviert"
// @Failure      413 {object} model.ErrorResponse "Request Body größer als 4 KiB"
// @Failure      422 {object} model.ErrorResponse "Betrag weicht vom gespeicherten Betrag ab"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /payments/{paymentId}/simulate [post]
func (h *PaymentHandler) SimulatePaymentHandler(c *gin.Context) {
	paymentID, user, ok := h.resolveRequest(c)
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

	payment, err := h.paymentService.SimulatePaymentWithMethod(c.Request.Context(), paymentID, user.UserID, req.Outcome, *req.Amount, req.paymentMethodInput())
	if err != nil {
		writePaymentError(c, err)
		return
	}
	c.JSON(http.StatusOK, payment)
}

// resolveRequest parses the payment ID and resolves the API user of the caller. It writes the
// problem response itself and reports false when the request cannot continue.
func (h *PaymentHandler) resolveRequest(c *gin.Context) (uuid.UUID, *model.UserModel, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return uuid.Nil, nil, false
	}

	paymentID, err := uuid.Parse(c.Param("paymentId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid payment id")
		return uuid.Nil, nil, false
	}

	user, ok := resolveAPIUser(c, h.userRepo, principal)
	return paymentID, user, ok
}

func writePaymentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidPaymentOutcome),
		errors.Is(err, service.ErrInvalidTicketCount),
		errors.Is(err, service.ErrInvalidPaymentMethod),
		errors.Is(err, service.ErrPaymentMethodRequired),
		errors.Is(err, service.ErrInvalidPaymentDetails):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrPaymentNotFound),
		errors.Is(err, service.ErrBookingNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrBookingNotPayable),
		errors.Is(err, service.ErrReservationExpired),
		errors.Is(err, service.ErrEventUnavailable),
		errors.Is(err, service.ErrPaymentAlreadyProcessed):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrPaymentAmountMismatch),
		errors.Is(err, service.ErrInvalidPaymentAmount):
		writeProblem(c, http.StatusUnprocessableEntity, err.Error())
	default:
		slog.Error("unexpected payment error", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
