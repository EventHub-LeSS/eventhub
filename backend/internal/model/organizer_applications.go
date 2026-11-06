package model

import (
	"time"

	"github.com/google/uuid"
)

// OrganizerApplicationStatus is the review state of an organizer application (EVENTHUB-294).
type OrganizerApplicationStatus string

const (
	OrganizerApplicationPending  OrganizerApplicationStatus = "pending"
	OrganizerApplicationApproved OrganizerApplicationStatus = "approved"
	OrganizerApplicationRejected OrganizerApplicationStatus = "rejected"
)

func IsValidOrganizerApplicationStatus(status OrganizerApplicationStatus) bool {
	switch status {
	case OrganizerApplicationPending, OrganizerApplicationApproved, OrganizerApplicationRejected:
		return true
	}
	return false
}

// OrganizerApplicationModel is a user's request, submitted via the contact form, to become an
// organizer. Approval creates the requested organization with the applicant as org_admin.
type OrganizerApplicationModel struct {
	ApplicationID      uuid.UUID                  `json:"applicationId" gorm:"column:application_id;type:uuid;primaryKey"`
	UserID             uuid.UUID                  `json:"userId" gorm:"column:user_id;type:uuid;not null"`
	OrganizationName   string                     `json:"organizationName" gorm:"column:organization_name;not null"`
	OrganizationAlias  string                     `json:"organizationAlias" gorm:"column:organization_alias;not null"`
	ContactEmail       string                     `json:"contactEmail" gorm:"column:contact_email;not null"`
	ContactPhoneNumber *string                    `json:"contactPhoneNumber" gorm:"column:contact_phone_number"`
	Street             *string                    `json:"street" gorm:"column:street"`
	HouseNumber        *string                    `json:"houseNumber" gorm:"column:house_number"`
	PostalCode         *string                    `json:"postalCode" gorm:"column:postal_code"`
	City               *string                    `json:"city" gorm:"column:city"`
	CountryCode        *string                    `json:"countryCode" gorm:"column:country_code;type:varchar(2)"`
	Message            string                     `json:"message" gorm:"column:message;not null"`
	Status             OrganizerApplicationStatus `json:"status" gorm:"column:status;not null;default:pending"`
	DecisionReason     *string                    `json:"decisionReason" gorm:"column:decision_reason"`
	DecidedBySubject   *string                    `json:"-" gorm:"column:decided_by_subject"`
	DecidedByUsername  *string                    `json:"decidedBy" gorm:"column:decided_by_username"`
	DecidedAt          *time.Time                 `json:"decidedAt" gorm:"column:decided_at"`
	OrganizationID     *uuid.UUID                 `json:"organizationId" gorm:"column:organization_id;type:uuid"`
	CreatedAt          time.Time                  `json:"createdAt" gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt          time.Time                  `json:"updatedAt" gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (OrganizerApplicationModel) TableName() string { return "organizer_applications" }

// SubmitOrganizerApplicationRequest is the contact form a user submits to become an organizer.
// The applicant is always the authenticated user; the form names the organization to create.
type SubmitOrganizerApplicationRequest struct {
	// OrganizationName is the display name of the organization to create.
	OrganizationName string `json:"organizationName" binding:"required" minLength:"2" maxLength:"200" example:"Stadtfest Bremen e.V."`
	// OrganizationAlias identifies the organization in tokens and URLs: 3-50 lowercase letters,
	// digits and hyphens, starting and ending with a letter or digit.
	OrganizationAlias  string  `json:"organizationAlias" binding:"required" example:"stadtfest-bremen"`
	ContactEmail       string  `json:"contactEmail" binding:"required,email" example:"info@stadtfest-bremen.de"`
	ContactPhoneNumber *string `json:"contactPhoneNumber,omitempty" maxLength:"50"`
	Street             *string `json:"street,omitempty" maxLength:"200"`
	HouseNumber        *string `json:"houseNumber,omitempty" maxLength:"20"`
	PostalCode         *string `json:"postalCode,omitempty" maxLength:"20"`
	City               *string `json:"city,omitempty" maxLength:"100"`
	CountryCode        *string `json:"countryCode,omitempty" minLength:"2" maxLength:"2" example:"DE"`
	// Message tells the reviewers who the applicant is and which events they plan.
	Message string `json:"message" binding:"required" minLength:"10" maxLength:"5000"`
}

// RejectOrganizerApplicationRequest carries the reason the applicant is told.
type RejectOrganizerApplicationRequest struct {
	Reason string `json:"reason" binding:"required" minLength:"1" maxLength:"1000" example:"Bitte geben Sie ein Impressum an."`
}

// ApproveOrganizerApplicationRequest optionally carries a note for the applicant.
type ApproveOrganizerApplicationRequest struct {
	Reason *string `json:"reason,omitempty" maxLength:"1000"`
}

type OrganizerApplicationFilter struct {
	Status OrganizerApplicationStatus
}

type OrganizerApplicationPage struct {
	Items []OrganizerApplicationModel `json:"items"`
	Page  int                         `json:"page"`
	Limit int                         `json:"limit"`
	Total int64                       `json:"total"`
}
