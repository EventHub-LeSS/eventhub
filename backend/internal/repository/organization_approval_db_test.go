package repository

import (
	"backend/internal/model"
	"backend/internal/testdb"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedOrganization(t *testing.T, db *gorm.DB, active bool, createdAt time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	err := db.Exec(`INSERT INTO organizations (organization_id, keycloak_org_id, name, alias, is_active, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, "kc-"+id.String(), "Org "+id.String(), "alias-"+id.String(), active, createdAt).Error
	if err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	return id
}

func TestOrganizationRepository_ListPendingPagesOldestFirst(t *testing.T) {
	db := testdb.Open(t)
	repo := NewOrganizationRepository(db)

	base := time.Now().Add(-time.Hour).UTC()
	newest := seedOrganization(t, db, false, base.Add(3*time.Minute))
	oldest := seedOrganization(t, db, false, base.Add(time.Minute))
	middle := seedOrganization(t, db, false, base.Add(2*time.Minute))
	seedOrganization(t, db, true, base) // approved organizations are never pending

	first, total, err := repo.ListPending(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(first) != 2 || first[0].OrganizationID != oldest || first[1].OrganizationID != middle {
		t.Fatalf("first page = %v, want [%s %s]", organizationIDs(first), oldest, middle)
	}
	second, _, err := repo.ListPending(2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].OrganizationID != newest {
		t.Fatalf("second page = %v, want [%s]", organizationIDs(second), newest)
	}
	beyond, _, err := repo.ListPending(3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(beyond) != 0 {
		t.Fatalf("page beyond the end = %v, want none", organizationIDs(beyond))
	}
}

func TestOrganizationRepository_ActivateOnlyChangesPendingOrganizations(t *testing.T) {
	db := testdb.Open(t)
	repo := NewOrganizationRepository(db)
	pending := seedOrganization(t, db, false, time.Now())

	activated, err := repo.Activate(pending)
	if err != nil || !activated {
		t.Fatalf("first approval: activated=%v err=%v", activated, err)
	}
	stored, err := repo.GetByID(pending)
	if err != nil || stored == nil || !stored.IsActive {
		t.Fatalf("organization not active after approval: %+v (%v)", stored, err)
	}
	// A second approval and an unknown organization change nothing; the caller tells them apart.
	if activated, err := repo.Activate(pending); err != nil || activated {
		t.Fatalf("second approval: activated=%v err=%v", activated, err)
	}
	if activated, err := repo.Activate(uuid.New()); err != nil || activated {
		t.Fatalf("unknown organization: activated=%v err=%v", activated, err)
	}
	pendingNow, total, err := repo.ListPending(1, 10)
	if err != nil || total != 0 || len(pendingNow) != 0 {
		t.Fatalf("approved organization still pending: %v total=%d err=%v", organizationIDs(pendingNow), total, err)
	}
}

func organizationIDs(orgs []*model.OrganizationModel) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(orgs))
	for _, org := range orgs {
		ids = append(ids, org.OrganizationID)
	}
	return ids
}
