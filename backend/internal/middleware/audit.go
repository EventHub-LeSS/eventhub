package middleware

import (
	"backend/internal/audit"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Audit marks a route as an audited business operation. It must run after authentication: the
// actor is taken from the verified principal only. The business logic reads the metadata from the
// request context and persists the audit entry together with the change. Without a principal no
// metadata is attached, and audited operations refuse to run.
func Audit(action audit.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c)
		if !ok || principal.Subject == "" {
			c.Next()
			return
		}
		ctx := audit.WithMeta(c.Request.Context(), audit.Meta{
			OperationID:   uuid.New(),
			Action:        action,
			ActorSubject:  principal.Subject,
			ActorUsername: principal.Username,
		})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
