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
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
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
// @Description  Replaces the complete role set of an organization member. Requires the org_admin role in the given organization (global admins bypass this check). organizationID is the Keycloak organization ID or alias as returned by POST /organizations and GET /users/me. Roles: org_admin (manage members), event_manager (manage events), finance_viewer (view sales and billing). Removing the last org_admin is rejected. The affected user must refresh their token before the new roles take effect.
// @Tags         organizations
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        organizationID path string true "Keycloak organization ID or Alias"
// @Param        username path string true "Username of the organization member"
// @Param        request body model.ConfigureOrgRolesRequest true "Complete role set for the member (empty = no roles)"
// @Success      200 {object} model.ConfigureOrgRolesResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/members/{username}/roles [put]
func (h *OrganizationHandler) ConfigureMemberRoles(c *gin.Context) {
	keycloakOrgID := c.Param("organizationID")
	username := strings.TrimSpace(c.Param("username"))

	var req model.ConfigureOrgRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeProblem(c, http.StatusBadRequest, "request body must be a JSON object with a roles array")
		return
	}
	if detail := validateOrgRoles(req.Roles); detail != "" {
		writeProblem(c, http.StatusBadRequest, detail)
		return
	}

	change, err := h.keycloakService.ConfigureOrganizationMemberRoles(c.Request.Context(), keycloakOrgID, username, req.Roles)
	if err != nil {
		logOrgRoleChangeFailure(c, username, keycloakOrgID, err)
		writeOrgRolesError(c, err)
		return
	}

	logOrgRoleChange(c, username, change)

	c.JSON(http.StatusOK, model.ConfigureOrgRolesResponse{
		Username:       username,
		OrganizationID: keycloakOrgID,
		Roles:          change.Applied,
		Message:        "roles updated successfully",
	})
}

// validateOrgRoles reports why a requested role set is invalid, or "" if it is valid.
// middleware.OrganizationRoles is the single source of truth for valid roles.
func validateOrgRoles(roles []string) string {
	seen := make(map[string]bool, len(roles))
	for _, role := range roles {
		if !middleware.IsValidOrganizationRole(role) {
			return fmt.Sprintf("unknown role %q; allowed roles: %s", role, strings.Join(middleware.OrganizationRoleNames(), ", "))
		}
		if seen[role] {
			return fmt.Sprintf("duplicate role %q; roles must be unique", role)
		}
		seen[role] = true
	}
	return ""
}

// writeOrgRolesError maps service errors to RFC 9457 problem+json responses
// (model.ErrorResponse via writeProblem). 401/403 come from the auth middleware
// and keep the model.APIError shape used across the API.
func writeOrgRolesError(c *gin.Context, err error) {
	switch {
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

func logOrgRoleChangeFailure(c *gin.Context, targetUsername, organizationID string, err error) {
	principal, _ := middleware.PrincipalFromContext(c)
	actor := ""
	if principal != nil {
		actor = principal.Username
	}
	slog.Error("organization member roles change failed",
		"event", "org_member_roles_change_failed",
		"actor_username", actor,
		"organization_id", organizationID,
		"target_username", targetUsername,
		"error", err.Error(),
	)
}
