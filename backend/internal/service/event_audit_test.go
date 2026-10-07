package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func singleEntry(t *testing.T, repo *fakeAuditRepo) *model.AuditLogModel {
	t.Helper()
	if len(repo.entries) != 1 {
		t.Fatalf("expected exactly one audit entry, got %d", len(repo.entries))
	}
	return repo.entries[0]
}

func wantPersonalAttribution(t *testing.T, e *model.AuditLogModel, action audit.Action) {
	t.Helper()
	if e.ActorSubject != "sub-anna" || e.ActorUsername != "anna@acme.test" {
		t.Errorf("actor = %q/%q, want the personal account", e.ActorSubject, e.ActorUsername)
	}
	if e.OrganizationID != orgB.KeycloakOrgID {
		t.Errorf("organization = %q, want %q", e.OrganizationID, orgB.KeycloakOrgID)
	}
	if e.Action != string(action) || e.Phase != audit.PhaseCompleted || e.ResourceType != audit.ResourceEvent {
		t.Errorf("unexpected entry: %+v", e)
	}
	if e.OperationID == uuid.Nil {
		t.Error("operation ID not set")
	}
}

func TestUpdateEvent_RecordsPriceChangeWithActor(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusPublished)
	svc, _, auditRepo := newTestServiceWithAudit(event)
	req := updateRequest()
	req.Price = decimal.NewFromFloat(35.5)

	if _, err := svc.UpdateEvent(auditCtx(audit.EventUpdated), event.EventID, []string{"kc-org-b"}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entry := singleEntry(t, auditRepo)
	wantPersonalAttribution(t, entry, audit.EventUpdated)
	if entry.ResourceID != event.EventID.String() {
		t.Errorf("resource = %q, want the event", entry.ResourceID)
	}
	fields := entry.Changes["fields"].(map[string]any)
	price := fields["price"].(map[string]any)
	if price["from"] != "20" || price["to"] != "35.5" {
		t.Errorf("price change = %v, want 20 -> 35.5", price)
	}
	if _, ok := fields["title"]; !ok {
		t.Error("title change missing")
	}
}

func TestUpdateEvent_UnchangedValuesWriteNoEntry(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, _, auditRepo := newTestServiceWithAudit(event)
	req := model.UpdateEventRequest{
		Title: event.Title, Description: event.Description, StartTime: event.StartTime, EndTime: event.EndTime,
		Capacity: event.Capacity, Price: event.Price, CategoryID: *event.CategoryID, LocationID: *event.LocationID,
	}

	if _, err := svc.UpdateEvent(auditCtx(audit.EventUpdated), event.EventID, []string{"kc-org-b"}, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(auditRepo.entries) != 0 {
		t.Errorf("no-op update recorded %d entries", len(auditRepo.entries))
	}
}

func TestPublishAndWithdraw_RecordStatusChange(t *testing.T) {
	t.Run("publish", func(t *testing.T) {
		event := eventOwnedBy(orgB, model.EventStatusDraft)
		svc, _, auditRepo := newTestServiceWithAudit(event)
		if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"}); err != nil {
			t.Fatal(err)
		}
		entry := singleEntry(t, auditRepo)
		wantPersonalAttribution(t, entry, audit.EventPublished)
		status := entry.Changes["fields"].(map[string]any)["status"].(map[string]any)
		if status["from"] != "draft" || status["to"] != "published" {
			t.Errorf("status change = %v", status)
		}
	})
	t.Run("withdraw", func(t *testing.T) {
		event := eventOwnedBy(orgB, model.EventStatusPublished)
		svc, _, auditRepo := newTestServiceWithAudit(event)
		if err := svc.WithdrawEvent(auditCtx(audit.EventCancelled), event.EventID, []string{"kc-org-b"}); err != nil {
			t.Fatal(err)
		}
		entry := singleEntry(t, auditRepo)
		wantPersonalAttribution(t, entry, audit.EventCancelled)
		status := entry.Changes["fields"].(map[string]any)["status"].(map[string]any)
		if status["from"] != "published" || status["to"] != "cancelled" {
			t.Errorf("status change = %v", status)
		}
	})
}

func TestCreateDraft_RecordsSnapshot(t *testing.T) {
	svc, _, auditRepo := newTestServiceWithAudit(nil)

	created, err := svc.CreateDraft(auditCtx(audit.EventCreated), []string{"kc-org-b"}, draftRequest(""))
	if err != nil {
		t.Fatal(err)
	}
	entry := singleEntry(t, auditRepo)
	wantPersonalAttribution(t, entry, audit.EventCreated)
	if entry.ResourceID != created.EventID.String() {
		t.Errorf("resource = %q, want %q", entry.ResourceID, created.EventID)
	}
	snapshot := entry.Changes["event"].(map[string]any)
	if snapshot["title"] != "Sommerfest 2026" || snapshot["price"] != "15" || snapshot["status"] != "draft" {
		t.Errorf("snapshot = %v", snapshot)
	}
}

func TestAuditedEventActions_RequireAuditContext(t *testing.T) {
	wrongAction := auditCtx(audit.EventPublished)
	calls := map[string]func(*EventService, context.Context) error{
		"create": func(s *EventService, ctx context.Context) error {
			_, err := s.CreateDraft(ctx, []string{"kc-org-b"}, draftRequest(""))
			return err
		},
		"update": func(s *EventService, ctx context.Context) error {
			_, err := s.UpdateEvent(ctx, uuid.New(), []string{"kc-org-b"}, updateRequest())
			return err
		},
		"publish": func(s *EventService, ctx context.Context) error {
			return s.PublishEvent(ctx, uuid.New(), []string{"kc-org-b"})
		},
		"withdraw": func(s *EventService, ctx context.Context) error {
			return s.WithdrawEvent(ctx, uuid.New(), []string{"kc-org-b"})
		},
	}
	for name, call := range calls {
		for ctxName, ctx := range map[string]context.Context{"missing": context.Background(), "wrong action": wrongAction} {
			if name == "publish" && ctxName == "wrong action" {
				continue
			}
			t.Run(name+"/"+ctxName, func(t *testing.T) {
				svc, repo, auditRepo := newTestServiceWithAudit(eventOwnedBy(orgB, model.EventStatusDraft))
				if err := call(svc, ctx); !errors.Is(err, audit.ErrMissingContext) {
					t.Fatalf("expected ErrMissingContext, got %v", err)
				}
				if repo.saved != nil || repo.created != nil || len(auditRepo.entries) != 0 {
					t.Error("operation ran without audit context")
				}
			})
		}
	}
}

func TestEventActions_FailWhenAuditCannotBeWritten(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, _, auditRepo := newTestServiceWithAudit(event)
	auditRepo.err = errors.New("disk full")

	err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"})
	if !errors.Is(err, audit.ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable so the transaction is rolled back, got %v", err)
	}
}

func TestRejectedEventActions_WriteNoEntry(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusDraft)
	svc, _, auditRepo := newTestServiceWithAudit(event)

	if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-a"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	if len(auditRepo.entries) != 0 {
		t.Errorf("rejected action recorded %d entries", len(auditRepo.entries))
	}
}
