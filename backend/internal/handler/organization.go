package handler

import (
	"backend/internal/audit"
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type OrganizationHandler struct {
	keycloakService *service.KeycloakService
	orgService      *service.OrganizationService
	orgRepo         repository.OrganizationRepository
	userRepo        repository.UserRepository
}

func NewOrganizationHandler(keycloakService *service.KeycloakService, orgService *service.OrganizationService, orgRepo repository.OrganizationRepository, userRepo repository.UserRepository) *OrganizationHandler {
	return &OrganizationHandler{
		keycloakService: keycloakService,
		orgService:      orgService,
		orgRepo:         orgRepo,
		userRepo:        userRepo,
	}
}

// @Summary      Create organization
// @Description  Creates a new organization in Keycloak and the API database. Requires the admin role on the backend client.
// @Tags         organizations
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        request body model.CreateOrganizationRequest true "Organization to create"
// @Success      201 {object} model.CreateOrganizationResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/ [post]
func (h *OrganizationHandler) CreateOrganization(c *gin.Context) {
	var req model.CreateOrganizationRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}
	req.OrgAdmin = strings.TrimSpace(req.OrgAdmin)

	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}

	created, err := h.orgService.CreateOrganization(c.Request.Context(), principal.AccessToken, req)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, service.ErrOrganizationExists):
			status = http.StatusConflict
		case errors.Is(err, service.ErrOrgAdminNotSynced):
			status = http.StatusBadRequest
		}
		c.JSON(status, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(status),
			Status: status,
			Detail: err.Error(),
		})
		return
	}
	keycloakOrgID := created.KeycloakOrgID

	c.JSON(http.StatusCreated, model.CreateOrganizationResponse{
		ID:      keycloakOrgID,
		Name:    req.InternalName,
		Alias:   req.DisplayName,
		Message: "Organization created successfully",
	})
}

// resolveOrganizationParam resolves the organization the path addresses by ID or alias and maps
// the lookup failure the way organization endpoints do: a global admin sees 404, members see
// unknown and foreign organizations as the same 403.
func (h *OrganizationHandler) resolveOrganizationParam(c *gin.Context, principal *middleware.Principal) (service.OrganizationRef, bool) {
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)
	org, err := h.keycloakService.ResolveOrganization(c.Request.Context(), c.Param("organizationID"))
	if err != nil {
		if errors.Is(err, service.ErrOrganizationNotFound) {
			if globalAdmin {
				writeProblem(c, http.StatusNotFound, err.Error())
			} else {
				// Unknown and foreign organizations look the same to organization members.
				middleware.AbortForbidden(c)
			}
			return service.OrganizationRef{}, false
		}
		slog.Error("organization: organization lookup failed", "error", err.Error())
		writeProblem(c, http.StatusInternalServerError, "failed to look up the organization")
		return service.OrganizationRef{}, false
	}
	return org, true
}

// GetOrganization returns the details of an organization for the settings card.
// @Summary      Get organization details
// @Description  Returns the details of an organization: display name, alias, contact columns and timestamps. Requires membership of the organization in any role (global admins bypass this check). organizationID is the Keycloak organization ID or alias as returned by POST /organizations and GET /users/me. The display name is loaded from the API database; the alias is the identity reference the token claim carries.
// @Tags         organizations
// @Security     BearerAuth
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Success      200 {object} model.OrganizationDetailsResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID} [get]
func (h *OrganizationHandler) GetOrganization(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	org, ok := h.resolveOrganizationParam(c, principal)
	if !ok {
		return
	}
	if !globalAdmin && !principal.IsOrganizationMember(org.ID, org.Alias) {
		middleware.AbortForbidden(c)
		return
	}

	row, err := h.orgRepo.GetByKeycloakOrgID(org.ID)
	if err != nil {
		slog.Error("organization: failed to load details", "organization_id", org.ID, "error", err.Error())
		writeProblem(c, http.StatusInternalServerError, "failed to load the organization details")
		return
	}
	if row == nil {
		// The organization exists in Keycloak, which roles and the settings card address it by;
		// its missing database row is an inconsistency, not an unknown organization.
		writeProblem(c, http.StatusInternalServerError, "the organization is not synchronized in the API database")
		return
	}

	c.JSON(http.StatusOK, model.OrganizationDetailsResponse{
		ID:                 org.ID,
		OrganizationID:     row.OrganizationID.String(),
		Alias:              row.Alias,
		Name:               row.Name,
		ContactEmail:       row.ContactEmail,
		ContactPhoneNumber: row.ContactPhoneNumber,
		Street:             row.Street,
		HouseNumber:        row.HouseNumber,
		PostalCode:         row.PostalCode,
		City:               row.City,
		CountryCode:        row.CountryCode,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	})
}

