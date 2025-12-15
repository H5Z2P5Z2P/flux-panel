package utils

import (
	"time"

	"go-backend/config"
	"go-backend/model"

	"github.com/golang-jwt/jwt/v5"
)

type UserClaims struct {
	UserId int64  `json:"userId"`
	RoleId int    `json:"roleId"`
	User   string `json:"user"`
	jwt.RegisteredClaims
}

func GenerateToken(user *model.User) (string, error) {
	claims := UserClaims{
		UserId: user.ID,
		RoleId: user.RoleId,
		User:   user.User,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时过期
			Issuer:    "flux-panel",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AppConfig.JwtSecret))
}
