package service

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
)

var (
	ErrApplicationNotFound   = errors.New("organizer application not found")
	ErrApplicationNotPending = errors.New("organizer application has already been decided")
	// ErrApplicantNotFound means the applicant's API user no longer exists.
	ErrApplicantNotFound = errors.New("applicant not found in API database")
	// ErrOwnApplication means the reviewer tried to decide on their own application.
	ErrOwnApplication = errors.New("reviewers cannot decide on their own organizer application")
)

// NotificationOccasionOrganizerApplication marks notifications about the decision on an
// organizer application.
const NotificationOccasionOrganizerApplication = "organizer_application"

// OrganizerApplicationService handles the contact form through which users apply to become an
// organizer (EVENTHUB-294). Global admins and org admins of any organization review the
// applications of others, never their own: approving it creates the
// requested organization with the applicant as org_admin, rejecting it records the reason. The
// applicant is notified of either decision.
type OrganizerApplicationService struct {
	tx           repository.Transactor
	applications repository.OrganizerApplicationRepository
	users        repository.UserRepository
	orgRepo      repository.OrganizationRepository
	orgService   *OrganizationService
}

func NewOrganizerApplicationService(tx repository.Transactor, applications repository.OrganizerApplicationRepository, users repository.UserRepository, orgRepo repository.OrganizationRepository, orgService *OrganizationService) *OrganizerApplicationService {
	return &OrganizerApplicationService{tx: tx, applications: applications, users: users, orgRepo: orgRepo, orgService: orgService}
}

// Submit stores a new pending application of the user. It fails with
// repository.ErrPendingApplicationExists while another application of the user awaits review, and
// with ErrOrganizationExists when the requested alias or name already belongs to an organization.
func (s *OrganizerApplicationService) Submit(userID uuid.UUID, req model.SubmitOrganizerApplicationRequest) (*model.OrganizerApplicationModel, error) {
	existing, err := s.orgRepo.GetByAnyRef("", req.OrganizationAlias, req.OrganizationName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrOrganizationExists
	}
	application := &model.OrganizerApplicationModel{
		ApplicationID:      uuid.New(),
		UserID:             userID,
		OrganizationName:   req.OrganizationName,
		OrganizationAlias:  req.OrganizationAlias,
		ContactEmail:       req.ContactEmail,
		ContactPhoneNumber: req.ContactPhoneNumber,
		Street:             req.Street,
		HouseNumber:        req.HouseNumber,
		PostalCode:         req.PostalCode,
		City:               req.City,
		CountryCode:        req.CountryCode,
		Message:            req.Message,
		Status:             model.OrganizerApplicationPending,
	}
	if err := s.applications.Create(application); err != nil {
		return nil, err
	}
	return application, nil
}

func (s *OrganizerApplicationService) ListOwn(userID uuid.UUID) ([]model.OrganizerApplicationModel, error) {
	return s.applications.ListForUser(userID)
}

func (s *OrganizerApplicationService) List(filter model.OrganizerApplicationFilter, page, limit int) (*model.OrganizerApplicationPage, error) {
	items, total, err := s.applications.List(filter, page, limit)
	if err != nil {
		return nil, err
	}
	return &model.OrganizerApplicationPage{Items: items, Page: page, Limit: limit, Total: total}, nil
}

func (s *OrganizerApplicationService) Get(applicationID uuid.UUID) (*model.OrganizerApplicationModel, error) {
	application, err := s.applications.GetByID(applicationID)
	if err != nil {
		return nil, err
	}
	if application == nil {
		return nil, ErrApplicationNotFound
	}
	return application, nil
}

// applicationMutexes serializes decisions per application, so two admins approving at once
// cannot both create an organization. Like orgMutexes it relies on the API running as a single
// instance; the row lock in the decision transaction still keeps the stored decision unique.
var applicationMutexes sync.Map // map[uuid.UUID]*sync.Mutex

