package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating ratings table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS ratings (
				rating_id UUID PRIMARY KEY,
				booking_id UUID NOT NULL,
				score INT NOT NULL CHECK (score >= 1 AND score <= 5),
				text TEXT NOT NULL,
				is_visible BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping ratings table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS ratings`)
		return err
	})
}
