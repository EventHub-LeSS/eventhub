package middleware

import (
	"net/http"

	"backend/internal/model"

	"github.com/gin-gonic/gin"
)

func RequireGlobalRole(roles ...GlobalRole) gin.HandlerFunc {
	allowed := make(map[GlobalRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c)
		if !ok {
			abortAuthentication(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}
		for role := range allowed {
			if principal.HasGlobalRole(role) {
				c.Next()
				return
			}
		}
		AbortForbidden(c)
	}
}

func RequireOrganizationRole(organizationParam string, roles ...OrganizationRole) gin.HandlerFunc {
	allowed := make(map[OrganizationRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c)
		if !ok {
			abortAuthentication(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
			return
		}
		if principal.HasGlobalRole(RoleAdmin) {
			c.Next()
			return
		}

		organizationID := c.Param(organizationParam)
		if organizationID == "" {
			AbortForbidden(c)
			return
		}
		for role := range allowed {
			if principal.HasOrganizationRoleIn(organizationID, role) {
				c.Next()
				return
			}
		}
		AbortForbidden(c)
	}
}

// AbortForbidden answers 403 in the API error format shared by all authorization checks.
func AbortForbidden(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, model.APIError{
		Error: model.APIErrorDetail{
			Code:    "FORBIDDEN",
			Message: "You do not have permission to perform this operation",
		},
	})
}
