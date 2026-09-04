package middleware

import (
	"net/http"

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
		abortForbidden(c)
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
		if organizationID == "" || principal.ActiveOrganization == nil || principal.ActiveOrganization.ID != organizationID {
			abortForbidden(c)
			return
		}
		for role := range allowed {
			if principal.HasOrganizationRole(role) {
				c.Next()
				return
			}
		}
		abortForbidden(c)
	}
}

func abortForbidden(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"code":    "FORBIDDEN",
			"message": "You do not have permission to perform this operation",
		},
	})
}
