package handler

import (
	"backend/internal/model"
	"backend/internal/repository"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB-293: admins list the organizations requested via the contact form and approve them.
// An organization only becomes public once an admin has approved it (EVENTHUB-287).
type OrganizationAdminHandler struct {
	orgRepo repository.OrganizationRepository
}

func NewOrganizationAdminHandler(orgRepo repository.OrganizationRepository) *OrganizationAdminHandler {
	return &OrganizationAdminHandler{orgRepo: orgRepo}
}

// PendingOrganizationListResponse mirrors UserAdminListResponse so the frontend can page admin lists uniformly.
type PendingOrganizationListResponse struct {
	Items []*model.OrganizationModel `json:"items"`
	Page  int                        `json:"page"`
	Limit int                        `json:"limit"`
	Total int64                      `json:"total"`
}

// @Summary      List pending organizations
// @Description  Returns the organizations waiting for admin approval, oldest request first. Available only to global admins.
// @Tags         organizations
// @Security     BearerAuth
// @Produce      json
// @Param        page  query int false "Page number (1-indexed)" default(1) minimum(1)
// @Param        limit query int false "Page size (1-100)" default(50) minimum(1) maximum(100)
// @Success      200 {object} PendingOrganizationListResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizations/pending [get]
func (h *OrganizationAdminHandler) ListPendingOrganizations(c *gin.Context) {
	page, limit, err := parsePageLimit(c)
	if err != nil {
		writeProblem(c, http.StatusBadRequest, err.Error())
		return
	}

	orgs, total, err := h.orgRepo.ListPending(page, limit)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load pending organizations", "page", page, "limit", limit, "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load pending organizations")
		return
	}
	if orgs == nil {
		orgs = []*model.OrganizationModel{}
	}

	c.JSON(http.StatusOK, PendingOrganizationListResponse{Items: orgs, Page: page, Limit: limit, Total: total})
}

// @Summary      Approve organization
// @Description  Approves a pending organization so it becomes public. Available only to global admins. An organization can be approved only once; approving an already approved organization returns 409.
// @Tags         organizations
// @Security     BearerAuth
// @Produce      json
// @Param        organizationID path string true "Organization ID (database UUID)"
// @Success      200 {object} model.OrganizationModel
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      409 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/organizations/{organizationID}/approve [patch]
func (h *OrganizationAdminHandler) ApproveOrganization(c *gin.Context) {
	id, err := uuid.Parse(c.Param("organizationID"))
	if err != nil {
		writeProblem(c, http.StatusBadRequest, "invalid organization id")
		return
	}

	activated, err := h.orgRepo.Activate(id)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to approve organization", "organization_id", id, "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to approve organization")
		return
	}

	// Activate changes only pending rows, so the row is loaded afterwards: for the response on
	// success, and to tell an unknown organization from an already approved one otherwise.
	org, err := h.orgRepo.GetByID(id)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load organization after approval", "organization_id", id, "activated", activated, "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to load organization")
		return
	}
	if org == nil {
		writeProblem(c, http.StatusNotFound, "organization not found")
		return
	}
	if !activated {
		writeProblem(c, http.StatusConflict, "organization is already approved")
		return
	}

	slog.InfoContext(c.Request.Context(), "organization approved", "event", "organization_approved", "organization_id", id)
	c.JSON(http.StatusOK, org)
}
