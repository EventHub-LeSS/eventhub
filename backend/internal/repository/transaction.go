package repository

import "gorm.io/gorm"

type Tx struct {
	Events        EventRepository
	Organizations OrganizationRepository
	Users         UserRepository
}

type Transactor interface {
	InTransaction(fn func(tx Tx) error) error
}

type transactor struct {
	db *gorm.DB
}

func NewTransactor(db *gorm.DB) Transactor {
	return &transactor{db: db}
}

func (t *transactor) InTransaction(fn func(tx Tx) error) error {
	return t.db.Transaction(func(gdb *gorm.DB) error {
		return fn(Tx{
			Events:        NewEventRepository(gdb),
			Organizations: NewOrganizationRepository(gdb),
			Users:         NewUserRepository(gdb),
		})
	})
}
