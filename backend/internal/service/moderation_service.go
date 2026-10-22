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
	tx repository.Transactor
}

func NewModerationService(tx repository.Transactor) *ModerationService {
	return &ModerationService{tx: tx}
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
