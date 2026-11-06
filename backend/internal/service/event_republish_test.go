package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestRestoreEventAsDraft_OwnershipAndStatus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		event *model.EventModel
		orgs  []string
		want  error
	}{
		{"own", eventOwnedBy(orgB, model.EventStatusCancelled), []string{"kc-org-b"}, nil},
		{"alias", eventOwnedBy(orgB, model.EventStatusCancelled), []string{"alias-b"}, nil},
		{"several organizations", eventOwnedBy(orgB, model.EventStatusCancelled), []string{"kc-org-a", "kc-org-b"}, nil},
		{"foreign", eventOwnedBy(orgB, model.EventStatusCancelled), []string{"kc-org-a"}, ErrForbidden},
		{"no organizations", eventOwnedBy(orgB, model.EventStatusCancelled), nil, ErrForbidden},
		{"no organizer", eventOwnedBy(nil, model.EventStatusCancelled), []string{"kc-org-b"}, ErrForbidden},
		{"missing", nil, []string{"kc-org-b"}, ErrEventNotFound},
		{"draft", eventOwnedBy(orgB, model.EventStatusDraft), []string{"kc-org-b"}, ErrNotCancelled},
		{"published", eventOwnedBy(orgB, model.EventStatusPublished), []string{"kc-org-b"}, ErrNotCancelled},
		{"completed", eventOwnedBy(orgB, model.EventStatusCompleted), []string{"kc-org-b"}, ErrNotCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, audits := newTestServiceWithAudit(tc.event)
			eventID := uuid.New()
			if tc.event != nil {
				eventID = tc.event.EventID
			}
			restored, err := svc.RestoreEventAsDraft(auditCtx(audit.EventRestoredAsDraft), eventID, tc.orgs)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tc.want != nil {
				if restored != nil || repo.saved != nil || len(audits.entries) != 0 {
					t.Fatal("rejected operation changed data")
				}
				return
			}
			if restored.EventID != tc.event.EventID || restored.Status != model.EventStatusDraft {
				t.Fatalf("restored=%+v", restored)
			}
			entry := singleEntry(t, audits)
			wantPersonalAttribution(t, entry, audit.EventRestoredAsDraft)
			change := entry.Changes["fields"].(map[string]any)["status"].(map[string]any)
			if change["from"] != "cancelled" || change["to"] != "draft" {
				t.Fatalf("audit=%v", change)
			}
		})
	}
}
func TestRepublishCancelledEvent_ValidationAndAudit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*model.EventModel)
		want   error
	}{
		{"complete", func(*model.EventModel) {}, nil},
		{"missing category", func(e *model.EventModel) { e.CategoryID = nil }, ErrIncomplete},
		{"missing location", func(e *model.EventModel) { e.LocationID = nil }, ErrIncomplete},
		{"missing title", func(e *model.EventModel) { e.Title = "" }, ErrIncomplete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := eventOwnedBy(orgB, model.EventStatusCancelled)
			tc.mutate(event)
			svc, repo, audits := newTestServiceWithAudit(event)
			err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
			if tc.want != nil {
				if repo.saved != nil || event.Status != model.EventStatusCancelled || len(audits.entries) != 0 {
					t.Fatal("invalid republish changed data")
				}
				return
			}
			entry := singleEntry(t, audits)
			wantPersonalAttribution(t, entry, audit.EventPublished)
			change := entry.Changes["fields"].(map[string]any)["status"].(map[string]any)
			if change["from"] != "cancelled" || change["to"] != "published" {
				t.Fatalf("audit=%v", change)
			}
		})
	}
}
func TestRestoreEventAsDraft_AllowsIncompleteEventsForEditing(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusCancelled)
	event.CategoryID = nil
	event.LocationID = nil
	svc, _ := newTestService(event)
	restored, err := svc.RestoreEventAsDraft(auditCtx(audit.EventRestoredAsDraft), event.EventID, []string{"kc-org-b"})
	if err != nil || restored.Status != model.EventStatusDraft {
		t.Fatalf("restore err=%v", err)
	}
	if err := svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"}); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("publish err=%v", err)
	}
}
func TestRestoreEventAsDraft_RequiresAudit(t *testing.T) {
	event := eventOwnedBy(orgB, model.EventStatusCancelled)
	svc, repo, audits := newTestServiceWithAudit(event)
	if _, err := svc.RestoreEventAsDraft(context.Background(), event.EventID, []string{"kc-org-b"}); !errors.Is(err, audit.ErrMissingContext) {
		t.Fatalf("err=%v", err)
	}
	if repo.saved != nil || len(audits.entries) != 0 {
		t.Fatal("missing audit context changed data")
	}
	audits.err = errors.New("audit unavailable")
	if _, err := svc.RestoreEventAsDraft(auditCtx(audit.EventRestoredAsDraft), event.EventID, []string{"kc-org-b"}); !errors.Is(err, audit.ErrUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestModeratorWithdrawalCannotBeBypassed(t *testing.T) {
	for _, restore := range []bool{false, true} {
		event := eventOwnedBy(orgB, model.EventStatusCancelled)
		event.WithdrawnByModerator = true
		svc, repo, audits := newTestServiceWithAudit(event)
		var err error
		if restore {
			_, err = svc.RestoreEventAsDraft(auditCtx(audit.EventRestoredAsDraft), event.EventID, []string{"kc-org-b"})
		} else {
			err = svc.PublishEvent(auditCtx(audit.EventPublished), event.EventID, []string{"kc-org-b"})
		}
		if !errors.Is(err, ErrModerationWithdrawal) || repo.saved != nil || len(audits.entries) != 0 {
			t.Fatalf("moderation bypass err=%v", err)
		}
	}
}
