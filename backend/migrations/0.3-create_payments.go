package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating payment_status enum...")
		_, err := db.Exec(`Create TYPE payment_status AS ENUM ('pending', 'completed', 'failed', 'refunded')`)
		if err != nil {
			return err
		}

		fmt.Println("Creating payments table...")
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS payments (
				payment_id UUID PRIMARY KEY,
				amount NUMERIC(12,2) NOT NULL,
				status payment_status NOT NULL DEFAULT 'pending',
				refund_amount NUMERIC(12,2) NOT NULL DEFAULT 0.00,

				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)
		return err
	},
		func(db migrations.DB) error {
			fmt.Println("Dropping payments table...")
			_, err := db.Exec(`DROP TABLE IF EXISTS payments`)
			if err != nil {
				return err
			}
			fmt.Println("Dropping payment_status enum...")
			_, err = db.Exec(`DROP TYPE IF EXISTS payment_status`)
			return err

		},
	)
}
