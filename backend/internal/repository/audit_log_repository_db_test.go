package repository

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/testdb"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func insertAuditRow(t *testing.T, db *gorm.DB, orgID, action string, occurredAt any) {
	t.Helper()
	err := db.Exec(`INSERT INTO audit_logs (audit_log_id, operation_id, occurred_at, actor_subject, actor_username, organization_id, action, resource_type, resource_id, phase)
		VALUES (?, ?, ?, 'sub', 'user', ?, ?, 'event', 'e', 'completed')`, uuid.New(), uuid.New(), occurredAt, orgID, action).Error
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuditLogRepository_AppendAndKeysetPaginationWithEqualTimestamps(t *testing.T) {
	db := testdb.Open(t)
	repo := NewAuditLogRepository(db)
	same := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		insertAuditRow(t, db, "org-1", fmt.Sprintf("a%d", i), same)
	}
	insertAuditRow(t, db, "org-2", "foreign", same)

	seen := map[uuid.UUID]bool{}
	var cursor *model.AuditLogCursor
	for pages := 0; pages < 5; pages++ {
		entries, err := repo.ListByOrganization("org-1", 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if seen[e.AuditLogID] {
				t.Fatalf("entry %s returned twice", e.AuditLogID)
			}
			if e.OrganizationID != "org-1" {
				t.Fatalf("foreign entry %+v", e)
			}
			seen[e.AuditLogID] = true
		}
		if len(entries) < 2 {
			break
		}
		last := entries[len(entries)-1]
		cursor = &model.AuditLogCursor{OccurredAt: last.OccurredAt, ID: last.AuditLogID}
	}
	if len(seen) != 5 {
		t.Errorf("walked %d entries, want 5", len(seen))
	}

	// A cursor from another organization's page never exposes foreign entries.
	foreign, err := repo.ListByOrganization("org-1", 10, &model.AuditLogCursor{OccurredAt: same.Add(time.Hour), ID: uuid.New()})
	if err != nil || len(foreign) != 5 {
		t.Errorf("got %d entries (err %v), want only the 5 of org-1", len(foreign), err)
	}
}

func TestAuditLogRepository_AppendStoresEntryAndRejectsInvalidOnes(t *testing.T) {
	db := testdb.Open(t)
	repo := NewAuditLogRepository(db)
	entry := &model.AuditLogModel{
		OperationID: uuid.New(), ActorSubject: "sub-anna", ActorUsername: "anna", OrganizationID: "org-1",
		Action: string(audit.EventUpdated), ResourceType: audit.ResourceEvent, ResourceID: "e", Phase: audit.PhaseCompleted,
		Changes: model.AuditChanges{"fields": map[string]any{"price": map[string]any{"from": "1", "to": "2"}}},
	}
	if err := repo.Append(entry); err != nil {
		t.Fatal(err)
	}
	entries, err := repo.ListByOrganization("org-1", 10, nil)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
	if entries[0].OccurredAt.IsZero() || time.Since(entries[0].OccurredAt) > time.Minute {
		t.Errorf("occurredAt = %s, want the database time of the insert", entries[0].OccurredAt)
	}
	if entries[0].Changes["fields"].(map[string]any)["price"].(map[string]any)["to"] != "2" {
		t.Errorf("changes = %v", entries[0].Changes)
	}

	for name, mutate := range map[string]func(*model.AuditLogModel){
		"no actor":        func(e *model.AuditLogModel) { e.ActorSubject = "" },
		"no organization": func(e *model.AuditLogModel) { e.OrganizationID = "" },
		"unknown phase":   func(e *model.AuditLogModel) { e.Phase = "weird" },
	} {
		bad := *entry
		bad.AuditLogID = uuid.Nil
		mutate(&bad)
		if err := repo.Append(&bad); err == nil {
			t.Errorf("%s: invalid entry was accepted", name)
		}
	}
}

