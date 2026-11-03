package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// 282: Warenkorb
// CartItemResponse is one item of the cart: the reserved tickets of one event. The reservation
// holds its seats until expiresAt; afterwards the item is gone and the seats are free again.
type CartItemResponse struct {
	// ID of the reservation behind the item; used to change or remove it.
	BookingID uuid.UUID `json:"bookingId"`
	// Reserved tickets of the item; set with PATCH, at most 10 per event.
	NumberOfTickets int `json:"numberOfTickets" example:"2"`
	// Null when the event no longer exists.
	Event *OwnBookingEvent `json:"event"`
	// Price of one ticket, from the current event data; the checkout stores the charged line.
	UnitPrice decimal.Decimal `json:"unitPrice" swaggertype:"string" example:"20"`
	// Unit price times the number of tickets, from the current event data.
	LineTotal decimal.Decimal `json:"lineTotal" swaggertype:"string" example:"40"`
	// Seats free for others plus the seats this item holds: the most tickets the item can have.
	// The reservation itself always keeps its tickets until it expires.
	AvailableSeats int64 `json:"availableSeats"`
	// False when the event is no longer published; such an item blocks the checkout.
	Bookable bool `json:"bookable"`
	// Deadline of the reservation; the tickets are released when it passes.
	ExpiresAt time.Time `json:"expiresAt"`
	// Time the item entered the cart.
	CreatedAt time.Time `json:"createdAt"`
}

// CartResponse is the cart of the caller: every live reservation without a payment.
type CartResponse struct {
	Items []CartItemResponse `json:"items"`
	// Total number of reserved tickets over all items.
	TotalTickets int `json:"totalTickets"`
	// Sum of the line totals of the items, from the current event data. The checkout computes
	// and stores the charged amount itself.
	TotalAmount decimal.Decimal `json:"totalAmount" swaggertype:"string" example:"40"`
	// True when the cart has items and all of them belong to published events.
	CheckoutReady bool `json:"checkoutReady"`
}
