package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating locations table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS locations (
				location_id UUID PRIMARY KEY,
				name TEXT NOT NULL UNIQUE,
				city TEXT NOT NULL,
				postal_code TEXT NOT NULL,
				street TEXT NOT NULL,
				house_number TEXT,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping locations table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS locations`)
		return err
	})
}
