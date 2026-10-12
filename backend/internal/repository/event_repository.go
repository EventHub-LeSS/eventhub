package repository

import (
	"backend/internal/model"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EventRepository interface {
	GetPublishedEventDetails(eventID uuid.UUID) (*model.PublishedEventDetailsResponse, error)
	CreateEvent(event *model.EventModel) error
	GetEventByID(eventID uuid.UUID) (*model.EventModel, error)
	LockEvent(eventID uuid.UUID) (*model.EventModel, error)
	GetAllEvents() ([]*model.EventModel, error)
	ListPublishedEvents(filter model.PublishedEventFilter) ([]model.PublishedEventResponse, error)
	UpdateEvent(event *model.EventModel) error
	// DeleteEvent removes the event row and reports whether there was one to remove.
	DeleteEvent(eventID uuid.UUID) (bool, error)
	ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error)
	// ListByOrganizers returns the events of the given organizations; an empty status matches every status.
	ListByOrganizers(organizationIDs []uuid.UUID, status model.EventStatus) ([]*model.EventModel, error)
	GetConfirmedTicketCount(eventID uuid.UUID) (int64, error)
	// HasBookings reports whether the event has bookings in any status, including cancelled,
	// failed and expired ones.
	HasBookings(eventID uuid.UUID) (bool, error)
}

type eventRepository struct {
	db *gorm.DB
}

func NewEventRepository(db *gorm.DB) EventRepository {
	return &eventRepository{db: db}
}

func (r *eventRepository) CreateEvent(event *model.EventModel) error {
	return translateEventWriteError(r.db.Create(event).Error)
}

func (r *eventRepository) GetEventByID(eventID uuid.UUID) (*model.EventModel, error) {
	return r.getEvent(eventID, false)
}

func (r *eventRepository) LockEvent(eventID uuid.UUID) (*model.EventModel, error) {
	return r.getEvent(eventID, true)
}

func (r *eventRepository) getEvent(eventID uuid.UUID, lock bool) (*model.EventModel, error) {
	event := &model.EventModel{}
	q := r.db
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(event, "event_id = ?", eventID).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (r *eventRepository) GetAllEvents() ([]*model.EventModel, error) {
	var events []*model.EventModel
	err := r.db.Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (r *eventRepository) UpdateEvent(event *model.EventModel) error {
	return translateEventWriteError(r.db.Save(event).Error)
}

// DeleteEvent is a hard delete: EventModel has no gorm.DeletedAt, so GORM issues a real DELETE.
func (r *eventRepository) DeleteEvent(eventID uuid.UUID) (bool, error) {
	result := r.db.Delete(&model.EventModel{}, "event_id = ?", eventID)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *eventRepository) ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error) {
	var events []*model.EventModel
	err := r.db.Where("organizer_id = ?", organizationID).Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (r *eventRepository) ListByOrganizers(organizationIDs []uuid.UUID, status model.EventStatus) ([]*model.EventModel, error) {
	if len(organizationIDs) == 0 {
		return nil, nil
	}
	query := r.db.Where("organizer_id IN ?", organizationIDs)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var events []*model.EventModel
	if err := query.Order("start_time ASC").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (r *eventRepository) GetConfirmedTicketCount(eventID uuid.UUID) (int64, error) {
	var soldTickets int64

	err := r.db.
		Table("bookings").
		Where("event_id = ? AND status = ?", eventID, model.BookingStatusConfirmed).
		Select("COALESCE(SUM(number_of_tickets), 0)").
		Scan(&soldTickets).Error

	if err != nil {
		return 0, err
	}

	return soldTickets, nil
}

func (r *eventRepository) HasBookings(eventID uuid.UUID) (bool, error) {
	var bookings int64
	if err := r.db.Table("bookings").Where("event_id = ?", eventID).Count(&bookings).Error; err != nil {
		return false, err
	}
	return bookings > 0, nil
}

// ListPublishedEvents resolves display data in the same query as the status filter.
// Inner joins omit incomplete events whose category or location was deleted.
func (r *eventRepository) ListPublishedEvents(filter model.PublishedEventFilter) ([]model.PublishedEventResponse, error) {
	events := make([]model.PublishedEventResponse, 0)
	query := r.db.Table("events").
		Select("events.event_id, events.title, events.start_time, events.end_time, events.status, events.price, categories.category_id AS category_id, categories.category AS category_name, locations.location_id AS location_id, locations.name AS location_name, locations.city AS location_city, locations.postal_code AS location_postal_code, locations.street AS location_street, locations.house_number AS location_house_number").
		Joins("JOIN categories ON categories.category_id = events.category_id").
		Joins("JOIN locations ON locations.location_id = events.location_id").
		Where("events.status = ?", model.EventStatusPublished)
	if filter.Date != nil {
		// Calendar days can be 23 or 25 hours across DST transitions.
		query = query.Where("events.start_time >= ? AND events.start_time < ?", *filter.Date, filter.Date.AddDate(0, 0, 1))
	}
	if filter.CategoryID != nil {
		query = query.Where("events.category_id = ?", *filter.CategoryID)
	}
	if filter.Location != "" {
		// Use ! as an explicit escape character so %, _ and ! remain literal text.
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(filter.Location) + "%"
		query = query.Where("(locations.city ILIKE ? ESCAPE '!' OR locations.name ILIKE ? ESCAPE '!')", pattern, pattern)
	}
	err := query.Order("events.start_time ASC, events.event_id ASC").Scan(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

// GetPublishedEventDetails reads event data and occupied capacity in one database snapshot.
func (r *eventRepository) GetPublishedEventDetails(eventID uuid.UUID) (*model.PublishedEventDetailsResponse, error) {
	var details model.PublishedEventDetailsResponse
	occupied := r.db.Table("bookings").
		Select("COALESCE(SUM(number_of_tickets), 0)").
		Where("event_id = events.event_id AND (status = ? OR (status = ? AND expires_at > NOW()))",
			model.BookingStatusConfirmed, model.BookingStatusReserved)
	confirmed := r.db.Table("bookings").
		Select("COALESCE(SUM(number_of_tickets), 0)").
		Where("event_id = events.event_id AND status = ?", model.BookingStatusConfirmed)
	result := r.db.Table("events").
		Select("events.event_id, events.title, events.description, events.start_time, events.end_time, events.status, events.price, events.capacity, (?) AS sold_tickets, GREATEST(events.capacity - (?), 0) AS available_seats, categories.category_id AS category_id, categories.category AS category_name, locations.location_id AS location_id, locations.name AS location_name, locations.city AS location_city, locations.postal_code AS location_postal_code, locations.street AS location_street, locations.house_number AS location_house_number", confirmed, occupied).
		Joins("JOIN categories ON categories.category_id = events.category_id").
		Joins("JOIN locations ON locations.location_id = events.location_id").
		Where("events.event_id = ? AND events.status = ?", eventID, model.EventStatusPublished).
		Scan(&details)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &details, nil
}
