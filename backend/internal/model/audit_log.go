package model

import (
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AuditChanges holds the structured, business-relevant details of an audit entry.
type AuditChanges map[string]any

func (c AuditChanges) Value() (driver.Value, error) {
	if c == nil {
		return "{}", nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func (c *AuditChanges) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case nil:
		*c = AuditChanges{}
		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("unsupported audit changes type %T", src)
	}
	return json.Unmarshal(raw, c)
}

// AuditLogModel is an append-only entry of the organization audit log. There are deliberately no
// foreign keys: the history must survive renamed or deleted users, events and organizations.
type AuditLogModel struct {
	AuditLogID     uuid.UUID    `json:"id" gorm:"column:audit_log_id;type:uuid;primaryKey"`
	OperationID    uuid.UUID    `json:"operationId" gorm:"column:operation_id;type:uuid"`
	OccurredAt     time.Time    `json:"occurredAt" gorm:"column:occurred_at;->"`
	ActorSubject   string       `json:"actorSubject" gorm:"column:actor_subject"`
	ActorUsername  string       `json:"actorUsername" gorm:"column:actor_username"`
	OrganizationID string       `json:"organizationId" gorm:"column:organization_id"`
	Action         string       `json:"action" gorm:"column:action"`
	ResourceType   string       `json:"resourceType" gorm:"column:resource_type"`
	ResourceID     string       `json:"resourceId" gorm:"column:resource_id"`
	Phase          string       `json:"phase" gorm:"column:phase"`
	Changes        AuditChanges `json:"changes" gorm:"column:changes;type:jsonb"`
}

func (AuditLogModel) TableName() string { return "audit_logs" }

// AuditLogCursor is the keyset position after which the next page starts.
type AuditLogCursor struct {
	OccurredAt time.Time
	ID         uuid.UUID
}

var ErrInvalidAuditCursor = errors.New("invalid cursor")

func (c AuditLogCursor) Encode() string {
	raw := strconv.FormatInt(c.OccurredAt.UnixMicro(), 10) + "|" + c.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeAuditLogCursor(s string) (AuditLogCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return AuditLogCursor{}, ErrInvalidAuditCursor
	}
	micros, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return AuditLogCursor{}, ErrInvalidAuditCursor
	}
	m, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return AuditLogCursor{}, ErrInvalidAuditCursor
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		return AuditLogCursor{}, ErrInvalidAuditCursor
	}
	return AuditLogCursor{OccurredAt: time.UnixMicro(m).UTC(), ID: parsed}, nil
}

// AuditLogPage is the response of the audit log endpoint, newest entries first.
type AuditLogPage struct {
	Items      []AuditLogModel `json:"items"`
	NextCursor *string         `json:"nextCursor"`
}
