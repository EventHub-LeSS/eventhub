package handler

import (
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
	"github.com/google/uuid"
	"github.com/henning-kln/gocloak"
)

type OrganizationHandler struct {
	keycloakService *service.KeycloakService
	orgRepo         repository.OrganizationRepository
	userRepo        repository.UserRepository
}

func NewOrganizationHandler(keycloakService *service.KeycloakService, orgRepo repository.OrganizationRepository, userRepo repository.UserRepository) *OrganizationHandler {
	return &OrganizationHandler{
		keycloakService: keycloakService,
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

	org := gocloak.OrganizationRepresentation{
		Name:  &req.InternalName,
		Alias: &req.DisplayName,
	}

	keycloakOrgID, adminKeycloakUserID, err := h.keycloakService.CreateOrganization(c.Request.Context(), principal.AccessToken, org, req.OrgAdmin)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrOrganizationExists) {
			status = http.StatusConflict
		}
		c.JSON(status, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(status),
			Status: status,
			Detail: err.Error(),
		})
		return
	}

	adminUser, err := h.userRepo.GetByKeycloakUserID(adminKeycloakUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "organization created in Keycloak but failed to look up admin user in database: " + err.Error(),
		})
		return
	}
	if adminUser == nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: "org admin user not found in API database, run sync first",
		})
		return
	}

	orgUUID := uuid.New()
	err = h.orgRepo.CreateOrganization(&model.OrganizationModel{
		OrganizationID:     orgUUID,
		KeycloakOrgID:      keycloakOrgID,
		Name:               req.InternalName,
		ContactEmail:       req.ContactEmail,
		ContactPhoneNumber: req.ContactPhoneNumber,
		Street:             req.Street,
		HouseNumber:        req.HouseNumber,
		PostalCode:         req.PostalCode,
		City:               req.City,
		CountryCode:        req.CountryCode,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "organization created in Keycloak but failed to insert into database: " + err.Error(),
		})
		return
	}

	err = h.orgRepo.AddMembership(&model.OrganizationMembershipModel{
		UserID:         adminUser.UserID,
		OrganizationID: orgUUID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "organization created in Keycloak and database but failed to add membership: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, model.CreateOrganizationResponse{
		ID:      keycloakOrgID,
		Name:    req.InternalName,
		Alias:   req.DisplayName,
		Message: "Organization created successfully",
	})
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
	case errors.Is(err, service.ErrActorNotOrgAdmin):
		middleware.AbortForbidden(c)
	case errors.Is(err, service.ErrUserNotFound),
		errors.Is(err, service.ErrOrganizationNotFound),
		errors.Is(err, service.ErrNotAMember):
		writeProblem(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrLastAdmin):
		writeProblem(c, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrOrgGroupsMissing):
		writeProblem(c, http.StatusInternalServerError, err.Error())
	default:
		writeProblem(c, http.StatusInternalServerError, "failed to configure organization roles, the request is safe to retry")
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
