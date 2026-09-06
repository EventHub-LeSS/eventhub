package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
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
		OrganizationID: orgUUID,
		KeycloakOrgID:  keycloakOrgID,
		Name:           req.InternalName,
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
