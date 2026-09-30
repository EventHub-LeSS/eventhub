package service

import (
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Runs against a real Postgres because the overbooking guarantee depends on
// database transactions and locking. Set TEST_DATABASE_DSN to enable, e.g.:
// postgres://postgres:test@localhost:5599/postgres?sslmode=disable
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set")
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	driver, err := migratePostgres.WithInstance(sqlDB, &migratePostgres.Config{})
	if err != nil {
		t.Fatalf("migration driver: %v", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://../../migrations", "postgres", driver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	sqlDB.Close()

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	t.Cleanup(func() {
		if gormDB, err := db.DB(); err == nil {
			gormDB.Close()
		}
	})
	err = db.Exec("TRUNCATE bookings, events, organizations, users, categories, locations CASCADE").Error
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func seedEvent(t *testing.T, db *gorm.DB, capacity int, status model.EventStatus) (eventID, userID uuid.UUID) {
	t.Helper()
	categoryID, locationID, orgID := uuid.New(), uuid.New(), uuid.New()
	eventID, userID = uuid.New(), uuid.New()

	statements := []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO categories (category_id, category) VALUES (?, ?)", []any{categoryID, "cat-" + categoryID.String()}},
		{"INSERT INTO locations (location_id, name, city, postal_code, street) VALUES (?, ?, ?, ?, ?)", []any{locationID, "loc-" + locationID.String(), "Bonn", "53111", "Weg"}},
		{"INSERT INTO organizations (organization_id, keycloak_org_id, name) VALUES (?, ?, ?)", []any{orgID, "kc-org-" + orgID.String(), "Org"}},
		{"INSERT INTO users (user_id, keycloak_user_id, first_name, last_name, email) VALUES (?, ?, ?, ?, ?)", []any{userID, "kc-" + userID.String(), "Max", "Muster", userID.String() + "@test.de"}},
		{`INSERT INTO events (event_id, title, start_time, end_time, capacity, status, price, category_id, organizer_id, location_id)
		  VALUES (?, ?, NOW() + interval '1 day', NOW() + interval '2 day', ?, ?, 0, ?, ?, ?)`,
			[]any{eventID, "Konzert", capacity, string(status), categoryID, orgID, locationID}},
	}
	for _, st := range statements {
		if err := db.Exec(st.sql, st.args...).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return eventID, userID
}

func reservedTickets(t *testing.T, db *gorm.DB, eventID uuid.UUID) int64 {
	t.Helper()
	var sum int64
	err := db.Raw("SELECT COALESCE(SUM(number_of_tickets), 0) FROM bookings WHERE event_id = ? AND status = 'reserved'", eventID).Scan(&sum).Error
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return sum
}

func TestReserveTickets_ParallelRequestsNeverOverbook(t *testing.T) {
	db := setupTestDB(t)
	eventID, userID := seedEvent(t, db, 1, model.EventStatusPublished)
	svc := NewBookingService(repository.NewBookingRepository(db), 0)

	const attempts = 50
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(attempts)
	sqlDB.SetMaxIdleConns(attempts)

	// Every goroutine warms up its own pooled connection first, so that the
	// actual reservations hit the database at the same moment.
	start := make(chan struct{})
	results := make(chan error, attempts)
	var ready, wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		ready.Add(1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			db.Exec("SELECT 1")
			ready.Done()
			<-start
			_, err := svc.ReserveTickets(context.Background(), eventID, userID, 1)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	wg.Wait()
	close(results)

	successes, rejected := 0, 0
	for err := range results {
		var capErr *CapacityExceededError
		switch {
		case err == nil:
			successes++
		case errors.As(err, &capErr):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successes != 1 || rejected != attempts-1 {
		t.Errorf("expected exactly 1 success and %d rejections, got %d successes and %d rejections", attempts-1, successes, rejected)
	}
	if sum := reservedTickets(t, db, eventID); sum != 1 {
		t.Errorf("overbooked: %d tickets reserved for capacity 1", sum)
	}
}

func TestReserveTickets_CapacityMath(t *testing.T) {
	db := setupTestDB(t)
	eventID, userID := seedEvent(t, db, 3, model.EventStatusPublished)
	svc := NewBookingService(repository.NewBookingRepository(db), 0)
	ctx := context.Background()

	if _, err := svc.ReserveTickets(ctx, eventID, userID, 2); err != nil {
		t.Fatalf("first reservation: %v", err)
	}

	var capErr *CapacityExceededError
	_, err := svc.ReserveTickets(ctx, eventID, userID, 2)
	if !errors.As(err, &capErr) {
		t.Fatalf("expected CapacityExceededError, got %v", err)
	}
	if capErr.Available != 1 || capErr.Requested != 2 {
		t.Errorf("expected available=1 requested=2, got available=%d requested=%d", capErr.Available, capErr.Requested)
	}

	if _, err := svc.ReserveTickets(ctx, eventID, userID, 1); err != nil {
		t.Fatalf("last seat: %v", err)
	}
	_, err = svc.ReserveTickets(ctx, eventID, userID, 1)
	if !errors.As(err, &capErr) || capErr.Available != 0 {
		t.Errorf("expected sold out (available=0), got %v", err)
	}
	if sum := reservedTickets(t, db, eventID); sum != 3 {
		t.Errorf("expected 3 reserved tickets, got %d", sum)
	}
}

func TestReserveTickets_ExpiredReservationFreesCapacity(t *testing.T) {
	db := setupTestDB(t)
	eventID, userID := seedEvent(t, db, 1, model.EventStatusPublished)
	svc := NewBookingService(repository.NewBookingRepository(db), 0)

	err := db.Exec(`INSERT INTO bookings (booking_id, user_id, event_id, number_of_tickets, status, expires_at)
	                VALUES (?, ?, ?, 1, 'reserved', NOW() - interval '1 hour')`, uuid.New(), userID, eventID).Error
	if err != nil {
		t.Fatalf("seed expired booking: %v", err)
	}

	booking, err := svc.ReserveTickets(context.Background(), eventID, userID, 1)
	if err != nil {
		t.Fatalf("expected expired reservation to free the seat, got %v", err)
	}
	if booking.ExpiresAt == nil || booking.ExpiresAt.Before(time.Now().Add(14*time.Minute)) {
		t.Errorf("expected expiry about 15 minutes ahead, got %v", booking.ExpiresAt)
	}
}

func TestReserveTickets_RejectsUnpublishedEvent(t *testing.T) {
	db := setupTestDB(t)
	eventID, userID := seedEvent(t, db, 10, model.EventStatusDraft)
	svc := NewBookingService(repository.NewBookingRepository(db), 0)

	_, err := svc.ReserveTickets(context.Background(), eventID, userID, 1)
	if !errors.Is(err, ErrEventNotPublished) {
		t.Errorf("expected ErrEventNotPublished, got %v", err)
	}
	if sum := reservedTickets(t, db, eventID); sum != 0 {
		t.Errorf("no booking must be created, got %d tickets", sum)
	}
}

func TestReserveTickets_UnknownEvent(t *testing.T) {
	db := setupTestDB(t)
	svc := NewBookingService(repository.NewBookingRepository(db), 0)

	_, err := svc.ReserveTickets(context.Background(), uuid.New(), uuid.New(), 1)
	if !errors.Is(err, ErrEventNotFound) {
		t.Errorf("expected ErrEventNotFound, got %v", err)
	}
}

func TestReserveTickets_RejectsNonPositiveCount(t *testing.T) {
	svc := NewBookingService(nil, 0)
	for _, n := range []int{0, -1} {
		if _, err := svc.ReserveTickets(context.Background(), uuid.New(), uuid.New(), n); !errors.Is(err, ErrInvalidTicketCount) {
			t.Errorf("tickets=%d: expected ErrInvalidTicketCount, got %v", n, err)
		}
	}
}
