package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error)
	GetByEmail(email string) (*model.UserModel, error)
	UpdateKeycloakUserID(userID uuid.UUID, keycloakUserID string) error
	Create(user *model.UserModel) error
	GetAll() ([]*model.UserModel, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error) {
	user := &model.UserModel{}
	err := r.db.Where("keycloak_user_id = ?", keycloakUserID).First(user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *userRepository) GetByEmail(email string) (*model.UserModel, error) {
	if email == "" {
		return nil, nil
	}
	user := &model.UserModel{}
	err := r.db.Where("LOWER(email) = LOWER(?)", email).First(user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *userRepository) UpdateKeycloakUserID(userID uuid.UUID, keycloakUserID string) error {
	return r.db.Model(&model.UserModel{}).Where("user_id = ?", userID).Update("keycloak_user_id", keycloakUserID).Error
}

func (r *userRepository) Create(user *model.UserModel) error {
	return r.db.Create(user).Error
}

func (r *userRepository) GetAll() ([]*model.UserModel, error) {
	var users []*model.UserModel
	err := r.db.Find(&users).Error
	if err != nil {
		return nil, err
	}
	return users, nil
}
