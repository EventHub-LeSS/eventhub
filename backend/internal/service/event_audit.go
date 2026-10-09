package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// appendEventAudit records the change in the transaction of the change itself: if the entry cannot
// be written, the change is rolled back.
func appendEventAudit(tx repository.Tx, meta audit.Meta, org *model.OrganizationModel, eventID uuid.UUID, changes model.AuditChanges) error {
	err := tx.Audit.Append(&model.AuditLogModel{
		OperationID:    meta.OperationID,
		ActorSubject:   meta.ActorSubject,
		ActorUsername:  meta.ActorUsername,
		OrganizationID: org.KeycloakOrgID,
		Action:         string(meta.Action),
		ResourceType:   audit.ResourceEvent,
		ResourceID:     eventID.String(),
		Phase:          audit.PhaseCompleted,
		Changes:        changes,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", audit.ErrUnavailable, err)
	}
	return nil
}

func statusChange(from, to model.EventStatus) model.AuditChanges {
	return model.AuditChanges{"fields": map[string]any{"status": fieldChange(string(from), string(to))}}
}

func fieldChange(from, to any) map[string]any {
	return map[string]any{"from": from, "to": to}
}

func auditUUID(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func auditString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func eventSnapshot(e *model.EventModel) map[string]any {
	return map[string]any{
		"title":       e.Title,
		"description": auditString(e.Description),
		"startTime":   e.StartTime.UTC().Format(time.RFC3339Nano),
		"endTime":     e.EndTime.UTC().Format(time.RFC3339Nano),
		"capacity":    e.Capacity,
		"status":      string(e.Status),
		"price":       e.Price.String(),
		"categoryId":  auditUUID(e.CategoryID),
		"locationId":  auditUUID(e.LocationID),
	}
}

// eventFieldChanges returns the business fields whose values differ; technical columns such as
// updatedAt do not count.
func eventFieldChanges(before, after *model.EventModel) map[string]any {
	b, a := eventSnapshot(before), eventSnapshot(after)
	changes := map[string]any{}
	for field, from := range b {
		if to := a[field]; from != to {
			changes[field] = fieldChange(from, to)
		}
	}
	return changes
}
