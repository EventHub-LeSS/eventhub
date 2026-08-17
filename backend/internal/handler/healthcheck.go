package handler

import (
	"backend/internal/model"
	"time"

	"github.com/gin-gonic/gin"
)

func Healthcheck(c *gin.Context) {
	c.JSON(200, model.HealthcheckModel{
		Message: "OK",
		Time:    time.Now(),
	})
}
