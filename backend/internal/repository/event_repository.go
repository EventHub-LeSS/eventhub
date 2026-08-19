package repository

import (
	"backend/internal/model"

	"github.com/go-pg/pg/v10"
	"github.com/google/uuid"
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
	db *pg.DB
}

func NewEventRepository(db *pg.DB) EventRepository {
	return &eventRepository{db: db}
}
func (r *eventRepository) CreateEvent(event *model.EventModel) error {
	_, err := r.db.Model(event).Insert()
	return err
}

func (r *eventRepository) GetEventByID(eventID uuid.UUID) (*model.EventModel, error) {
	event := &model.EventModel{}
	err := r.db.Model(event).Where("event_id = ?", eventID).Select()
	if err != nil {
		return nil, err
	}
	return event, nil
}

func (r *eventRepository) GetAllEvents() ([]*model.EventModel, error) {
	var events []*model.EventModel
	err := r.db.Model(&events).Select()
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (r *eventRepository) UpdateEvent(event *model.EventModel) error {
	_, err := r.db.Model(event).WherePK().Update()
	return err
}
func (r *eventRepository) DeleteEvent(eventID uuid.UUID) error {
	event := &model.EventModel{EventID: eventID}
	_, err := r.db.Model(event).WherePK().Delete()
	return err
}

func (r *eventRepository) ListByOrganization(organizationID uuid.UUID) ([]*model.EventModel, error) {
	var events []*model.EventModel
	err := r.db.Model(&events).Where("organizer_id = ?", organizationID).Select()
	if err != nil {
		return nil, err
	}
	return events, nil
}
