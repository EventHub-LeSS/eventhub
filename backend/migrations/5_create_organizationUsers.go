package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating organization_users table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS organization_users (
				user_id UUID PRIMARY KEY,
				organization_name TEXT NOT NULL,
				contact_person_name TEXT 
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping organization_users table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS organization_users`)
		return err

	})
}