func TestAuditLogRetention_DeletesOnlyEntriesOlderThanTenYears(t *testing.T) {
	db := testdb.Open(t)
	const old = "((now() AT TIME ZONE 'UTC') - INTERVAL '10 years') AT TIME ZONE 'UTC'"
	rows := []struct {
		action string
		at     string
		kept   bool
	}{
		{"just-expired", old + " - INTERVAL '1 minute'", false},
		{"ancient", "TIMESTAMPTZ '2001-02-28 12:00+00'", false},
		{"just-inside", old + " + INTERVAL '1 minute'", true},
		{"nine-years", "now() - INTERVAL '9 years'", true},
		{"fresh", "now()", true},
	}
	for _, r := range rows {
		err := db.Exec(`INSERT INTO audit_logs (audit_log_id, operation_id, occurred_at, actor_subject, organization_id, action, resource_type, resource_id, phase)
			VALUES (gen_random_uuid(), gen_random_uuid(), `+r.at+`, 'sub', 'org-1', ?, 'event', 'e', 'started')`, r.action).Error
		if err != nil {
			t.Fatal(err)
		}
	}
	seeded := seedCategory(t, db)

	var deleted int64
	if err := db.Raw("SELECT purge_expired_audit_logs()").Scan(&deleted).Error; err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Errorf("deleted %d entries, want 2", deleted)
	}
	var remaining []string
	db.Raw("SELECT action FROM audit_logs ORDER BY action").Scan(&remaining)
	if fmt.Sprint(remaining) != "[fresh just-inside nine-years]" {
		t.Errorf("remaining = %v", remaining)
	}
	var categories int64
	db.Raw("SELECT count(*) FROM categories WHERE category_id = ?", seeded).Scan(&categories)
	if categories != 1 {
		t.Error("retention touched another table")
	}
}

func seedCategory(t *testing.T, db *gorm.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := db.Exec("INSERT INTO categories (category_id, category) VALUES (?, ?)", id, "c-"+id.String()).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

// The job is executed by pg_cron itself, not just registered: its schedule is shortened to one
// second for the test and restored afterwards.
func TestAuditLogRetention_JobIsScheduledAndRunsInTheDatabase(t *testing.T) {
	db := testdb.Open(t)

	type job struct {
		JobID    int64
		Schedule string
		Command  string
		Active   bool
	}
	var jobs []job
	if err := db.Raw("SELECT jobid AS job_id, schedule, command, active FROM cron.job WHERE jobname = 'audit-log-retention'").Scan(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Schedule != "0 3 * * *" || !jobs[0].Active || jobs[0].Command != "SELECT purge_expired_audit_logs()" {
		t.Fatalf("jobs = %+v, want exactly one active daily job", jobs)
	}

	insertAuditRow(t, db, "org-1", "expired", time.Now().UTC().AddDate(-10, 0, -1))
	insertAuditRow(t, db, "org-1", "recent", time.Now().UTC())

	if err := db.Exec("SELECT cron.alter_job(?, schedule := '1 seconds')", jobs[0].JobID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("SELECT cron.alter_job(?, schedule := ?)", jobs[0].JobID, jobs[0].Schedule) })

	deadline := time.Now().Add(30 * time.Second)
	for {
		var expired int64
		db.Raw("SELECT count(*) FROM audit_logs WHERE action = 'expired'").Scan(&expired)
		var succeeded int64
		db.Raw("SELECT count(*) FROM cron.job_run_details WHERE jobid = ? AND status = 'succeeded'", jobs[0].JobID).Scan(&succeeded)
		if expired == 0 && succeeded > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("pg_cron did not run the retention job (expired rows left: %d, successful runs: %d)", expired, succeeded)
		}
		time.Sleep(500 * time.Millisecond)
	}
	var recent int64
	db.Raw("SELECT count(*) FROM audit_logs WHERE action = 'recent'").Scan(&recent)
	if recent != 1 {
		t.Error("the scheduled job deleted a recent entry")
	}
}

func TestAuditLogRetention_MigrationCanBeRepeated(t *testing.T) {
	db := testdb.Open(t)
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000007_audit_log_retention.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(sql)).Error; err != nil {
		t.Fatalf("repeating the migration failed: %v", err)
	}
	var jobs int64
	db.Raw("SELECT count(*) FROM cron.job WHERE jobname = 'audit-log-retention'").Scan(&jobs)
	if jobs != 1 {
		t.Errorf("%d retention jobs after repeating the migration, want 1", jobs)
	}
}
