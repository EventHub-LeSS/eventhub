package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"net/http"

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

	org := gocloak.OrganizationRepresentation{
		Name:  &req.InternalName,
		Alias: &req.DisplayName,
	}

	orgID, err := h.keycloakService.CreateOrganization(org)
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
