package handler

import (
	"backend/internal/model"
	"net/http"

	"github.com/gin-gonic/gin"
)

func writeProblem(c *gin.Context, status int, detail string) {
	c.JSON(status, model.ErrorResponse{
		Type:   "about:blank",
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}
