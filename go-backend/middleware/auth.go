package middleware

import (
	"net/http"
	"strings"

	"go-backend/config"
	"go-backend/result"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusOK, result.Err(-1, "未登录"))
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusOK, result.Err(-1, "无效的Token格式"))
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims := &utils.UserClaims{}

		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(config.AppConfig.JwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusOK, result.Err(-1, "Token无效或已过期"))
			c.Abort()
			return
		}

		c.Set("claims", claims)
		c.Next()
	}
}

func RequireRole(roleId int) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, exists := c.Get("claims")
		if !exists {
			c.JSON(http.StatusOK, result.Err(-1, "未授权"))
			c.Abort()
			return
		}
		userClaims := claims.(*utils.UserClaims)
		if userClaims.RoleId > roleId { // Assuming lower roleId is higher privilege (0 is admin)
			c.JSON(http.StatusOK, result.Err(-1, "权限不足"))
			c.Abort()
			return
		}
		c.Next()
	}
}
