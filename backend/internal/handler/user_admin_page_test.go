package handler

import (
	"backend/internal/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type pageUserRepository struct {
	load func(context.Context, int, int) ([]*model.UserModel, int64, error)
}

func (r pageUserRepository) GetPage(ctx context.Context, page, limit int) ([]*model.UserModel, int64, error) {
	return r.load(ctx, page, limit)
}

func (r pageUserRepository) GetByID(uuid.UUID) (*model.UserModel, error) {
	panic("list must not fetch individual database users")
}

type pageUserRoles struct {
	load func(context.Context, []string) ([][]string, error)
}

func (r pageUserRoles) GetUsersGlobalRoles(ctx context.Context, ids []string) ([][]string, error) {
	return r.load(ctx, ids)
}

func (r pageUserRoles) GetUserGlobalRoles(context.Context, string) ([]string, error) {
	panic("list must use page role lookup")
}

func (r pageUserRoles) SetUserGlobalRoles(context.Context, string, []string) ([]string, error) {
	panic("list must not modify roles")
}

func TestListUsersFetchesOnlyDatabasePage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, total := range []int64{25, 1000000} {
		t.Run(fmt.Sprint(total), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			users := []*model.UserModel{
				{UserID: uuid.New(), KeycloakUserID: "page-a", Email: "a@example.test"},
				{UserID: uuid.New(), KeycloakUserID: "page-b", Email: "b@example.test"},
			}
			pageCalls, roleCalls := 0, 0
			repo := pageUserRepository{load: func(gotCtx context.Context, page, limit int) ([]*model.UserModel, int64, error) {
				pageCalls++
				if gotCtx != ctx || page != 2 || limit != 2 {
					t.Fatalf("unexpected page request: %d/%d", page, limit)
				}
				return users, total, nil
			}}
			roles := pageUserRoles{load: func(gotCtx context.Context, ids []string) ([][]string, error) {
				roleCalls++
				if gotCtx != ctx || !reflect.DeepEqual(ids, []string{"page-a", "page-b"}) {
					return nil, errors.New("off-page user has broken Keycloak state")
				}
				return [][]string{{"admin"}, {}}, nil
			}}
			recorder := runUserPageRequest(NewUserAdminHandler(roles, repo), "/admin/users?page=2&limit=2", ctx)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			var body UserAdminListResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if pageCalls != 1 || roleCalls != 1 || body.Total != total || body.Page != 2 || body.Limit != 2 || len(body.Items) != 2 {
				t.Fatalf("unexpected calls/result: %d/%d, %+v", pageCalls, roleCalls, body)
			}
			if body.Items[0].UserID != users[0].UserID || body.Items[1].UserID != users[1].UserID ||
				!reflect.DeepEqual(body.Items[0].Roles, []string{"admin"}) || body.Items[1].Roles == nil {
				t.Fatalf("order/roles changed: %+v", body.Items)
			}
		})
	}
}

func TestListUsersEmptyPageSkipsKeycloak(t *testing.T) {
	for _, page := range []int{1, 4, math.MaxInt} {
		repo := pageUserRepository{load: func(_ context.Context, gotPage, limit int) ([]*model.UserModel, int64, error) {
			if gotPage != page || limit != 50 {
				t.Fatalf("page=%d limit=%d", gotPage, limit)
			}
			return []*model.UserModel{}, 5, nil
		}}
		roles := pageUserRoles{load: func(context.Context, []string) ([][]string, error) {
			t.Fatal("Keycloak called for empty page")
			return nil, nil
		}}
		recorder := runUserPageRequest(NewUserAdminHandler(roles, repo), fmt.Sprintf("/admin/users?page=%d", page), context.Background())
		if recorder.Code != http.StatusOK {
			t.Fatalf("status=%d, body=%s", recorder.Code, recorder.Body)
		}
		var body UserAdminListResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Items == nil || len(body.Items) != 0 || body.Total != 5 {
			t.Fatalf("unexpected empty response: %+v", body)
		}
	}
}

func TestListUsersQueryValidation(t *testing.T) {
	for _, query := range []string{"page=0", "page=-1", "page=no", "limit=0", "limit=-1", "limit=101", "limit=no", "page=999999999999999999999999"} {
		repo := pageUserRepository{load: func(context.Context, int, int) ([]*model.UserModel, int64, error) {
			t.Fatal("database queried for invalid parameters")
			return nil, 0, nil
		}}
		recorder := runUserPageRequest(NewUserAdminHandler(pageUserRoles{}, repo), "/admin/users?"+query, context.Background())
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s: status=%d", query, recorder.Code)
		}
	}
}

func TestListUsersPageErrors(t *testing.T) {
	for _, failure := range []string{"database", "keycloak", "incomplete roles"} {
		t.Run(failure, func(t *testing.T) {
			roleCalls := 0
			repo := pageUserRepository{load: func(context.Context, int, int) ([]*model.UserModel, int64, error) {
				if failure == "database" {
					return nil, 0, errors.New("query failed")
				}
				return []*model.UserModel{{KeycloakUserID: "page-user"}}, 100, nil
			}}
			roles := pageUserRoles{load: func(context.Context, []string) ([][]string, error) {
				roleCalls++
				if failure == "incomplete roles" {
					return [][]string{}, nil
				}
				return nil, errors.New("on-page Keycloak error")
			}}
			recorder := runUserPageRequest(NewUserAdminHandler(roles, repo), "/admin/users", context.Background())
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
			}
			if failure == "database" && roleCalls != 0 {
				t.Fatal("Keycloak called after database error")
			}
		})
	}
}

func runUserPageRequest(h *UserAdminHandler, path string, ctx context.Context) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/admin/users", h.ListUsers)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil).WithContext(ctx))
	return recorder
}
