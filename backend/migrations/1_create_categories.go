package main

import (
	"fmt"

	"github.com/go-pg/migrations/v8"
)

func init() {
	migrations.MustRegisterTx(func(db migrations.DB) error {
		fmt.Println("Creating categories table...")
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS categories (
				category_id UUID PRIMARY KEY,
				category TEXT NOT NULL UNIQUE,
				description TEXT
			)`)
		return err
	}, func(db migrations.DB) error {
		fmt.Println("Dropping categories table...")
		_, err := db.Exec(`DROP TABLE IF EXISTS categories`)
		return err
	})
}
