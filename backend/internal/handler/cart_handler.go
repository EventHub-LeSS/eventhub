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

// 282: Warenkorb
type AddCartItemRequest struct {
	EventID  *uuid.UUID `json:"eventId" binding:"required" example:"550e8400-e29b-41d4-a716-446655440000"`
	Quantity int        `json:"quantity" binding:"required,gte=1" example:"2"`
}

type UpdateCartItemRequest struct {
	Quantity int `json:"quantity" binding:"required,gte=1" example:"3"`
}

type CartHandler struct {
	cartService *service.CartService
	userRepo    repository.UserRepository
}

func NewCartHandler(cartService *service.CartService, userRepo repository.UserRepository) *CartHandler {
	return &CartHandler{cartService: cartService, userRepo: userRepo}
}

// GetCartHandler godoc
// @Summary      Warenkorb abrufen
// @Description  Liefert den Warenkorb des Aufrufers: jede lebende Reservierung ohne Zahlung mit der aktuellen Verfügbarkeit und den aktuellen Preisen ihres Events. Reservierungen halten ihre Plätze bis expiresAt; danach sind sie nicht mehr Teil des Warenkorb. totalAmount ist die Summe der aktuellen Zeilenbeträge — der Checkout berechnet und speichert den zu zahlenden Betrag selbst. checkoutReady ist nur true, wenn der Warenkorb Items hat und alle Events noch veröffentlicht sind.
// @Tags         cart
// @Produce      json
// @Success      200 {object} model.CartResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart [get]
func (h *CartHandler) GetCartHandler(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}
	cart, err := h.cartService.GetCart(c.Request.Context(), user.UserID)
	if err != nil {
		writeCartError(c, err)
		return
	}
	c.JSON(http.StatusOK, cart)
}

// AddCartItemHandler godoc
// @Summary      Tickets in den Warenkorb legen
// @Description  Reserviert Tickets eines veröffentlichten Events für den AufrufER und legt sie damit in den Warenkorb. Die Reservierung hält die Plätze bis expiresAt (15 Minuten) und verfällt danach. Existiert bereits ein Item für das Event, wächst dessen Anzahl (höchstens 10 pro Event) und die Reservierung wird erneuert; ein abgelaufenes Item wird ersetzt. Der Platz wird wie bei einer Buchung sofort gegen die Kapazität geprüft (409). Die Antwort ist der Warenkorb nach der Änderung.
// @Tags         cart
// @Accept       json
// @Produce      json
// @Param        item body AddCartItemRequest true "Event und Ticketanzahl"
// @Success      201 {object} model.CartResponse "Warenkorb nach dem Hinzufügen"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Event unbekannt"
// @Failure      409 {object} model.ErrorResponse "Event nicht veröffentlicht oder Kapazität überschritten"
// @Failure      422 {object} model.ErrorResponse "Mehr als 10 Tickets für ein Event"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart/items [post]
func (h *CartHandler) AddCartItemHandler(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	var req AddCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	cart, err := h.cartService.AddToCart(c.Request.Context(), user.UserID, *req.EventID, req.Quantity)
	if err != nil {
		writeCartError(c, err)
		return
	}
	c.JSON(http.StatusCreated, cart)
}

// UpdateCartItemHandler godoc
// @Summary      Ticketanzahl eines Warenkorb-Items ändern
// @Description  Setzt die Ticketanzahl eines eigenen Warenkorb-Items (Reservierung ohne Zahlung). Das Item behält seine reservierten Plätze, mehr Tickets brauchen freie Plätze (409); die Reservierung wird erneuert. Abgelaufene, ausgecheckte, stornierte oder fremde Items liefern 404. Die Antwort ist der Warenkorb nach der Änderung.
// @Tags         cart
// @Accept       json
// @Produce      json
// @Param        bookingId path string true "ID der Reservierung hinter dem Item"
// @Param        item body UpdateCartItemRequest true "Neue Ticketanzahl"
// @Success      200 {object} model.CartResponse "Warenkorb nach der Änderung"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Item unbekannt oder kein Warenkorb-Item"
// @Failure      409 {object} model.ErrorResponse "Reservierung abgelaufen oder Kapazität überschritten"
// @Failure      422 {object} model.ErrorResponse "Anzahl außerhalb 1 bis 10"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart/items/{bookingId} [patch]
func (h *CartHandler) UpdateCartItemHandler(c *gin.Context) {
	bookingID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	var req UpdateCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	cart, err := h.cartService.UpdateQuantity(c.Request.Context(), user.UserID, bookingID, req.Quantity)
	if err != nil {
		writeCartError(c, err)
		return
	}
	c.JSON(http.StatusOK, cart)
}