// ListOrganizationMembers returns the members and organization roles for an organization.
// @Summary      List organization members
// @Description  Returns members and their organization roles. Requires the org_admin role in the organization (global admins bypass this check).
// @Tags         organizations
// @Security     BearerAuth
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Success      200 {array} model.OrganizationMemberResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/members [get]
func (h *OrganizationHandler) ListOrganizationMembers(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	org, ok := h.resolveOrganizationParam(c, principal)
	if !ok {
		return
	}
	if !globalAdmin &&
		!principal.HasOrganizationRoleIn(org.ID, middleware.RoleOrganizationAdmin) &&
		!principal.HasOrganizationRoleIn(org.Alias, middleware.RoleOrganizationAdmin) {
		middleware.AbortForbidden(c)
		return
	}

	members, err := h.keycloakService.ListOrganizationMembers(c.Request.Context(), org.ID, principal.Subject, globalAdmin)
	if err != nil {
		writeOrgRolesError(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// InviteOrganizationMember sends an organization invitation through Keycloak.
// @Summary      Invite an organization member
// @Description  Sends an invitation to join the organization. Requires the org_admin role in the organization (global admins bypass this check).
// @Tags         organizations
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Param        request body model.InviteOrganizationMemberRequest true "Member invitation"
// @Success      202 {object} model.InviteOrganizationMemberResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/members/invitations [post]
func (h *OrganizationHandler) InviteOrganizationMember(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	org, ok := h.resolveOrganizationParam(c, principal)
	if !ok {
		return
	}
	if !globalAdmin &&
		!principal.HasOrganizationRoleIn(org.ID, middleware.RoleOrganizationAdmin) &&
		!principal.HasOrganizationRoleIn(org.Alias, middleware.RoleOrganizationAdmin) {
		middleware.AbortForbidden(c)
		return
	}

	var req model.InviteOrganizationMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "email must be a valid email address")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if err := h.keycloakService.InviteOrganizationMember(c.Request.Context(), org.ID, principal.Subject, req.Email, req.Name, globalAdmin); err != nil {
		writeOrganizationInvitationError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, model.InviteOrganizationMemberResponse{Message: "invitation sent"})
}

// UpdateOrganization sets the display name of an organization. Alias and contact columns are
// deliberately not changeable (EVENTHUB-188).
// @Summary      Update organization details
// @Description  Sets the display name of an organization. Requires the org_admin role in the given organization (global admins bypass this check); it is checked against the token and again against Keycloak, so a demoted admin cannot act on an old token. The change runs Keycloak first, then the API database row, and both are recorded in the audit log: a started entry before the first change and a result entry afterwards sharing one operationId; a result entry that could not be written is reported with 500 and the request is safe to retry. name is required; the alias is read-only, because the token claim carries it — sending it with a different value is rejected with 400. organizationID is the Keycloak organization ID or alias.
// @Tags         organizations
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Param        request body model.UpdateOrganizationRequest true "Organization details to set"
// @Success      200 {object} model.UpdateOrganizationResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID} [put]
func (h *OrganizationHandler) UpdateOrganization(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	org, ok := h.resolveOrganizationParam(c, principal)
	if !ok {
		return
	}
	if !globalAdmin &&
		!principal.HasOrganizationRoleIn(org.ID, middleware.RoleOrganizationAdmin) &&
		!principal.HasOrganizationRoleIn(org.Alias, middleware.RoleOrganizationAdmin) {
		middleware.AbortForbidden(c)
		return
	}

	var req model.UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "request body must be a JSON object with a name")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeProblem(c, http.StatusBadRequest, "name must not be empty")
		return
	}
	if req.Alias != nil && strings.TrimSpace(*req.Alias) != "" && strings.TrimSpace(*req.Alias) != org.Alias {
		writeProblem(c, http.StatusBadRequest, "alias is read-only: the token claim carries it, so every member would have to refresh their token")
		return
	}

	change, err := h.orgService.UpdateOrganizationName(c.Request.Context(), org.ID, name, service.OrgRoleActor{
		UserID:      principal.Subject,
		GlobalAdmin: globalAdmin,
	})
	if err != nil {
		logOrgNameChangeFailure(c, org.ID, name, err)
		writeOrgDetailsError(c, err)
		return
	}

	logOrgNameChange(c, change)

	c.JSON(http.StatusOK, model.UpdateOrganizationResponse{
		ID:      change.OrganizationID,
		Alias:   change.Alias,
		Name:    change.Name,
		Message: "organization updated successfully",
	})
}

