package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

type updateUserRepoStub struct {
	user *model.UserModel
}

func (r updateUserRepoStub) GetPage(context.Context, int, int) ([]*model.UserModel, int64, error) {
	panic("UpdateUserRoles must not call GetPage")
}

func (r updateUserRepoStub) GetByID(id uuid.UUID) (*model.UserModel, error) {
	if r.user != nil && r.user.UserID == id {
		return r.user, nil
	}
	return nil, nil
}

type updateUserRolesStub struct{}

func (updateUserRolesStub) GetUsersGlobalRoles(context.Context, []string) ([][]string, error) {
	panic("UpdateUserRoles must not call GetUsersGlobalRoles")
}

func (updateUserRolesStub) GetUserGlobalRoles(context.Context, string) ([]string, error) {
	panic("UpdateUserRoles must not call GetUserGlobalRoles")
}

func (updateUserRolesStub) SetUserGlobalRoles(context.Context, string, []string) ([]string, error) {
	return nil, service.ErrLastGlobalAdmin
}

func TestUpdateUserRolesReturnsConflictForLastGlobalAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userID := uuid.New()
	repo := updateUserRepoStub{user: &model.UserModel{
		UserID:         userID,
		KeycloakUserID: "kc-user-1",
	}}
	router := gin.New()
	router.PUT("/admin/users/:userID/roles", NewUserAdminHandler(updateUserRolesStub{}, repo).UpdateUserRoles)
	body, _ := json.Marshal(UserAdminRolesRequest{Roles: []string{"visitor"}})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+userID.String()+"/roles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
