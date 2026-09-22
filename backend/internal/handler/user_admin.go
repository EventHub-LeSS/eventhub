package handler

import (
	"backend/internal/middleware"
	"backend/internal/model"
	"backend/internal/service"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var allowedAdminRoleNames = map[string]struct{}{
	string(middleware.RoleAdmin):     {},
	string(middleware.RoleModerator): {},
	string(middleware.RoleVisitor):   {},
}

// UserAdminHandler manages backend user administration endpoints for global admins.
type UserAdminHandler struct {
	userRepo        adminUserRepository
	keycloakService adminUserRoles
}

type adminUserRepository interface {
	GetPage(context.Context, int, int) ([]*model.UserModel, int64, error)
	GetByID(uuid.UUID) (*model.UserModel, error)
}

type adminUserRoles interface {
	GetUsersGlobalRoles(context.Context, []string) ([][]string, error)
	GetUserGlobalRoles(context.Context, string) ([]string, error)
	SetUserGlobalRoles(context.Context, string, []string) ([]string, error)
}

func NewUserAdminHandler(keycloakService adminUserRoles, userRepo adminUserRepository) *UserAdminHandler {
	return &UserAdminHandler{keycloakService: keycloakService, userRepo: userRepo}
}

// UserAdminListItem is the subset of user data exposed to administrators.
type UserAdminListItem struct {
	UserID         uuid.UUID `json:"userId"`
	KeycloakUserID string    `json:"keycloakUserId"`
	FirstName      string    `json:"firstName"`
	LastName       string    `json:"lastName"`
	Email          string    `json:"email"`
	PhoneNumber    *string   `json:"phoneNumber,omitempty"`
	Roles          []string  `json:"roles"`
}

// UserAdminRolesResponse exposes a user's effective global roles.
type UserAdminRolesResponse struct {
	UserID         uuid.UUID `json:"userId"`
	KeycloakUserID string    `json:"keycloakUserId"`
	Roles          []string  `json:"roles"`
}

// UserAdminListResponse wraps paginated user items.
type UserAdminListResponse struct {
	Items []UserAdminListItem `json:"items"`
	Page  int                 `json:"page"`
	Limit int                 `json:"limit"`
	Total int64               `json:"total"`
}

// UserAdminRolesRequest sets the complete global role set for a user.
type UserAdminRolesRequest struct {
	Roles []string `json:"roles" binding:"required"`
}

// @Summary      List users
// @Description  Returns a database page of shared users ordered by userId ascending, with their current global roles. Only users on this page are looked up in Keycloak, with at most four concurrent lookups and a 15-second total role-lookup timeout. Available only to global admins. total counts all database users; concurrent user creation or deletion can cause count/page differences.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Param        page query int false "Page number (1-indexed)" default(1) minimum(1)
// @Param        limit query int false "Page size (1-100)" default(50) minimum(1) maximum(100)
// @Success      200 {object} UserAdminListResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/users [get]
func (h *UserAdminHandler) ListUsers(c *gin.Context) {
	page, limit, err := parsePageLimit(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	users, total, err := h.userRepo.GetPage(c.Request.Context(), page, limit)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load admin user page", "page", page, "limit", limit, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "failed to load users",
		})
		return
	}

	items := make([]UserAdminListItem, 0, len(users))
	ids := make([]string, len(users))
	for i, user := range users {
		if user == nil {
			writeProblem(c, http.StatusInternalServerError, "invalid user returned by database")
			return
		}
		ids[i] = user.KeycloakUserID
	}
	if len(users) > 0 {
		roles, err := h.keycloakService.GetUsersGlobalRoles(c.Request.Context(), ids)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "failed to load admin user page roles", "page", page, "limit", limit, "error", err)
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Type:   "about:blank",
				Title:  http.StatusText(http.StatusInternalServerError),
				Status: http.StatusInternalServerError,
				Detail: "failed to load user roles",
			})
			return
		}
		if len(roles) != len(users) {
			writeProblem(c, http.StatusInternalServerError, "incomplete user roles response")
			return
		}
		for i, user := range users {
			items = append(items, UserAdminListItem{
				UserID:         user.UserID,
				KeycloakUserID: user.KeycloakUserID,
				FirstName:      user.FirstName,
				LastName:       user.LastName,
				Email:          user.Email,
				PhoneNumber:    user.PhoneNumber,
				Roles:          roles[i],
			})
		}
	}

	c.JSON(http.StatusOK, UserAdminListResponse{
		Items: items,
		Page:  page,
		Limit: limit,
		Total: total,
	})
}

