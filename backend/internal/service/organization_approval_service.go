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

// ErrOrganizationAlreadyApproved means the organization is not waiting for approval anymore.
var ErrOrganizationAlreadyApproved = errors.New("organization is already approved")

// OrganizationApprovalService lets global admins approve organizations that are waiting for
// approval (EVENTHUB-293). Every approval is recorded in the audit log against the approving
// admin, in the same transaction as the change.
type OrganizationApprovalService struct {
	tx repository.Transactor
}

func NewOrganizationApprovalService(tx repository.Transactor) *OrganizationApprovalService {
	return &OrganizationApprovalService{tx: tx}
}

// ApproveOrganization activates a pending organization and returns it. It returns
// ErrOrganizationNotFound for an unknown organization and ErrOrganizationAlreadyApproved when the
// organization is active already; neither changes or records anything.
func (s *OrganizationApprovalService) ApproveOrganization(ctx context.Context, organizationID uuid.UUID) (*model.OrganizationModel, error) {
	meta, err := audit.MetaFromContext(ctx, audit.OrganizationApproved)
	if err != nil {
		return nil, err
	}
	var approved *model.OrganizationModel
	err = s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		// Activate changes only pending rows, so parallel approvals record a single entry.
		activated, err := tx.Organizations.Activate(organizationID)
		if err != nil {
			return err
		}
		org, err := tx.Organizations.GetByID(organizationID)
		if err != nil {
			return err
		}
		if org == nil {
			return ErrOrganizationNotFound
		}
		if !activated {
			return ErrOrganizationAlreadyApproved
		}
		if err := appendOrganizationApprovalAudit(tx, meta, org); err != nil {
			return err
		}
		approved = org
		return nil
	})
	if err != nil {
		return nil, err
	}
	return approved, nil
}

// appendOrganizationApprovalAudit records the entry under the approved organization, so that it
// can trace its approval in its own audit log.
func appendOrganizationApprovalAudit(tx repository.Tx, meta audit.Meta, org *model.OrganizationModel) error {
	scope := org.KeycloakOrgID
	if scope == "" {
		scope = audit.ScopePlatform
	}
	err := tx.Audit.Append(&model.AuditLogModel{
		OperationID:    meta.OperationID,
		ActorSubject:   meta.ActorSubject,
		ActorUsername:  meta.ActorUsername,
		OrganizationID: scope,
		Action:         string(meta.Action),
		ResourceType:   audit.ResourceOrganization,
		ResourceID:     org.OrganizationID.String(),
		Phase:          audit.PhaseCompleted,
		Changes: model.AuditChanges{
			"fields": map[string]any{"isActive": fieldChange(false, true)},
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %w", audit.ErrUnavailable, err)
	}
	return nil
}
