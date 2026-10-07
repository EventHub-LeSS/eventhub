package model

type EventAvailability string

const (
	EventAvailabilityAvailable              EventAvailability = "available"
	EventAvailabilityAlmostSoldOut          EventAvailability = "almost_sold_out"
	EventAvailabilitySoldOut                EventAvailability = "sold_out"
	EventAvailabilityTemporarilyUnavailable EventAvailability = "temporarily_unavailable"
)

// PublishedEventDetailsResponse contains public event details and current availability.
// Bookable is a snapshot; booking checks availability again in its transaction.
type PublishedEventDetailsResponse struct {
	PublishedEventResponse `gorm:"embedded"`
	Description            *string           `json:"description"`
	Capacity               int               `json:"capacity"`
	AvailableSeats         int64             `json:"availableSeats"`
	Bookable               bool              `json:"bookable" gorm:"-"`
	SoldTickets            int64             `json:"soldTickets"`
	OccupancyPercent       float64           `json:"occupancyPercent" gorm:"-"`
	Availability           EventAvailability `json:"availability" gorm:"-" enums:"available,almost_sold_out,sold_out,temporarily_unavailable"`
}
