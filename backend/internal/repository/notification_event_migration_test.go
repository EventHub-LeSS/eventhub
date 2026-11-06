package repository

import (
	"backend/internal/testdb"
	"os"
	"testing"

	"github.com/google/uuid"
)

// Migration 000016 (EVENTHUB-291) lets cancellation notices reference their event. An event can be
// withdrawn several times (EVENTHUB-290), so a user may hold several notices for the same event.
func TestNotificationEventMigrationAllowsRepeatedCancellations(t *testing.T) {
	db := testdb.Open(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	// Temporary tables shadow the real tables only on this connection.
	for _, stmt := range []string{
		"CREATE TEMP TABLE events (event_id UUID PRIMARY KEY) ON COMMIT DROP",
		"CREATE TEMP TABLE notification_data (notification_id UUID PRIMARY KEY, user_id UUID, occasion TEXT NOT NULL) ON COMMIT DROP",
	} {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	up, err := os.ReadFile("../../migrations/000016_notification_event.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(up)).Error; err != nil {
		t.Fatal(err)
	}
	var unique bool
	if err := tx.Raw("SELECT indisunique FROM pg_index WHERE indexrelid = 'pg_temp.idx_notification_event'::regclass").Scan(&unique).Error; err != nil {
		t.Fatal(err)
	}
	if unique {
		t.Fatal("idx_notification_event must not be unique")
	}

	event, user := uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO events (event_id) VALUES (?)", event).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := tx.Exec("INSERT INTO notification_data (notification_id, user_id, occasion, event_id) VALUES (?, ?, 'event_cancelled', ?)",
			uuid.New(), user, event).Error; err != nil {
			t.Fatalf("repeated cancellation notice rejected: %v", err)
		}
	}
	if err := tx.Exec("DELETE FROM events WHERE event_id = ?", event).Error; err != nil {
		t.Fatal(err)
	}
	var referencing int64
	if err := tx.Raw("SELECT count(*) FROM notification_data WHERE event_id IS NOT NULL").Scan(&referencing).Error; err != nil {
		t.Fatal(err)
	}
	if referencing != 0 {
		t.Fatalf("%d notices still reference the deleted event", referencing)
	}

	down, err := os.ReadFile("../../migrations/000016_notification_event.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(down)).Error; err != nil {
		t.Fatal(err)
	}
	var column, index bool
	if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = 'pg_temp.notification_data'::regclass AND attname = 'event_id' AND NOT attisdropped)").Scan(&column).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Raw("SELECT to_regclass('pg_temp.idx_notification_event') IS NOT NULL").Scan(&index).Error; err != nil {
		t.Fatal(err)
	}
	if column || index {
		t.Fatalf("down migration left column=%v index=%v", column, index)
	}
}
