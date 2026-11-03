package repository

import (
	"backend/internal/model"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 282: Warenkorb
// CartItem is one live reservation of the user without a payment, with the current data of its
// event and the seats the event has occupied. The cart is exactly these items; expired
// reservations hold no seats and are not part of it.
type CartItem struct {
	Booking *model.BookingModel
	// Event is nil when the event of the booking no longer exists.
	Event *model.EventModel
	// Location is nil when the event has no location or the event no longer exists.
	Location *model.LocationModel
	// Occupied is what CountOccupiedTickets would say for the event: the tickets of confirmed
	// bookings and live reservations, including this item's own tickets.
	Occupied int64
	// Now is the database clock at the time of the read.
	Now time.Time
}

// CartRepository reads the cart of a user. Writes go through the booking repository inside
// transactions, like everywhere else.
type CartRepository interface {
	// ListCart returns the live cart items of the user, the oldest first, from one statement
	// and thus one snapshot: one clock for every expiry and one occupancy count per event.
	ListCart(userID uuid.UUID) ([]*CartItem, error)
}

type cartRepository struct {
	db *gorm.DB
}

func NewCartRepository(db *gorm.DB) CartRepository {
	return &cartRepository{db: db}
}

// cartQuery selects the live cart items of a user with event, location and per-event occupancy.
// The clock CTE reads clock_timestamp() once, so every item is judged by the same database time
// and the occupancy counts use the same instant, like CountOccupiedTickets.
const cartQuery = `WITH clock AS MATERIALIZED (SELECT clock_timestamp() AS database_now),
    occupied AS (
        SELECT event_id, SUM(number_of_tickets) AS occupied
        FROM bookings
        WHERE status = ? OR (status = ? AND expires_at > (SELECT database_now FROM clock))
        GROUP BY event_id
    )
	SELECT b.*,
	    e.event_id AS item_event_id, e.title AS event_title, e.description AS event_description,
	    e.start_time AS event_start_time, e.end_time AS event_end_time, e.capacity AS event_capacity,
	    e.status AS event_status, e.price AS event_price, e.category_id AS event_category_id,
	    e.organizer_id AS event_organizer_id, e.location_id AS event_location_id,
	    e.created_at AS event_created_at, e.updated_at AS event_updated_at,
	    l.location_id AS location_id, l.name AS location_name, l.city AS location_city,
	    l.postal_code AS location_postal_code, l.street AS location_street, l.house_number AS location_house_number,
	    COALESCE(o.occupied, 0) AS occupied, clock.database_now
	FROM bookings b
	CROSS JOIN clock
	LEFT JOIN events e ON e.event_id = b.event_id
	LEFT JOIN locations l ON l.location_id = e.location_id
	LEFT JOIN occupied o ON o.event_id = b.event_id
	WHERE b.user_id = ? AND b.status = ? AND b.payment_id IS NULL
	    AND b.expires_at > (SELECT database_now FROM clock)
	ORDER BY b.created_at, b.booking_id`

// cartRow is one row of cartQuery; the event columns are NULL when the event no longer exists.
type cartRow struct {
	model.BookingModel
	ItemEventID          *uuid.UUID          `gorm:"column:item_event_id"`
	EventTitle           *string             `gorm:"column:event_title"`
	EventDescription     *string             `gorm:"column:event_description"`
	EventStartTime       *time.Time          `gorm:"column:event_start_time"`
	EventEndTime         *time.Time          `gorm:"column:event_end_time"`
	EventCapacity        *int                `gorm:"column:event_capacity"`
	EventStatus          *model.EventStatus  `gorm:"column:event_status"`
	EventPrice           decimal.NullDecimal `gorm:"column:event_price"`
	EventCategoryID      *uuid.UUID          `gorm:"column:event_category_id"`
	EventOrganizerID     *uuid.UUID          `gorm:"column:event_organizer_id"`
	EventLocationID      *uuid.UUID          `gorm:"column:event_location_id"`
	EventCreatedAt       *time.Time          `gorm:"column:event_created_at"`
	EventUpdatedAt       *time.Time          `gorm:"column:event_updated_at"`
	LocationID           *uuid.UUID          `gorm:"column:location_id"`
	LocationName         *string             `gorm:"column:location_name"`
	LocationCity         *string             `gorm:"column:location_city"`
	LocationPostalCode   *string             `gorm:"column:location_postal_code"`
	LocationStreet       *string             `gorm:"column:location_street"`
	LocationHouseNumber  *string             `gorm:"column:location_house_number"`
	Occupied             int64               `gorm:"column:occupied"`
	DatabaseNow          time.Time           `gorm:"column:database_now"`
}

func (r *cartRepository) ListCart(userID uuid.UUID) ([]*CartItem, error) {
	var rows []cartRow
	err := r.db.Raw(cartQuery,
		model.BookingStatusConfirmed, model.BookingStatusReserved, userID, model.BookingStatusReserved).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]*CartItem, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].cartItem())
	}
	return items, nil
}

func (row *cartRow) cartItem() *CartItem {
	booking := row.BookingModel
	item := &CartItem{Booking: &booking, Occupied: row.Occupied, Now: row.DatabaseNow}
	if row.ItemEventID != nil && row.EventStatus != nil {
		item.Event = &model.EventModel{
			EventID:      *row.ItemEventID,
			Title:        derefString(row.EventTitle),
			Description:  row.EventDescription,
			StartTime:    derefTime(row.EventStartTime),
			EndTime:      derefTime(row.EventEndTime),
			Capacity:     derefInt(row.EventCapacity),
			Status:       *row.EventStatus,
			Price:        row.EventPrice.Decimal,
			CategoryID:   row.EventCategoryID,
			OrganizerID:  row.EventOrganizerID,
			LocationID:   row.EventLocationID,
			CreatedAt:    derefTime(row.EventCreatedAt),
			UpdatedAt:    derefTime(row.EventUpdatedAt),
		}
	}
	if row.LocationID != nil {
		item.Location = &model.LocationModel{
			LocationID:   *row.LocationID,
			Name:         derefString(row.LocationName),
			City:         derefString(row.LocationCity),
			PostalCode:   derefString(row.LocationPostalCode),
			Street:       derefString(row.LocationStreet),
			HouseNumber:  row.LocationHouseNumber,
		}
	}
	return item
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
