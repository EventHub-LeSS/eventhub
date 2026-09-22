package repository

import (
	"backend/internal/model"
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository interface {
	GetByID(userID uuid.UUID) (*model.UserModel, error)
	GetByKeycloakUserID(keycloakUserID string) (*model.UserModel, error)
	Create(user *model.UserModel) error
	GetAll() ([]*model.UserModel, error)
	GetPage(ctx context.Context, page, limit int) ([]*model.UserModel, int64, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByID(userID uuid.UUID) (*model.UserModel, error) {
	user := &model.UserModel{}
	err := r.db.Where("user_id = ?", userID).First(user).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return user, nil
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

func (r *userRepository) GetPage(ctx context.Context, page, limit int) ([]*model.UserModel, int64, error) {
	if page < 1 || limit < 1 {
		return nil, 0, fmt.Errorf("page and limit must be positive")
	}
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.UserModel{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	users := make([]*model.UserModel, 0)
	// Check the page boundary before multiplying, including for MaxInt page requests.
	if total == 0 || int64(page-1) > (total-1)/int64(limit) {
		return users, total, nil
	}
	if page-1 > math.MaxInt/limit {
		return nil, 0, fmt.Errorf("page offset exceeds supported range")
	}
	err := r.db.WithContext(ctx).Order("user_id ASC").
		Limit(limit).Offset((page - 1) * limit).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}
