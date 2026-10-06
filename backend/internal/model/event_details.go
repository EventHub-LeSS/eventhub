package model

// PublishedEventDetailsResponse contains public event details and current availability.
// Bookable is a snapshot; booking checks availability again in its transaction.
type PublishedEventDetailsResponse struct {
	PublishedEventResponse `gorm:"embedded"`
	Description            *string `json:"description"`
	Capacity               int     `json:"capacity"`
	AvailableSeats         int64   `json:"availableSeats"`
	Bookable               bool    `json:"bookable" gorm:"-"`
}
