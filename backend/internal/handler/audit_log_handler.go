package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/repository"
	"backend/internal/service"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

const (
	defaultAuditLogLimit = 50
	maxAuditLogLimit     = 100
)

type organizationResolver interface {
	ResolveOrganization(ctx context.Context, organizationIDOrAlias string) (service.OrganizationRef, error)
}

type AuditLogHandler struct {
	resolver organizationResolver
	repo     repository.AuditLogRepository
}

func NewAuditLogHandler(resolver organizationResolver, repo repository.AuditLogRepository) *AuditLogHandler {
	return &AuditLogHandler{resolver: resolver, repo: repo}
}

// @Summary      List organization audit log
// @Description  Returns the audit log of an organization, newest entries first: who (personal account) changed what and when. Covers event changes (including price changes, publishing and cancellation) and changes of member roles. Requires the org_admin role in the given organization (global admins bypass this check). organizationID is the Keycloak organization ID or alias. Entries of operations in Keycloak consist of a "started" entry and a "succeeded" or "incomplete" entry with the same operationId; a "started" entry without a result means the outcome is unconfirmed. Entries are deleted ten years after they were recorded. Use nextCursor from the response as cursor to get the next page.
// @Tags         organizations
// @Security     BearerAuth
// @Produce      json
// @Param        organizationID path  string true  "Keycloak organization ID or Alias"
// @Param        limit          query int    false "Entries per page (default 50, maximum 100)"
// @Param        cursor         query string false "Cursor of the previous page"
// @Success      200 {object} model.AuditLogPage
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /organizations/{organizationID}/audit-logs [get]
func (h *AuditLogHandler) ListAuditLogs(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.APIError{
			Error: model.APIErrorDetail{Code: "UNAUTHENTICATED", Message: "Authentication is required"},
		})
		return
	}
	globalAdmin := principal.HasGlobalRole(middleware.RoleAdmin)

	org, err := h.resolver.ResolveOrganization(c.Request.Context(), c.Param("organizationID"))
	if err != nil {
		if errors.Is(err, service.ErrOrganizationNotFound) {
			if globalAdmin {
				writeProblem(c, http.StatusNotFound, err.Error())
			} else {
				// Unknown and foreign organizations look the same to organization admins.
				middleware.AbortForbidden(c)
			}
			return
		}
		slog.Error("audit log: organization lookup failed", "error", err.Error())
		writeProblem(c, http.StatusInternalServerError, "failed to look up the organization")
		return
	}
	if !globalAdmin &&
		!principal.HasOrganizationRoleIn(org.ID, middleware.RoleOrganizationAdmin) &&
		!principal.HasOrganizationRoleIn(org.Alias, middleware.RoleOrganizationAdmin) {
		middleware.AbortForbidden(c)
		return
	}

	limit := defaultAuditLogLimit
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxAuditLogLimit {
			writeProblem(c, http.StatusBadRequest, "limit must be a number between 1 and "+strconv.Itoa(maxAuditLogLimit))
			return
		}
	}
	var cursor *model.AuditLogCursor
	if raw := c.Query("cursor"); raw != "" {
		decoded, err := model.DecodeAuditLogCursor(raw)
		if err != nil {
			writeProblem(c, http.StatusBadRequest, "invalid cursor")
			return
		}
		cursor = &decoded
	}

	entries, err := h.repo.ListByOrganization(org.ID, limit+1, cursor)
	if err != nil {
		slog.Error("audit log: query failed", "organization_id", org.ID, "error", err.Error())
		writeProblem(c, http.StatusInternalServerError, "failed to load the audit log")
		return
	}

	page := model.AuditLogPage{Items: entries}
	if len(entries) > limit {
		page.Items = entries[:limit]
		last := page.Items[limit-1]
		next := model.AuditLogCursor{OccurredAt: last.OccurredAt, ID: last.AuditLogID}.Encode()
		page.NextCursor = &next
	}
	c.JSON(http.StatusOK, page)
}