// writeOrgDetailsError maps service errors of the organization details change to RFC 9457
// problem+json responses (model.ErrorResponse via writeProblem). 401/403 keep the model.APIError
// shape used by the auth middleware across the API.
func writeOrgDetailsError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, audit.ErrResultNotRecorded):
		writeProblem(c, http.StatusInternalServerError, "the organization name may have been updated but its result could not be recorded in the audit log; check the current details before retrying")
	case errors.Is(err, audit.ErrUnavailable), errors.Is(err, audit.ErrMissingContext):
		writeProblem(c, http.StatusInternalServerError, "the audit log is unavailable, the organization details were not changed")
	case errors.Is(err, service.ErrActorNotOrgAdmin):
		middleware.AbortForbidden(c)
	case errors.Is(err, service.ErrOrganizationNotFound):
		// The organization was resolved when the request started but no longer exists.
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrOrganizationNotInDatabase):
		writeProblem(c, http.StatusInternalServerError, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "failed to update the organization details, the request is safe to retry")
	}
}

// logOrgNameChange records who changed the details of which organization, for the audit trail.
func logOrgNameChange(c *gin.Context, change *service.OrgNameChange) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		return
	}
	slog.Info("organization details changed",
		"event", "org_details_changed",
		"actor_username", principal.Username,
		"actor_subject", principal.Subject,
		"organization_id", change.OrganizationID,
		"name", change.Name,
	)
}

// logOrgNameChangeFailure records a failed details change, so failed requests stay visible.
func logOrgNameChangeFailure(c *gin.Context, organizationID, name string, err error) {
	principal, _ := middleware.PrincipalFromContext(c)
	actor := ""
	if principal != nil {
		actor = principal.Username
	}
	slog.Error("organization details change failed",
		"event", "org_details_change_failed",
		"actor_username", actor,
		"organization_id", organizationID,
		"requested_name", name,
		"error", err.Error(),
	)
}

// EVENTHUB-188: Rollen innerhalb einer Organisation vergeben und entziehen
// @Summary      Configure organization member roles
// @Description  Replaces the complete role set of an organization member. Requires the org_admin role in the given organization (global admins bypass this check); it is checked against the token and again against Keycloak, so a demoted admin cannot act on an old token. organizationID is the Keycloak organization ID or alias as returned by POST /organizations and GET /users/me. roles is required; an empty array removes all roles. Roles: org_admin (manage members), event_manager (manage events), finance_viewer (view sales and billing). Removing the last org_admin is rejected. The affected user must refresh their token before the new roles take effect.
// @Tags         organizations
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Param        username path string true "Username of the organization member"
// @Param        request body model.ConfigureOrgRolesRequest true "Complete role set for the member (empty array = no roles)"
// @Success      200 {object} model.ConfigureOrgRolesResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/members/{username}/roles [put]
func (h *OrganizationHandler) ConfigureMemberRoles(c *gin.Context) {
	organizationParam := c.Param("organizationID")
	username := strings.TrimSpace(c.Param("username"))

	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	// Resolve first, then authorize against exactly this organization: the path may carry the
	// organization ID or alias, while tokens identify organizations by alias only.
	org, err := h.keycloakService.ResolveOrganization(c.Request.Context(), organizationParam)
	if err != nil {
		if errors.Is(err, service.ErrOrganizationNotFound) && !globalAdmin {
			// Unknown and foreign organizations look the same to organization admins.
			middleware.AbortForbidden(c)
			return
		}
		logOrgRoleChangeFailure(c, username, organizationParam, nil, err)
		writeOrgRolesError(c, err)
		return
	}
	if !globalAdmin &&
		!principal.HasOrganizationRoleIn(org.ID, middleware.RoleOrganizationAdmin) &&
		!principal.HasOrganizationRoleIn(org.Alias, middleware.RoleOrganizationAdmin) {
		middleware.AbortForbidden(c)
		return
	}

	var req model.ConfigureOrgRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, orgRolesBindingMessage(err))
		return
	}
	actor := service.OrgRoleActor{UserID: principal.Subject, GlobalAdmin: globalAdmin}
	change, err := h.keycloakService.ConfigureOrganizationMemberRoles(c.Request.Context(), org.ID, username, req.Roles, actor)
	if err != nil {
		logOrgRoleChangeFailure(c, username, org.ID, change, err)
		writeOrgRolesError(c, err)
		return
	}

	logOrgRoleChange(c, username, change)

	c.JSON(http.StatusOK, model.ConfigureOrgRolesResponse{
		Username:       username,
		OrganizationID: organizationParam,
		Roles:          change.Applied,
		Message:        "roles updated successfully",
	})
}

