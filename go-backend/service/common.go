package service

import (
	"go-backend/result"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func ResponseError(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, result.Err(code, msg))
}

func isTransientNodeSyncError(err error) bool {
	if err == nil {
		return false
	}
	return isTransientNodeSyncMessage(err.Error())
}

func isTransientNodeSyncMessage(msg string) bool {
	if msg == "" {
		return false
	}
	transientMarkers := []string{
		"节点不在线",
		"Timeout",
		"timeout",
		"connection closed",
		"节点无响应",
	}
	for _, marker := range transientMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func isGostNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return isGostNotFoundMessage(err.Error())
}

func isGostNotFoundMessage(msg string) bool {
	if msg == "" {
		return false
	}
	lowerMsg := strings.ToLower(msg)
	return strings.Contains(lowerMsg, "not found") || strings.Contains(msg, "不存在")
}
