package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/audit"

	"github.com/gin-gonic/gin"
)

func serveAudit(t *testing.T, principal *Principal, header http.Header) (audit.Meta, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var (
		meta audit.Meta
		err  error
	)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if principal != nil {
			SetPrincipalForRequest(c, principal)
		}
		c.Next()
	})
	r.POST("/x", Audit(audit.EventUpdated), func(c *gin.Context) {
		meta, err = audit.MetaFromContext(c.Request.Context(), audit.EventUpdated)
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	for k, v := range header {
		req.Header[k] = v
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
	return meta, err
}

func TestAudit_TakesActorFromVerifiedPrincipal(t *testing.T) {
	meta, err := serveAudit(t, &Principal{Subject: "sub-anna", Username: "anna@acme.test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ActorSubject != "sub-anna" || meta.ActorUsername != "anna@acme.test" || meta.Action != audit.EventUpdated {
		t.Errorf("meta = %+v", meta)
	}
	if meta.OperationID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("operation ID not set")
	}
}

func TestAudit_IgnoresActorDataFromTheRequest(t *testing.T) {
	header := http.Header{"X-User": {"mallory"}, "X-Actor-Subject": {"sub-mallory"}, "X-Forwarded-User": {"mallory"}}
	meta, err := serveAudit(t, &Principal{Subject: "sub-anna", Username: "anna"}, header)
	if err != nil || meta.ActorSubject != "sub-anna" {
		t.Fatalf("meta = %+v, err = %v", meta, err)
	}
}

func TestAudit_WithoutPrincipalAttachesNoContext(t *testing.T) {
	for name, principal := range map[string]*Principal{"none": nil, "without subject": {Username: "x"}} {
		t.Run(name, func(t *testing.T) {
			if _, err := serveAudit(t, principal, nil); err == nil {
				t.Fatal("audit context must not exist without an authenticated principal")
			}
		})
	}
}

func TestAudit_EveryRequestGetsItsOwnOperationID(t *testing.T) {
	p := &Principal{Subject: "sub-anna"}
	first, _ := serveAudit(t, p, nil)
	second, _ := serveAudit(t, p, nil)
	if first.OperationID == second.OperationID {
		t.Error("operation IDs must be unique per request")
	}
}
