package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating notification_data table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS notification_data (
				notification_id UUID PRIMARY KEY,
				user_id UUID NOT NULL,
				subject TEXT NOT NULL,
				content TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT NOW()
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)

		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping notification_data table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS notification_data`)
		return err

	})
}
