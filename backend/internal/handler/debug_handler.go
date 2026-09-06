package handler

import (
	"backend/internal/model"
	"backend/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type debugTokenRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type debugTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

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
// @Param        request body debugTokenRequest true "Credentials"
// @Success      200 {object} debugTokenResponse
// @Failure      400 {object} model.ErrorResponse
// @Failure      500 {object} model.ErrorResponse
// @Router       /debug/token [post]
func (h *DebugHandler) GetToken(c *gin.Context) {
	var req debugTokenRequest
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

	c.JSON(http.StatusOK, debugTokenResponse{
		AccessToken:  jwt.AccessToken,
		TokenType:    jwt.TokenType,
		ExpiresIn:    jwt.ExpiresIn,
		RefreshToken: jwt.RefreshToken,
	})
}
