package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating private_users table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS payments (
				user_id UUID PRIMARY KEY,
				first_name TEXT NOT NULL,
				last_name TEXT NOT NULL
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping payments table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS payments`)
		return err

	})
}
