package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrRatingNotFound = errors.New("rating not found")

// ModerationService lets platform moderators hide problematic user content. Every change is
// recorded in the audit log in the same transaction, together with the reason.
type ModerationService struct {
	tx      repository.Transactor
	ratings repository.ModerationRepository
}

func NewModerationService(tx repository.Transactor, ratings repository.ModerationRepository) *ModerationService {
	return &ModerationService{tx: tx, ratings: ratings}
}

func (s *ModerationService) ListRatings(filter model.ModeratedRatingFilter, page, limit int) (*model.ModeratedRatingPage, error) {
	items, total, err := s.ratings.ListRatings(filter, page, limit)
	if err != nil {
		return nil, err
	}
	return &model.ModeratedRatingPage{Items: items, Page: page, Limit: limit, Total: total}, nil
}

func (s *ModerationService) ListEvents(filter model.ModeratedEventFilter, page, limit int) (*model.ModeratedEventPage, error) {
	items, total, err := s.ratings.ListEvents(filter, page, limit)
	if err != nil {
		return nil, err
	}
	return &model.ModeratedEventPage{Items: items, Page: page, Limit: limit, Total: total}, nil
}

func (s *ModerationService) GetEvent(eventID uuid.UUID) (*model.ModeratedEvent, error) {
	items, _, err := s.ratings.ListEvents(model.ModeratedEventFilter{EventID: &eventID}, 1, 1)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrEventNotFound
	}
	return &items[0], nil
}

func (s *ModerationService) EditEventContent(ctx context.Context, eventID uuid.UUID, req model.ModerateEventContentRequest) (*model.EventModerationResponse, error) {
	meta, err := audit.MetaFromContext(ctx, audit.EventModerated)
	if err != nil {
		return nil, err
	}
	result := &model.EventModerationResponse{EventID: eventID}
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		before := *event
		if req.Title != nil {
			event.Title = *req.Title
		}
		if req.Description != nil {
			event.Description = req.Description
		}
		result.Status = event.Status
		fields := eventFieldChanges(&before, event)
		if len(fields) == 0 {
			return nil
		}
		if err := tx.Events.UpdateEvent(event); err != nil {
			return err
		}
		if err := appendModeratedEventAudit(tx, meta, event, model.AuditChanges{"fields": fields, "reason": req.Reason}); err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *ModerationService) WithdrawEvent(ctx context.Context, eventID uuid.UUID, reason string) (*model.EventModerationResponse, error) {
	meta, err := audit.MetaFromContext(ctx, audit.EventModerated)
	if err != nil {
		return nil, err
	}
	result := &model.EventModerationResponse{EventID: eventID, Status: model.EventStatusCancelled}
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		event, err := tx.Events.LockEvent(eventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrEventNotFound
		}
		if event.Status == model.EventStatusCancelled && event.WithdrawnByModerator {
			return nil
		}
		if event.Status != model.EventStatusDraft && event.Status != model.EventStatusPublished && event.Status != model.EventStatusCancelled {
			return ErrInvalidStatus
		}
		before := event.Status
		previouslyBlocked := event.WithdrawnByModerator
		event.WithdrawnByModerator = true
		event.Status = model.EventStatusCancelled
		if err := tx.Events.UpdateEvent(event); err != nil {
			return err
		}
		fields := map[string]any{"status": fieldChange(string(before), string(event.Status))}
		if !previouslyBlocked {
			fields["withdrawnByModerator"] = fieldChange(false, true)
		}
		changes := model.AuditChanges{"fields": fields, "reason": reason}
		if err := appendModeratedEventAudit(tx, meta, event, changes); err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func appendModeratedEventAudit(tx repository.Tx, meta audit.Meta, event *model.EventModel, changes model.AuditChanges) error {
	org := &model.OrganizationModel{KeycloakOrgID: audit.ScopePlatform}
	if event.OrganizerID != nil {
		owner, err := tx.Organizations.GetByID(*event.OrganizerID)
		if err != nil {
			return err
		}
		if owner != nil && owner.KeycloakOrgID != "" {
			org = owner
		}
	}
	return appendEventAudit(tx, meta, org, event.EventID, changes)
}

// SetRatingVisibility hides or shows a rating. Hidden ratings no longer count towards rating
// summaries and recommendations. Requesting the current visibility changes and records nothing.
func (s *ModerationService) SetRatingVisibility(ctx context.Context, ratingID uuid.UUID, hidden bool, reason string) (*model.RatingVisibilityResponse, error) {
	meta, err := audit.MetaFromContext(ctx, audit.RatingVisibilityChanged)
	if err != nil {
		return nil, err
	}
	visible := !hidden
	result := &model.RatingVisibilityResponse{RatingID: ratingID, Hidden: hidden}
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		target, err := tx.Moderation.LockRating(ratingID)
		if err != nil {
			return err
		}
		if target == nil {
			return ErrRatingNotFound
		}
		if target.IsVisible == visible {
			return nil
		}
		if err := tx.Moderation.SetRatingVisibility(ratingID, visible); err != nil {
			return err
		}
		result.Changed = true
		return appendRatingModerationAudit(tx, meta, target, visible, reason)
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// appendRatingModerationAudit records the entry under the organization of the rated event, so that
// the organization can trace moderation of its content in its audit log.
func appendRatingModerationAudit(tx repository.Tx, meta audit.Meta, target *model.RatingModerationTarget, visible bool, reason string) error {
	orgID := audit.ScopePlatform
	if target.KeycloakOrgID != nil && *target.KeycloakOrgID != "" {
		orgID = *target.KeycloakOrgID
	}
	err := tx.Audit.Append(&model.AuditLogModel{
		OperationID:    meta.OperationID,
		ActorSubject:   meta.ActorSubject,
		ActorUsername:  meta.ActorUsername,
		OrganizationID: orgID,
		Action:         string(meta.Action),
		ResourceType:   audit.ResourceRating,
		ResourceID:     target.RatingID.String(),
		Phase:          audit.PhaseCompleted,
		Changes: model.AuditChanges{
			"fields":  map[string]any{"isVisible": fieldChange(target.IsVisible, visible)},
			"reason":  reason,
			"eventId": auditUUID(target.EventID),
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", audit.ErrUnavailable, err)
	}
	return nil
}
