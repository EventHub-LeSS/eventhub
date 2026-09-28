package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EventRepository interface {
	CreateEvent(event *model.EventModel) error
	GetEventByID(eventID uuid.UUID) (*model.EventModel, error)
	GetAllEvents() ([]*model.EventModel, error)
	UpdateEvent(event *model.EventModel) error
	DeleteEvent(eventID uuid.UUID) error
	ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error)
	// ListByOrganizers returns the events of the given organizations; an empty status matches every status.
	ListByOrganizers(organizationIDs []uuid.UUID, status model.EventStatus) ([]*model.EventModel, error)
	GetConfirmedTicketCount(eventID uuid.UUID) (int64, error)
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
	event := &model.EventModel{}
	err := r.db.First(event, "event_id = ?", eventID).Error
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

func (r *eventRepository) DeleteEvent(eventID uuid.UUID) error {
	return r.db.Delete(&model.EventModel{}, "event_id = ?", eventID).Error
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
