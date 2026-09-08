package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type DebugHandler struct {
	keycloakService *service.KeycloakService
}

func NewDebugHandler(keycloakService *service.KeycloakService) *DebugHandler {
	return &DebugHandler{keycloakService: keycloakService}
}

// @Summary      [DEBUG] Get access token
// @Description  TEST ONLY — accepts username/password and returns a Keycloak access token. Disabled in production (requires DEBUG_ENABLED=true).
// @Tags         debug
// @Accept       json
// @Produce      json
// @Param        request body model.DebugTokenRequest true "Credentials"
// @Success      200 {object} model.DebugTokenResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /debug/token [post]
func (h *DebugHandler) GetToken(c *gin.Context) {
	var req model.DebugTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusBadRequest),
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	jwt, err := h.keycloakService.LoginUser(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, model.ErrorResponse{
			Type:   "about:blank",
			Title:  http.StatusText(http.StatusUnauthorized),
			Status: http.StatusUnauthorized,
			Detail: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.DebugTokenResponse{
		AccessToken:  jwt.AccessToken,
		TokenType:    jwt.TokenType,
		ExpiresIn:    jwt.ExpiresIn,
		RefreshToken: jwt.RefreshToken,
	})
}
