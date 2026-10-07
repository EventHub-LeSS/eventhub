package repository

import (
	"context"

	"gorm.io/gorm"
)

type Tx struct {
	Events        EventRepository
	Organizations OrganizationRepository
	Users         UserRepository
	Audit         AuditLogRepository
}

type Transactor interface {
	InTransaction(ctx context.Context, fn func(tx Tx) error) error
}

type transactor struct {
	db *gorm.DB
}

func NewTransactor(db *gorm.DB) Transactor {
	return &transactor{db: db}
}

func (t *transactor) InTransaction(ctx context.Context, fn func(tx Tx) error) error {
	return t.db.WithContext(ctx).Transaction(func(gdb *gorm.DB) error {
		return fn(Tx{
			Events:        NewEventRepository(gdb),
			Organizations: NewOrganizationRepository(gdb),
			Users:         NewUserRepository(gdb),
			Audit:         NewAuditLogRepository(gdb),
		})
	})
}
