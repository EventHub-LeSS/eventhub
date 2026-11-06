package repository

import (
	"backend/internal/model"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrPendingApplicationExists means the user already has an application awaiting review.
var ErrPendingApplicationExists = errors.New("an organizer application of this user is already pending")

const (
	sqlStateUniqueViolation          = "23505"
	pendingApplicationUniqueIndexKey = "idx_organizer_applications_one_pending"
)

type OrganizerApplicationRepository interface {
	// Create stores a new application. It returns ErrPendingApplicationExists when the user
	// already has a pending one.
	Create(application *model.OrganizerApplicationModel) error
	// GetByID returns the application, or nil if it does not exist.
	GetByID(applicationID uuid.UUID) (*model.OrganizerApplicationModel, error)
	// Lock returns the application and locks its row until the transaction ends, or nil if it
	// does not exist.
	Lock(applicationID uuid.UUID) (*model.OrganizerApplicationModel, error)
	// Update persists every column of the application.
	Update(application *model.OrganizerApplicationModel) error
	// ListForUser returns all applications of the user, newest first.
	ListForUser(userID uuid.UUID) ([]model.OrganizerApplicationModel, error)
	// List returns one page of applications, oldest first so reviewers work through the queue,
	// and the number of all matching applications.
	List(filter model.OrganizerApplicationFilter, page, limit int) ([]model.OrganizerApplicationModel, int64, error)
}

type organizerApplicationRepository struct {
	db *gorm.DB
}

func NewOrganizerApplicationRepository(db *gorm.DB) OrganizerApplicationRepository {
	return &organizerApplicationRepository{db: db}
}

func (r *organizerApplicationRepository) Create(application *model.OrganizerApplicationModel) error {
	err := r.db.Create(application).Error
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateUniqueViolation && pgErr.ConstraintName == pendingApplicationUniqueIndexKey {
		return ErrPendingApplicationExists
	}
	return err
}

func (r *organizerApplicationRepository) GetByID(applicationID uuid.UUID) (*model.OrganizerApplicationModel, error) {
	return r.first(r.db, applicationID)
}

func (r *organizerApplicationRepository) Lock(applicationID uuid.UUID) (*model.OrganizerApplicationModel, error) {
	return r.first(r.db.Clauses(clause.Locking{Strength: "UPDATE"}), applicationID)
}

func (r *organizerApplicationRepository) first(db *gorm.DB, applicationID uuid.UUID) (*model.OrganizerApplicationModel, error) {
	application := &model.OrganizerApplicationModel{}
	err := db.Where("application_id = ?", applicationID).First(application).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return application, nil
}

func (r *organizerApplicationRepository) Update(application *model.OrganizerApplicationModel) error {
	return r.db.Save(application).Error
}

func (r *organizerApplicationRepository) ListForUser(userID uuid.UUID) ([]model.OrganizerApplicationModel, error) {
	applications := make([]model.OrganizerApplicationModel, 0)
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC, application_id DESC").Find(&applications).Error
	return applications, err
}

func (r *organizerApplicationRepository) List(filter model.OrganizerApplicationFilter, page, limit int) ([]model.OrganizerApplicationModel, int64, error) {
	query := r.db.Model(&model.OrganizerApplicationModel{})
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	applications := make([]model.OrganizerApplicationModel, 0)
	err := query.Order("created_at, application_id").Offset((page - 1) * limit).Limit(limit).Find(&applications).Error
	return applications, total, err
}
