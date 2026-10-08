// Package audit carries the identity of the acting organization member from the HTTP layer to the
// business logic, so that every business-relevant change is recorded against a personal account.
//
// Adding an audited action: define an Action here, register middleware.Audit(action) on the route
// and let the service persist a model.AuditLogModel in the same transaction as the change (see
// EventService); changes in external systems use the start/result protocol (see KeycloakService).
package audit

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// Action names an audited business operation.
type Action string

const (
	EventCreated                  Action = "event.created"
	EventUpdated                  Action = "event.updated"
	EventPublished                Action = "event.published"
	EventCancelled                Action = "event.cancelled"
	OrganizationMemberRolesChange Action = "organization.member_roles_changed"
)

// Resource types of audited objects.
const (
	ResourceEvent              = "event"
	ResourceOrganizationMember = "organization_member"
)

// Phases describe how far an operation got. Local changes are recorded once as PhaseCompleted,
// together with the change. Operations in external systems cannot be rolled back with the local
// transaction: they are recorded as PhaseStarted before the first change and conclude with
// PhaseSucceeded or PhaseIncomplete under the same operation ID. A PhaseStarted entry without a
// conclusion means the outcome is unconfirmed.
const (
	PhaseCompleted  = "completed"
	PhaseStarted    = "started"
	PhaseSucceeded  = "succeeded"
	PhaseIncomplete = "incomplete"
)

var (
	// ErrMissingContext means an audited operation ran without the audit middleware.
	ErrMissingContext = errors.New("audit context is missing")
	// ErrUnavailable means the audit entry could not be persisted, so the operation did not run.
	ErrUnavailable = errors.New("audit log unavailable")
	// ErrResultNotRecorded means the operation ran but its result could not be persisted.
	ErrResultNotRecorded = errors.New("operation result could not be recorded in the audit log")
)

// Meta identifies who performs which audited operation. It is built from the verified access
// token only, never from request data.
type Meta struct {
	OperationID   uuid.UUID
	Action        Action
	ActorSubject  string
	ActorUsername string
}

type metaKey struct{}

// WithMeta attaches the audit metadata to the context.
func WithMeta(ctx context.Context, meta Meta) context.Context {
	return context.WithValue(ctx, metaKey{}, meta)
}

// MetaFromContext returns the metadata of the audited operation, which must be the given action.
func MetaFromContext(ctx context.Context, action Action) (Meta, error) {
	meta, ok := ctx.Value(metaKey{}).(Meta)
	if !ok || meta.ActorSubject == "" || meta.OperationID == uuid.Nil || meta.Action != action {
		return Meta{}, ErrMissingContext
	}
	return meta, nil
}
