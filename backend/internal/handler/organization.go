package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/henning-kln/gocloak"
)

type OrganizationHandler struct {
	keycloakService *service.KeycloakService
}

func NewOrganizationHandler(keycloakService *service.KeycloakService) *OrganizationHandler {
	return &OrganizationHandler{
		keycloakService: keycloakService,
	}
}

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
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{"code": "UNAUTHENTICATED", "message": "Authentication is required"},
		})
		return
	}

	org := gocloak.OrganizationRepresentation{
		Name:  &req.InternalName,
		Alias: &req.DisplayName,
	}

	orgID, _, err := h.keycloakService.CreateOrganization(c.Request.Context(), principal.AccessToken, org, req.OrgAdmin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, model.CreateOrganizationResponse{
		ID:      orgID,
		Name:    req.InternalName,
		Alias:   req.DisplayName,
		Message: "Organization created successfully",
	})
}
