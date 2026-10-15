package model

import "github.com/google/uuid"

// SalesDashboardEventResponse contains sales figures for an own event.
type SalesDashboardEventResponse struct {
	EventID        uuid.UUID `json:"eventId"`
	Title          string    `json:"title"`
	Capacity       int       `json:"capacity"`
	SoldTickets    int64     `json:"soldTickets"`
	AvailableSeats int64     `json:"availableSeats"`
}
