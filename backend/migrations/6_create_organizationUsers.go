package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating payments table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS payments (
				user_id UUID PRIMARY KEY,
				organizaiton_name TEXT NOT NULL,
				contact_person_name TEXT 
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping payments table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS payments`)
		return err

	})
}
