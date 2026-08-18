package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating users table...")
		_, err := db.Exec(`CREATE TYPE user_role AS ENUM ('administrator', 'organizer', 'visitor')`)
		_, err = db.Exec(`CREATE TYPE user_type AS ENUM ('private', 'organization')`)
		_, err = db.Exec(`
			CREATE TABLE IF NOT EXISTS users (
				user_id UUID PRIMARY KEY,
				role user_role NOT NULL,
				email TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				created_at TIMESTAMP NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP NOT NULL DEFAULT NOW()
			)`)

		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping users table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS users`)
		if err != nil {
			return err
		}
		fmt.Println("Dropping user_role and user_type enums...")
		_, err = db.Exec(`DROP TYPE IF EXISTS user_role`)
		if err != nil {
			return err
		}
		_, err = db.Exec(`DROP TYPE IF EXISTS user_type`)
		if err != nil {
			return err
		}
		return nil

	})
}
