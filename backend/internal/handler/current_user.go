package handler

import (
	"net/http"

	"backend/internal/middleware"

	"github.com/gin-gonic/gin"
)

type CurrentUserResponse struct {
	Subject       string                       `json:"subject"`
	Username      string                       `json:"username"`
	GlobalRoles   []middleware.GlobalRole      `json:"globalRoles"`
	Organization  *CurrentOrganizationResponse `json:"organization,omitempty"`
	Organizations []CurrentOrganizationResponse `json:"organizations"`
}

type CurrentOrganizationResponse struct {
	ID    string                        `json:"id"`
	Alias string                        `json:"alias"`
	Roles []middleware.OrganizationRole `json:"roles"`
}

// @Summary      Get current user
// @Description  Returns the authenticated user's profile, global roles, active organization, and organization memberships
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} CurrentUserResponse
// @Failure      401 {object} model.APIError
// @Router       /users/me [get]
func CurrentUser(c *gin.Context) {
	principal, ok := middleware.PrincipalFromContext(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{"code": "UNAUTHENTICATED", "message": "Authentication is required"},
		})
		return
	}

	response := CurrentUserResponse{
		Subject:       principal.Subject,
		Username:      principal.Username,
		GlobalRoles:   make([]middleware.GlobalRole, 0, len(principal.GlobalRoles)),
		Organizations: make([]CurrentOrganizationResponse, 0, len(principal.Organizations)),
	}
	for _, role := range []middleware.GlobalRole{
		middleware.RoleAdmin,
		middleware.RoleModerator,
		middleware.RoleVisitor,
	} {
		if principal.HasGlobalRole(role) {
			response.GlobalRoles = append(response.GlobalRoles, role)
		}
	}

	for _, org := range principal.Organizations {
		response.Organizations = append(response.Organizations, CurrentOrganizationResponse{
			ID:    org.ID,
			Alias: org.Alias,
			Roles: orderedOrganizationRoles(org.Roles),
		})
	}

	if principal.ActiveOrganization != nil {
		response.Organization = &CurrentOrganizationResponse{
			ID:    principal.ActiveOrganization.ID,
			Alias: principal.ActiveOrganization.Alias,
			Roles: orderedOrganizationRoles(principal.ActiveOrganization.Roles),
		}
	}

	c.JSON(http.StatusOK, response)
}

func orderedOrganizationRoles(roles map[middleware.OrganizationRole]struct{}) []middleware.OrganizationRole {
	ordered := make([]middleware.OrganizationRole, 0, len(roles))
	for _, role := range []middleware.OrganizationRole{
		middleware.RoleOrganizationAdmin,
		middleware.RoleEventManager,
		middleware.RoleFinanceViewer,
	} {
		if _, ok := roles[role]; ok {
			ordered = append(ordered, role)
		}
	}
	return ordered
}
