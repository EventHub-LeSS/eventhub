package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating events table...")
		_, err := db.Exec(`CREATE TYPE event_status AS ENUM ('draft', 'published', 'cancelled', 'completed')`)
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS events (
				event_id UUID PRIMARY KEY,
				title TEXT NOT NULL,
				description TEXT,
				start_time TIMESTAMP NOT NULL,
				end_time TIMESTAMP NOT NULL,
				capacity INT NOT NULL,
				status event_status NOT NULL DEFAULT 'draft',
				price NUMERIC(12,2) NOT NULL,
				category_id UUID REFERENCES categories(category_id) ON DELETE SET NULL,
				organizer_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
				location_id UUID REFERENCES locations(location_id) ON DELETE SET NULL,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)

		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping events table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS events`)
		if err != nil {
			return err
		}
		fmt.Println("Dropping event_status enum...")
		_, err = db.Exec(`DROP TYPE IF EXISTS event_status`)
		if err != nil {
			return err
		}
		return nil

	})
}
