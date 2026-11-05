package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"context"
	"errors"
	"testing"
)

func TestModerationWithdrawEvent(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			base, events, entries := newTestServiceWithAudit(event)
			svc := NewModerationService(base.tx, nil)
			result, err := svc.WithdrawEvent(auditCtx(audit.EventModerated), event.EventID, "Confirmed fraud")
			if status == model.EventStatusCompleted {
				if !errors.Is(err, ErrInvalidStatus) || events.saved != nil || len(entries.entries) != 0 {
					t.Fatalf("completed event changed: %v", err)
				}
				return
			}
			if err != nil || result.Status != model.EventStatusCancelled || result.Changed != (status != model.EventStatusCancelled) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if status == model.EventStatusCancelled {
				if events.saved != nil || len(entries.entries) != 0 {
					t.Fatal("idempotent withdrawal wrote data")
				}
				return
			}
			entry := singleEntry(t, entries)
			if events.saved.Status != model.EventStatusCancelled || entry.Action != string(audit.EventModerated) || entry.ActorSubject != "sub-anna" || entry.Changes["reason"] != "Confirmed fraud" || entry.OrganizationID != orgB.KeycloakOrgID {
				t.Fatalf("missing status or attribution: %+v", entry)
			}
		})
	}
}

func TestModerationWithdrawEventFailures(t *testing.T) {
	event := eventOwnedBy(nil, model.EventStatusPublished)
	base, events, entries := newTestServiceWithAudit(event)
	svc := NewModerationService(base.tx, nil)
	if _, err := svc.WithdrawEvent(context.Background(), event.EventID, "fraud"); !errors.Is(err, audit.ErrMissingContext) || events.saved != nil {
		t.Fatalf("missing context: %v", err)
	}
	entries.err = errors.New("audit unavailable")
	if _, err := svc.WithdrawEvent(auditCtx(audit.EventModerated), event.EventID, "fraud"); !errors.Is(err, audit.ErrUnavailable) {
		t.Fatalf("audit failure: %v", err)
	}
	events.event = nil
	if _, err := svc.WithdrawEvent(auditCtx(audit.EventModerated), event.EventID, "fraud"); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
}

func TestModerationEditEventContent(t *testing.T) {
	for _, status := range []model.EventStatus{model.EventStatusDraft, model.EventStatusPublished, model.EventStatusCancelled, model.EventStatusCompleted} {
		t.Run(string(status), func(t *testing.T) {
			event := eventOwnedBy(orgB, status)
			before := *event
			base, events, entries := newTestServiceWithAudit(event)
			svc := NewModerationService(base.tx, nil)
			title, description := "Corrected title", "Corrected description"
			req := model.ModerateEventContentRequest{Title: &title, Description: &description, Reason: "Offensive content"}
			result, err := svc.EditEventContent(auditCtx(audit.EventModerated), event.EventID, req)
			if err != nil || !result.Changed || result.Status != status {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if events.saved.Title != title || *events.saved.Description != description || events.saved.Capacity != before.Capacity || !events.saved.Price.Equal(before.Price) || events.saved.OrganizerID != before.OrganizerID || events.saved.Status != before.Status || events.saved.StartTime != before.StartTime {
				t.Fatalf("unexpected event: %+v", events.saved)
			}
			entry := singleEntry(t, entries)
			if entry.Changes["reason"] != req.Reason || entry.ActorSubject != "sub-anna" {
				t.Fatalf("missing attribution: %+v", entry)
			}
			result, err = svc.EditEventContent(auditCtx(audit.EventModerated), event.EventID, req)
			if err != nil || result.Changed || len(entries.entries) != 1 {
				t.Fatalf("idempotent edit: %+v %v", result, err)
			}
		})
	}
}