// @Summary      Get user roles
// @Description  Returns the effective global roles for a user. Available only to global admins.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Param        userID path string true "Database user UUID"
// @Success      200 {object} UserAdminRolesResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/users/{userID}/roles [get]
func (h *UserAdminHandler) GetUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: "invalid user id",
		})
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load user by id", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "failed to load user",
		})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusNotFound),
			Status: http.StatusNotFound,
			Detail: "user not found",
		})
		return
	}

	roles, err := h.keycloakService.GetUserGlobalRoles(c.Request.Context(), user.KeycloakUserID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load user global roles", "user_id", userID, "keycloak_user_id", user.KeycloakUserID, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "failed to load roles",
		})
		return
	}

	c.JSON(http.StatusOK, UserAdminRolesResponse{
		UserID:         user.UserID,
		KeycloakUserID: user.KeycloakUserID,
		Roles:          roles,
	})
}

// @Summary      Update user roles
// @Description  Replaces the user's global roles with the submitted set. Only global admins may perform this change; the change is applied in Keycloak and takes effect after a fresh token or login.
// @Tags         users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        userID path string true "Database user UUID"
// @Param        request body UserAdminRolesRequest true "Roles to assign"
// @Success      200 {object} UserAdminRolesResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      401 {object} model.APIError
// @Failure      403 {object} model.APIError
// @Failure      404 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /admin/users/{userID}/roles [put]
func (h *UserAdminHandler) UpdateUserRoles(c *gin.Context) {
	userID, err := uuid.Parse(c.Param("userID"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: "invalid user id",
		})
		return
	}

	var req UserAdminRolesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: "request body must be a JSON object containing a roles array",
		})
		return
	}

	user, err := h.userRepo.GetByID(userID)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to load user by id", "user_id", userID, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "failed to load user",
		})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusNotFound),
			Status: http.StatusNotFound,
			Detail: "user not found",
		})
		return
	}

	normalized, err := normalizeGlobalRoleNames(req.Roles)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	updated, err := h.keycloakService.SetUserGlobalRoles(c.Request.Context(), user.KeycloakUserID, normalized)
	if err != nil {
		if errors.Is(err, service.ErrLastGlobalAdmin) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Type:   "about:blank",
				Title:  http.StatusText(http.StatusConflict),
				Status: http.StatusConflict,
				Detail: err.Error(),
			})
			return
		}
		slog.ErrorContext(c.Request.Context(), "failed to update user global roles", "user_id", userID, "keycloak_user_id", user.KeycloakUserID, "error", err)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusInternalServerError),
			Status: http.StatusInternalServerError,
			Detail: "failed to update user roles",
		})
		return
	}

	c.JSON(http.StatusOK, UserAdminRolesResponse{
		UserID:         user.UserID,
		KeycloakUserID: user.KeycloakUserID,
		Roles:          updated,
	})
}

func parsePageLimit(c *gin.Context) (int, int, error) {
	page := 1
	if value := c.Query("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return 0, 0, fmt.Errorf("page must be a positive integer")
		}
		page = parsed
	}

	limit := 50
	if value := c.Query("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		if parsed > 100 {
			return 0, 0, fmt.Errorf("limit must be <= 100")
		}
		limit = parsed
	}
	return page, limit, nil
}

func normalizeGlobalRoleNames(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]struct{}, len(input))
	ordered := make([]string, 0, len(input))
	for _, raw := range input {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("unsupported role %q; allowed roles: admin, moderator, visitor", name)
		}
		if _, ok := allowedAdminRoleNames[name]; !ok {
			return nil, fmt.Errorf("unsupported role %q; allowed roles: admin, moderator, visitor", name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	return ordered, nil
}
