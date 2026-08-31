package model

import (
	"time"

	"github.com/google/uuid"
)

type OrganizationMembershipModel struct {
	UserID         uuid.UUID `json:"userId" gorm:"column:user_id;type:uuid;primaryKey"`
	OrganizationID uuid.UUID `json:"organizationId" gorm:"column:organization_id;type:uuid;primaryKey"`
	JoinedAt       time.Time `json:"joinedAt" gorm:"column:joined_at;not null;default:CURRENT_TIMESTAMP;autoCreateTime"`
}

func (OrganizationMembershipModel) TableName() string { return "organization_memberships" }