func lockApplication(applicationID uuid.UUID) func() {
	value, _ := applicationMutexes.LoadOrStore(applicationID, &sync.Mutex{})
	//nolint:errcheck // applicationMutexes only ever stores *sync.Mutex.
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Approve creates the requested organization in Keycloak and the API database with the applicant
// as org_admin, then marks the application approved, records the decision in the audit log of the
// new organization and notifies the applicant. Reviewers need not be allowed to manage Keycloak
// organizations themselves: the organization is created with the backend service account. The
// applicant's next token carries the organization.
func (s *OrganizerApplicationService) Approve(ctx context.Context, applicationID uuid.UUID, reason *string) (*model.OrganizerApplicationModel, error) {
	meta, err := audit.MetaFromContext(ctx, audit.OrganizerApplicationApproved)
	if err != nil {
		return nil, err
	}
	unlock := lockApplication(applicationID)
	defer unlock()

	application, applicant, err := s.reviewable(applicationID, meta)
	if err != nil {
		return nil, err
	}

	contactEmail := application.ContactEmail
	created, err := s.orgService.CreateOrganizationAsService(ctx, model.CreateOrganizationRequest{
		InternalName:       application.OrganizationName,
		DisplayName:        application.OrganizationAlias,
		OrgAdmin:           applicant.Email,
		ContactEmail:       &contactEmail,
		ContactPhoneNumber: application.ContactPhoneNumber,
		Street:             application.Street,
		HouseNumber:        application.HouseNumber,
		PostalCode:         application.PostalCode,
		City:               application.City,
		CountryCode:        application.CountryCode,
	})
	if err != nil {
		return nil, err
	}

	decided, err := s.decide(ctx, meta, applicationID, created.KeycloakOrgID, func(a *model.OrganizerApplicationModel) {
		a.Status = model.OrganizerApplicationApproved
		a.DecisionReason = reason
		a.OrganizationID = &created.OrganizationID
	}, func(a *model.OrganizerApplicationModel) *model.NotificationData {
		content := fmt.Sprintf("Ihre Bewerbung als Veranstalter wurde angenommen. Die Organisation %q wurde angelegt und Sie sind ihr Administrator. Bitte melden Sie sich erneut an, um Veranstaltungen anzulegen.", a.OrganizationName)
		if reason != nil && *reason != "" {
			content += " Hinweis: " + *reason
		}
		return &model.NotificationData{Subject: "Bewerbung als Veranstalter angenommen", Content: content}
	})
	if err != nil {
		// The organization exists, but the application still reads as pending. An admin has to
		// mark it by hand; a repeated approval would fail because the alias is now taken.
		slog.ErrorContext(ctx, "organization created for organizer application but the decision could not be stored",
			"application_id", applicationID, "keycloak_org_id", created.KeycloakOrgID, "error", err)
		return nil, err
	}
	return decided, nil
}

// Reject marks the application rejected with the reason, records the decision in the platform
// audit log and notifies the applicant.
func (s *OrganizerApplicationService) Reject(ctx context.Context, applicationID uuid.UUID, reason string) (*model.OrganizerApplicationModel, error) {
	meta, err := audit.MetaFromContext(ctx, audit.OrganizerApplicationRejected)
	if err != nil {
		return nil, err
	}
	unlock := lockApplication(applicationID)
	defer unlock()

	if _, _, err := s.reviewable(applicationID, meta); err != nil {
		return nil, err
	}
	return s.decide(ctx, meta, applicationID, audit.ScopePlatform, func(a *model.OrganizerApplicationModel) {
		a.Status = model.OrganizerApplicationRejected
		a.DecisionReason = &reason
	}, func(a *model.OrganizerApplicationModel) *model.NotificationData {
		return &model.NotificationData{
			Subject: "Bewerbung als Veranstalter abgelehnt",
			Content: fmt.Sprintf("Ihre Bewerbung als Veranstalter für %q wurde abgelehnt. Begründung: %s", a.OrganizationName, reason),
		}
	})
}

// reviewable loads a pending application and its applicant and refuses decisions of the applicant
// on their own application.
func (s *OrganizerApplicationService) reviewable(applicationID uuid.UUID, meta audit.Meta) (*model.OrganizerApplicationModel, *model.UserModel, error) {
	application, err := s.Get(applicationID)
	if err != nil {
		return nil, nil, err
	}
	if application.Status != model.OrganizerApplicationPending {
		return nil, nil, ErrApplicationNotPending
	}
	applicant, err := s.users.GetByID(application.UserID)
	if err != nil {
		return nil, nil, err
	}
	if applicant == nil {
		return nil, nil, ErrApplicantNotFound
	}
	if applicant.KeycloakUserID == meta.ActorSubject {
		return nil, nil, ErrOwnApplication
	}
	return application, applicant, nil
}

// decide stores the decision on a pending application together with its audit entry and the
// notification of the applicant, all in one transaction.
func (s *OrganizerApplicationService) decide(
	ctx context.Context,
	meta audit.Meta,
	applicationID uuid.UUID,
	auditOrgID string,
	apply func(*model.OrganizerApplicationModel),
	notification func(*model.OrganizerApplicationModel) *model.NotificationData,
) (*model.OrganizerApplicationModel, error) {
	var result *model.OrganizerApplicationModel
	err := s.tx.InTransaction(ctx, func(tx repository.Tx) error {
		application, err := tx.Applications.Lock(applicationID)
		if err != nil {
			return err
		}
		if application == nil {
			return ErrApplicationNotFound
		}
		if application.Status != model.OrganizerApplicationPending {
			return ErrApplicationNotPending
		}
		now, err := tx.Notifications.DatabaseNow()
		if err != nil {
			return err
		}
		apply(application)
		application.DecidedAt = &now
		application.DecidedBySubject = &meta.ActorSubject
		application.DecidedByUsername = &meta.ActorUsername
		if err := tx.Applications.Update(application); err != nil {
			return err
		}

		err = tx.Audit.Append(&model.AuditLogModel{
			OperationID:    meta.OperationID,
			ActorSubject:   meta.ActorSubject,
			ActorUsername:  meta.ActorUsername,
			OrganizationID: auditOrgID,
			Action:         string(meta.Action),
			ResourceType:   audit.ResourceOrganizerApplication,
			ResourceID:     application.ApplicationID.String(),
			Phase:          audit.PhaseCompleted,
			Changes: model.AuditChanges{
				"fields":            map[string]any{"status": fieldChange(string(model.OrganizerApplicationPending), string(application.Status))},
				"reason":            application.DecisionReason,
				"applicantUserId":   application.UserID.String(),
				"organizationAlias": application.OrganizationAlias,
				"organizationId":    auditUUID(application.OrganizationID),
			},
		})
		if err != nil {
			return fmt.Errorf("%w: %w", audit.ErrUnavailable, err)
		}

		note := notification(application)
		note.NotificationID = uuid.New()
		note.UserID = &application.UserID
		note.Occasion = NotificationOccasionOrganizerApplication
		note.CreatedAt = now
		note.UpdatedAt = now
		if err := tx.Notifications.Create(note); err != nil {
			return err
		}
		result = application
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
