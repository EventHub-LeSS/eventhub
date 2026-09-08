package handler

import (
	"backend/internal/model"
	"time"

	"github.com/gin-gonic/gin"
)

// @Summary      Health check
// @Description  Returns service health status
// @Tags         system
// @Produce      json
// @Success      200 {object} model.HealthcheckModel
// @Router       / [get]
func Healthcheck(c *gin.Context) {
	c.JSON(200, model.HealthcheckModel{
		Message: "OK",
		Time:    time.Now(),
	})
}
