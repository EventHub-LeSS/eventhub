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
}

type eventRepository struct {
	db *gorm.DB
}

func NewEventRepository(db *gorm.DB) EventRepository {
	return &eventRepository{db: db}
}
func (r *eventRepository) CreateEvent(event *model.EventModel) error {
	return r.db.Create(event).Error
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
	return r.db.Save(event).Error
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
