package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParsePageLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("defaults", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/users", nil)

		page, limit, err := parsePageLimit(c)
		if err != nil {
			t.Fatalf("parsePageLimit() unexpected error: %v", err)
		}
		if page != 1 || limit != 50 {
			t.Fatalf("parsePageLimit() = (%d, %d), want (1, 50)", page, limit)
		}
	})

	t.Run("valid custom values", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/users?page=2&limit=10", nil)

		page, limit, err := parsePageLimit(c)
		if err != nil {
			t.Fatalf("parsePageLimit() unexpected error: %v", err)
		}
		if page != 2 || limit != 10 {
			t.Fatalf("parsePageLimit() = (%d, %d), want (2, 10)", page, limit)
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/users?page=0&limit=101", nil)

		_, _, err := parsePageLimit(c)
		if err == nil {
			t.Fatal("parsePageLimit() expected error for invalid page/limit")
		}
	})
}
