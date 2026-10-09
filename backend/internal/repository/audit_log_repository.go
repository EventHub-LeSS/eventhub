package repository

import (
	"backend/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AuditLogRepository is append-only: entries are never updated or deleted by the application;
// retention is enforced by the database (see migration 000007).
type AuditLogRepository interface {
	Append(entry *model.AuditLogModel) error
	// ListByOrganization returns up to limit entries of the organization, newest first, starting
	// after cursor. The organization filter always applies, whatever the cursor says.
	ListByOrganization(keycloakOrgID string, limit int, cursor *model.AuditLogCursor) ([]model.AuditLogModel, error)
}

type auditLogRepository struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Append(entry *model.AuditLogModel) error {
	if entry.AuditLogID == uuid.Nil {
		entry.AuditLogID = uuid.New()
	}
	return r.db.Create(entry).Error
}

func (r *auditLogRepository) ListByOrganization(keycloakOrgID string, limit int, cursor *model.AuditLogCursor) ([]model.AuditLogModel, error) {
	query := r.db.Where("organization_id = ?", keycloakOrgID)
	if cursor != nil {
		query = query.Where("(occurred_at, audit_log_id) < (?, ?)", cursor.OccurredAt, cursor.ID)
	}
	entries := make([]model.AuditLogModel, 0, limit)
	err := query.Order("occurred_at DESC, audit_log_id DESC").Limit(limit).Find(&entries).Error
	return entries, err
}
