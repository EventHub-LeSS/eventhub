package handler

import (
	"backend/internal/audit"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// EVENTHUB-293: admins list the organizations requested via the contact form and approve them.
// An organization only becomes public once an admin has approved it (EVENTHUB-287).
type OrganizationAdminHandler struct {
	orgRepo   repository.OrganizationRepository
	approvals *service.OrganizationApprovalService
}

func NewOrganizationAdminHandler(orgRepo repository.OrganizationRepository, approvals *service.OrganizationApprovalService) *OrganizationAdminHandler {
	return &OrganizationAdminHandler{orgRepo: orgRepo, approvals: approvals}
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
// @Description  Approves a pending organization so it becomes public. Available only to global admins; the approval is recorded in the audit log. An organization can be approved only once; approving an already approved organization returns 409.
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

	org, err := h.approvals.ApproveOrganization(c.Request.Context(), id)
	switch {
	case err == nil:
		slog.InfoContext(c.Request.Context(), "organization approved", "event", "organization_approved", "organization_id", id)
		c.JSON(http.StatusOK, org)
	case errors.Is(err, service.ErrOrganizationNotFound):
		writeProblem(c, http.StatusNotFound, "organization not found")
	case errors.Is(err, service.ErrOrganizationAlreadyApproved):
		writeProblem(c, http.StatusConflict, "organization is already approved")
	case errors.Is(err, audit.ErrUnavailable), errors.Is(err, audit.ErrMissingContext):
		writeProblem(c, http.StatusInternalServerError, "the audit log is unavailable, the organization was not approved")
	default:
		slog.ErrorContext(c.Request.Context(), "failed to approve organization", "organization_id", id, "error", err)
		writeProblem(c, http.StatusInternalServerError, "failed to approve organization")
	}
}
