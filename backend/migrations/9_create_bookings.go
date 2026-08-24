package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating bookings table...")
		_, err := db.Exec(`CREATE TYPE booking_status AS ENUM ('reserved', 'confirmed', 'cancelled', 'failed')`)
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS bookings (
				booking_id UUID PRIMARY KEY,
				user_id UUID REFERENCES users(user_id) ON DELETE SET NULL,
				event_id UUID REFERENCES events(event_id) ON DELETE SET NULL,
				payment_id UUID REFERENCES payments(payment_id) ON DELETE SET NULL,
				number_of_tickets INT NOT NULL CHECK (number_of_tickets > 0),
				status booking_status NOT NULL DEFAULT 'reserved',
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)

		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping bookings table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS bookings`)
		if err != nil {
			return err
		}
		fmt.Println("Dropping booking_status enum...")
		_, err = db.Exec(`DROP TYPE IF EXISTS booking_status`)
		if err != nil {
			return err
		}
		return nil

	})
}
