package main

import (
	"database/sql"
	"errors"
	"log"

	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"

	"backend/internal/db"
)

func main() {
	config := db.LoadConfig()

	sqlDB, err := sql.Open("pgx", config.DSN())
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("connect database: %v", err)
	}

	driver, err := migratePostgres.WithInstance(
		sqlDB,
		&migratePostgres.Config{},
	)
	if err != nil {
		log.Fatalf("create migration driver: %v", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file:///app/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		log.Fatalf("create migrator: %v", err)
	}

	err = m.Up()

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migration failed: %v", err)
	}

	log.Println("database migrations complete")
}