// orgRolesBindingMessage turns binding errors of ConfigureOrgRolesRequest into
// problem details: a missing roles field, unknown and duplicate roles are reported
// explicitly, any other error means a malformed body and gets a sanitized message.
// model.OrganizationRoles is the single source of truth for valid roles.
func orgRolesBindingMessage(err error) string {
	var valErrs validator.ValidationErrors
	if !errors.As(err, &valErrs) {
		return "request body must be a JSON object with a roles array"
	}
	details := make([]string, 0, len(valErrs))
	for _, fieldErr := range valErrs {
		switch fieldErr.Tag() {
		case "required":
			details = append(details, "roles is required; send an empty array to remove all roles")
		case "org_role":
			details = append(details, fmt.Sprintf("unknown role %q; allowed roles: %s", fieldErr.Value(), strings.Join(model.OrganizationRoleNames(), ", ")))
		case "unique":
			details = append(details, "duplicate role: roles must be unique")
		default:
			details = append(details, fieldErr.Error())
		}
	}
	return strings.Join(details, "; ")
}

// writeOrgRolesError maps service errors to RFC 9457 problem+json responses
// (model.ErrorResponse via writeProblem). 401/403 keep the model.APIError shape
// used by the auth middleware across the API.
func writeOrgRolesError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, audit.ErrResultNotRecorded):
		writeProblem(c, http.StatusInternalServerError, "the role change may have been applied but its result could not be recorded in the audit log; check the current roles before retrying")
	case errors.Is(err, audit.ErrUnavailable), errors.Is(err, audit.ErrMissingContext):
		writeProblem(c, http.StatusInternalServerError, "the audit log is unavailable, the role change was not performed")
	case errors.Is(err, service.ErrActorNotOrgAdmin):
		middleware.AbortForbidden(c)
	case errors.Is(err, service.ErrUserNotFound),
		errors.Is(err, service.ErrOrganizationNotFound),
		errors.Is(err, service.ErrNotAMember):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrLastAdmin):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrOrganizationInvitationExists):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrOrgGroupsMissing):
		writeProblem(c, http.StatusInternalServerError, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "failed to configure organization roles, the request is safe to retry")
	}
}

func writeOrganizationInvitationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrActorNotOrgAdmin):
		middleware.AbortForbidden(c)
	case errors.Is(err, service.ErrOrganizationNotFound):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrOrganizationInvitationExists):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrOrgGroupsMissing):
		writeProblem(c, http.StatusInternalServerError, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "failed to send organization invitation")
	}
}

// logOrgRoleChange records who changed whose roles where, for the audit trail (EVENTHUB-188).
func logOrgRoleChange(c *gin.Context, targetUsername string, change *service.OrgRoleChange) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		return
	}
	slog.Info("organization member roles changed",
		"event", "org_member_roles_changed",
		"actor_username", principal.Username,
		"actor_subject", principal.Subject,
		"organization_id", change.OrganizationID,
		"target_username", targetUsername,
		"granted", strings.Join(change.Granted, ","),
		"revoked", strings.Join(change.Revoked, ","),
	)
}

// logOrgRoleChangeFailure records a failed role change. change holds what was already applied
// before the failure, so partial changes stay in the audit trail.
func logOrgRoleChangeFailure(c *gin.Context, targetUsername, organizationID string, change *service.OrgRoleChange, err error) {
	principal, _ := middleware.PrincipalFromContext(c)
	actor := ""
	if principal != nil {
		actor = principal.Username
	}
	var granted, revoked []string
	if change != nil {
		granted, revoked = change.Granted, change.Revoked
	}
	slog.Error("organization member roles change failed",
		"event", "org_member_roles_change_failed",
		"actor_username", actor,
		"organization_id", organizationID,
		"target_username", targetUsername,
		"granted", strings.Join(granted, ","),
		"revoked", strings.Join(revoked, ","),
		"error", err.Error(),
	)
}
