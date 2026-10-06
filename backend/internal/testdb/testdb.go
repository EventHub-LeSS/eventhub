// Package testdb provides the Postgres database for tests that need a real one.
package testdb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// lockKey identifies the advisory lock that serializes database tests; the value is arbitrary.
const lockKey = 770077

// Open returns a migrated, empty database for the test, or skips the test when
// TEST_DATABASE_DSN is not set.
//
// Every call truncates all tables, so point TEST_DATABASE_DSN at a throwaway instance and never
// at your development database, e.g.:
//
//	docker run -d --rm -p 5599:5432 -e POSTGRES_PASSWORD=test postgres:16
//	TEST_DATABASE_DSN=postgres://postgres:test@localhost:5599/postgres?sslmode=disable go test ./...
//
// go test runs packages in parallel and all of them share this database, so Open holds a Postgres
// advisory lock until the test ends: database tests run one at a time, even across packages.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set")
	}

	lock(t, dsn)
	migrateUp(t, dsn)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	t.Cleanup(func() {
		if gormDB, err := db.DB(); err == nil {
			gormDB.Close()
		}
	})
	if err := db.Exec("TRUNCATE bookings, events, organizations, users, categories, locations CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

// lock takes the advisory lock on a dedicated connection and releases it when the test ends.
// Cleanups run last-in-first-out, so the lock outlives every connection the test opens afterwards.
func lock(t *testing.T, dsn string) {
	t.Helper()
	ctx := context.Background()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		sqlDB.Close()
		t.Fatalf("connect to database: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		conn.Close()
		sqlDB.Close()
		t.Fatalf("lock test database: %v", err)
	}
	t.Cleanup(func() {
		// Closing the session would release the lock as well; unlocking first frees it at once.
		conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
		conn.Close()
		sqlDB.Close()
	})
}

func migrateUp(t *testing.T, dsn string) {
	t.Helper()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer sqlDB.Close()

	source, err := iofs.New(os.DirFS(migrationsDir()), ".")
	if err != nil {
		t.Fatalf("migration source: %v", err)
	}
	driver, err := migratePostgres.WithInstance(sqlDB, &migratePostgres.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
}

// migrationsDir locates backend/migrations independently of the working directory of the test.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}