// RemoveCartItemHandler godoc
// @Summary      Item aus dem Warenkorb entfernen
// @Description  Entfernt ein eigenes Warenkorb-Item (Reservierung ohne Zahlung) und gibt dessen Plätze sofort frei. Abgelaufene, ausgecheckte, stornierte oder fremde Items liefern 404. Die Antwort ist der Warenkorb nach der Änderung.
// @Tags         cart
// @Produce      json
// @Param        bookingId path string true "ID der Reservierung hinter dem Item"
// @Success      200 {object} model.CartResponse "Warenkorb nach dem Entfernen"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      404 {object} model.ErrorResponse "Item unbekannt oder kein Warenkorb-Item"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart/items/{bookingId} [delete]
func (h *CartHandler) RemoveCartItemHandler(c *gin.Context) {
	bookingID, user, ok := h.resolveRequest(c)
	if !ok {
		return
	}

	cart, err := h.cartService.RemoveFromCart(c.Request.Context(), user.UserID, bookingID)
	if err != nil {
		writeCartError(c, err)
		return
	}
	c.JSON(http.StatusOK, cart)
}

// ClearCartHandler godoc
// @Summary      Warenkorb leeren
// @Description  Entfernt alle Warenkorb-Items des Aufrufers und gibt deren Plätze sofort frei. Reservierungen mit offener Zahlung gehören nicht zum Warenkorb und bleiben unberührt.
// @Tags         cart
// @Produce      json
// @Success      200 {object} model.CartResponse "Leerer Warenkorb"
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart [delete]
func (h *CartHandler) ClearCartHandler(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	cart, err := h.cartService.ClearCart(c.Request.Context(), user.UserID)
	if err != nil {
		writeCartError(c, err)
		return
	}
	c.JSON(http.StatusOK, cart)
}

// CheckoutHandler godoc
// @Summary      Warenkorb auschecken
// @Description  Überführt alle lebenden Reservierungen des Aufrufers in einen Zahlungsvorgang: ein Zahlung mit dem Summenbetrag und pro Item ein Betrag aus Ticketanzahl mal aktuellem Event-Preis, alles serverseitig berechnet und gespeichert; Beträge vom Client werden ignoriert. Alle Buchungen teilen sich eine erneuerte Reservierungsfrist (15 Minuten) — die Zahlung muss vor expiresAt abgeschlossen werden. Items mit abgelaufener Reservierung halten keine Plätze und werden ausgelassen; ist kein Item mehr lebend, liefert der Aufruf 409 bzw. 422 bei leerem Warenkorb. Ein Event, das nicht mehr veröffentlicht ist, lässt den gesamten Checkout scheitern (409), weil die Zahlung alle Items zusammen oder gar nicht abschließt. Existiert bereits ein offener Zahlungsvorgang, wird dieser unverändert zurückgegeben (200).
// @Tags         cart
// @Produce      json
// @Success      200 {object} model.PaymentResponse "Offener Zahlungsvorgang existierte bereits"
// @Success      201 {object} model.PaymentResponse "Zahlungsvorgang angelegt"
// @Failure      401 {object} model.ErrorResponse
// @Failure      403 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Reservierungen abgelaufen oder Event nicht mehr verfügbar"
// @Failure      422 {object} model.ErrorResponse "Warenkorb ist leer"
// @Failure      500 {object} model.ErrorResponse
// @Security     BearerAuth
// @Router       /cart/checkout [post]
func (h *CartHandler) CheckoutHandler(c *gin.Context) {
	user, ok := h.resolveUser(c)
	if !ok {
		return
	}

	payment, created, err := h.cartService.Checkout(c.Request.Context(), user.UserID)
	if err != nil {
		writeCartError(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, payment)
}

// resolveUser resolves the API user of the caller. It writes the problem response itself and
// reports false when the request cannot continue.
func (h *CartHandler) resolveUser(c *gin.Context) (*model.UserModel, bool) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return nil, false
	}
	return resolveAPIUser(c, h.userRepo, principal)
}

// resolveRequest parses the booking ID of the cart item and resolves the API user of the caller.
// It writes the problem response itself and reports false when the request cannot continue.
func (h *CartHandler) resolveRequest(c *gin.Context) (uuid.UUID, *model.UserModel, bool) {
	bookingID, err := uuid.Parse(c.Param("bookingId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid booking id")
		return uuid.Nil, nil, false
	}
	user, ok := h.resolveUser(c)
	return bookingID, user, ok
}

func writeCartError(c *gin.Context, err error) {
	var capacityErr *service.CapacityExceededError
	switch {
	case errors.Is(err, service.ErrInvalidTicketCount):
		writeProblem(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrCartItemNotFound), errors.Is(err, service.ErrEventNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrEventNotPublished),
		errors.Is(err, service.ErrReservationExpired),
		errors.Is(err, service.ErrEventUnavailable):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.As(err, &capacityErr):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrTicketLimitExceeded), errors.Is(err, service.ErrCartEmpty):
		writeProblem(c, http.StatusUnprocessableEntity, err.Error())
	default:
		slog.Error("unexpected cart error", "error", err)
		writeProblem(c, http.StatusInternalServerError, "internal error")
	}
}
