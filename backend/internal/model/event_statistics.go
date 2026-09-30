package model

import "github.com/google/uuid"

type EventStatistics struct {
	EventID        uuid.UUID `json:"eventId"`
	Capacity       int       `json:"capacity"`
	SoldTickets    int64     `json:"soldTickets"`
	AvailableSeats int64     `json:"availableSeats"`
}

type SoldTicketsResponse struct {
	EventID     uuid.UUID `json:"eventId"`
	SoldTickets int64     `json:"soldTickets"`
}

type AvailableSeatsResponse struct {
	EventID        uuid.UUID `json:"eventId"`
	AvailableSeats int64     `json:"availableSeats"`
}
