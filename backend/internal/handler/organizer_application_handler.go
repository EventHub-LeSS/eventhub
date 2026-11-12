package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrganizerApplicationHandler serves the contact form through which users apply to become an
// organizer and the review of those applications by global admins (EVENTHUB-294, EVENTHUB-304).
type OrganizerApplicationHandler struct {
	applications *service.OrganizerApplicationService
	userRepo     repository.UserRepository
}

func NewOrganizerApplicationHandler(applications *service.OrganizerApplicationService, userRepo repository.UserRepository) *OrganizerApplicationHandler {
	return &OrganizerApplicationHandler{applications: applications, userRepo: userRepo}
}

// organizationAliasPattern keeps aliases usable in tokens and URLs.
var organizationAliasPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}[a-z0-9]$`)

var countryCodePattern = regexp.MustCompile(`^[A-Z]{2}$`)

const maxDecisionReasonLength = 1000

// SubmitApplicationHandler godoc
// @Summary      Apply to become an organizer
// @Description  Contact form of the authenticated user to become an organizer. The application waits for review by a global admin; approval creates the organization with the applicant as org_admin. Only one application per user can be pending. The alias must be 3-50 lowercase letters, digits or hyphens and start and end with a letter or digit.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body model.SubmitOrganizerApplicationRequest true "Contact form"
// @Success      201 {object} model.OrganizerApplicationModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "An application is already pending, or the organization alias or name is taken"
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizer-applications [post]
func (h *OrganizerApplicationHandler) SubmitApplicationHandler(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}
	var req model.SubmitOrganizerApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "organizationName, organizationAlias, a valid contactEmail and message are required")
		return
	}
	if err := normalizeApplication(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}
	user, ok := resolveAPIUser(c, h.userRepo, principal)
	if !ok {
		return
	}
	application, err := h.applications.Submit(user.UserID, req)
	switch {
	case errors.Is(err, repository.ErrPendingApplicationExists):
		writeProblem(c, http.StatusConflict, "you already have a pending organizer application")
	case errors.Is(err, service.ErrOrganizationExists):
		writeProblem(c, http.StatusConflict, "an organization with this alias or name already exists")
	case err != nil:
		slog.ErrorContext(c.Request.Context(), "failed to submit organizer application", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to submit application")
	default:
		c.JSON(http.StatusCreated, application)
	}
}

// ListOwnApplicationsHandler godoc
// @Summary      List own organizer applications
// @Description  All organizer applications of the authenticated user with their review state, newest first.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Produce      json
// @Success      200 {array} model.OrganizerApplicationModel
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /me/organizer-applications [get]
func (h *OrganizerApplicationHandler) ListOwnApplicationsHandler(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		writeProblem(c, http.StatusUnauthorized, "authentication is required")
		return
	}
	user, ok := resolveAPIUser(c, h.userRepo, principal)
	if !ok {
		return
	}
	applications, err := h.applications.ListOwn(user.UserID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to list own organizer applications", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load applications")
		return
	}
	c.JSON(http.StatusOK, applications)
}

// ListApplicationsHandler godoc
// @Summary      List organizer applications for review
// @Description  Requires the global admin role. Oldest first, so reviewers work through the queue.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Produce      json
// @Param        status query string false "Review state" Enums(pending,approved,rejected)
// @Param        page   query int false "Page" default(1) minimum(1)
// @Param        limit  query int false "Page size" default(50) minimum(1) maximum(100)
// @Success      200 {object} model.OrganizerApplicationPage
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizer-applications [get]
func (h *OrganizerApplicationHandler) ListApplicationsHandler(c *gin.Context) {
	filter := model.OrganizerApplicationFilter{Status: model.OrganizerApplicationStatus(c.Query("status"))}
	if filter.Status != "" && !model.IsValidOrganizerApplicationStatus(filter.Status) {
		writeProblem(c, http.StatusBadRequest, "invalid application status")
		return
	}
	page, limit, err := parsePageLimit(c)
	if err == nil && page-1 > math.MaxInt/limit {
		err = errors.New("page is too large")
	}
	if err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.applications.List(filter, page, limit)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to list organizer applications", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load applications")
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetApplicationHandler godoc
// @Summary      Inspect an organizer application
// @Description  Requires the global admin role.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Produce      json
// @Param        applicationId path string true "Application UUID"
// @Success      200 {object} model.OrganizerApplicationModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizer-applications/{applicationId} [get]
func (h *OrganizerApplicationHandler) GetApplicationHandler(c *gin.Context) {
	applicationID, ok := parseApplicationID(c)
	if !ok {
		return
	}
	application, err := h.applications.Get(applicationID)
	writeApplicationResult(c, application, err)
}

// ApproveApplicationHandler godoc
// @Summary      Approve an organizer application
// @Description  Requires the global admin role; reviewers cannot approve their own application (403). Creates the requested organization with the backend service account in Keycloak and in the API database with the applicant as org_admin, marks the application approved, records the decision in the audit log of the new organization and notifies the applicant. An optional reason (at most 1000 characters) is passed on to the applicant. The applicant sees the organization after logging in again.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        applicationId path string true "Application UUID"
// @Param        request body model.ApproveOrganizerApplicationRequest false "Optional note"
// @Success      200 {object} model.OrganizerApplicationModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Already decided, or the organization alias or name is taken"
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizer-applications/{applicationId}/approve [post]
func (h *OrganizerApplicationHandler) ApproveApplicationHandler(c *gin.Context) {
	applicationID, ok := parseApplicationID(c)
	if !ok {
		return
	}
	var req model.ApproveOrganizerApplicationRequest
	// The body is optional: an approval without a note sends none.
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		writeProblem(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Reason != nil {
		reason := strings.TrimSpace(*req.Reason)
		if utf8.RuneCountInString(reason) > maxDecisionReasonLength {
			writeProblem(c, http.StatusBadRequest, fmt.Sprintf("reason must be at most %d characters", maxDecisionReasonLength))
			return
		}
		req.Reason = &reason
		if reason == "" {
			req.Reason = nil
		}
	}
	application, err := h.applications.Approve(c.Request.Context(), applicationID, req.Reason)
	switch {
	case errors.Is(err, service.ErrOrganizationExists):
		writeProblem(c, http.StatusConflict, "an organization with this alias or name already exists; reject the application or ask the applicant to apply again with another alias")
	case errors.Is(err, service.ErrApplicantNotFound), errors.Is(err, service.ErrOrgAdminNotSynced):
		writeProblem(c, http.StatusConflict, "the applicant is not synchronized between Keycloak and the API database")
	default:
		writeApplicationResult(c, application, err)
	}
}

// RejectApplicationHandler godoc
// @Summary      Reject an organizer application
// @Description  Requires the global admin role; reviewers cannot reject their own application (403). Requires a reason of 1-1000 characters after trimming, which the applicant is told. The decision is recorded in the platform audit log.
// @Tags         organizer-applications
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        applicationId path string true "Application UUID"
// @Param        request body model.RejectOrganizerApplicationRequest true "Reason"
// @Success      200 {object} model.OrganizerApplicationModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse "Already decided"
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizer-applications/{applicationId}/reject [post]
func (h *OrganizerApplicationHandler) RejectApplicationHandler(c *gin.Context) {
	applicationID, ok := parseApplicationID(c)
	if !ok {
		return
	}
	var req model.RejectOrganizerApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "reason is required")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxDecisionReasonLength {
		writeProblem(c, http.StatusBadRequest, fmt.Sprintf("reason must be 1-%d characters", maxDecisionReasonLength))
		return
	}
	application, err := h.applications.Reject(c.Request.Context(), applicationID, reason)
	writeApplicationResult(c, application, err)
}

func parseApplicationID(c *gin.Context) (uuid.UUID, bool) {
	applicationID, err := uuid.Parse(c.Param("applicationId"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid application id")
		return uuid.Nil, false
	}
	return applicationID, true
}

func writeApplicationResult(c *gin.Context, application *model.OrganizerApplicationModel, err error) {
	switch {
	case errors.Is(err, service.ErrApplicationNotFound):
		writeProblem(c, http.StatusNotFound, "application not found")
	case errors.Is(err, service.ErrApplicationNotPending):
		writeProblem(c, http.StatusConflict, "application has already been decided")
	case errors.Is(err, service.ErrOwnApplication):
		writeProblem(c, http.StatusForbidden, "you cannot decide on your own application")
	case errors.Is(err, audit.ErrMissingContext), errors.Is(err, audit.ErrUnavailable):
		slog.ErrorContext(c.Request.Context(), "organizer application decision not audited", "error", err)
		writeProblem(c, http.StatusInternalServerError, "audit log unavailable")
	case err != nil:
		slog.ErrorContext(c.Request.Context(), "organizer application request failed", "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to process application")
	default:
		c.JSON(http.StatusOK, application)
	}
}

// normalizeApplication trims the contact form and checks the lengths and formats the binding
// tags cannot express. Empty optional fields are stored as NULL.
func normalizeApplication(req *model.SubmitOrganizerApplicationRequest) error {
	req.OrganizationName = strings.TrimSpace(req.OrganizationName)
	req.OrganizationAlias = strings.ToLower(strings.TrimSpace(req.OrganizationAlias))
	req.ContactEmail = strings.TrimSpace(req.ContactEmail)
	req.Message = strings.TrimSpace(req.Message)

	if n := utf8.RuneCountInString(req.OrganizationName); n < 2 || n > 200 {
		return errors.New("organizationName must be 2-200 characters")
	}
	if !organizationAliasPattern.MatchString(req.OrganizationAlias) {
		return errors.New("organizationAlias must be 3-50 lowercase letters, digits or hyphens and start and end with a letter or digit")
	}
	if n := utf8.RuneCountInString(req.Message); n < 10 || n > 5000 {
		return errors.New("message must be 10-5000 characters")
	}
	optional := []struct {
		name  string
		value **string
		max   int
	}{
		{"contactPhoneNumber", &req.ContactPhoneNumber, 50},
		{"street", &req.Street, 200},
		{"houseNumber", &req.HouseNumber, 20},
		{"postalCode", &req.PostalCode, 20},
		{"city", &req.City, 100},
		{"countryCode", &req.CountryCode, 2},
	}
	for _, field := range optional {
		if *field.value == nil {
			continue
		}
		trimmed := strings.TrimSpace(**field.value)
		if trimmed == "" {
			*field.value = nil
			continue
		}
		if utf8.RuneCountInString(trimmed) > field.max {
			return fmt.Errorf("%s must be at most %d characters", field.name, field.max)
		}
		*field.value = &trimmed
	}
	if req.CountryCode != nil {
		upper := strings.ToUpper(*req.CountryCode)
		if !countryCodePattern.MatchString(upper) {
			return errors.New("countryCode must be a two-letter ISO 3166-1 code")
		}
		req.CountryCode = &upper
	}
	return nil
}
