package repository

import (
	"backend/internal/testdb"
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestModeratorWithdrawalMigrationPreservesExistingBlocks(t *testing.T) {
	db := testdb.Open(t)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	// Temporary tables shadow the real tables only on this connection.
	for _, stmt := range []string{
		"CREATE TEMP TABLE events (event_id UUID PRIMARY KEY,status TEXT NOT NULL) ON COMMIT DROP",
		"CREATE TEMP TABLE audit_logs (resource_type TEXT,resource_id TEXT,action TEXT,changes JSONB) ON COMMIT DROP",
	} {
		if err := tx.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	moderated := uuid.New()
	for _, row := range []struct {
		id                      uuid.UUID
		status, action, changes string
	}{
		{moderated, "cancelled", "event.moderated", `{"fields":{"status":{"from":"published","to":"cancelled"}}}`},
		{uuid.New(), "cancelled", "event.cancelled", `{"fields":{"status":{"to":"cancelled"}}}`},
		{uuid.New(), "cancelled", "event.moderated", `{"fields":{"title":{"to":"Corrected"}}}`},
		{uuid.New(), "completed", "event.moderated", `{"fields":{"status":{"to":"cancelled"}}}`},
	} {
		if err := tx.Exec("INSERT INTO events (event_id,status) VALUES (?,?)", row.id, row.status).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("INSERT INTO audit_logs (resource_type,resource_id,action,changes) VALUES ('event',?,?,?::jsonb)", row.id.String(), row.action, row.changes).Error; err != nil {
			t.Fatal(err)
		}
	}
	up, err := os.ReadFile("../../migrations/000014_moderator_withdrawal.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(up)).Error; err != nil {
		t.Fatal(err)
	}
	var blocked []uuid.UUID
	if err := tx.Raw("SELECT event_id FROM events WHERE withdrawn_by_moderator").Scan(&blocked).Error; err != nil {
		t.Fatal(err)
	}
	if len(blocked) != 1 || blocked[0] != moderated {
		t.Fatalf("backfilled=%v", blocked)
	}
	down, err := os.ReadFile("../../migrations/000014_moderator_withdrawal.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(down)).Error; err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid='pg_temp.events'::regclass AND attname='withdrawn_by_moderator' AND NOT attisdropped)").Scan(&exists).Error; err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("down migration did not remove column")
	}
}
