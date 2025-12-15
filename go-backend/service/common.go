package service

import (
	"go-backend/result"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ResponseError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, result.Err(code, msg))
}
