package utils

import (
	"strconv"
	"time"

	"go-backend/config"
	"go-backend/model"

	"github.com/golang-jwt/jwt/v5"
)

type UserClaims struct {
	RoleId int    `json:"role_id"`
	User   string `json:"user"`
	Name   string `json:"name"` // Java sets this to user.User
	jwt.RegisteredClaims
}

func GenerateToken(user *model.User) (string, error) {
	claims := UserClaims{
		RoleId: user.RoleId,
		User:   user.User,
		Name:   user.User, // Java: payload.put("name", user.getUser());
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // 24小时过期
			Issuer:    "flux-panel",                                       // Java doesn't strictly set iss, but verifies it? No, Java validateToken only checks signature and exp.
			// Java sets "sub" to userId.toString()
			Subject: strconv.FormatInt(user.ID, 10),
		},
	}

	// Java sets "typ": "JWT" in header, golang-jwt does this by default or we can leave it.
	// Key exact match: sub, iat, exp, user, name, role_id.

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AppConfig.JwtSecret))
}

func (c *UserClaims) GetUserId() int64 {
	id, _ := strconv.ParseInt(c.Subject, 10, 64)
	return id
}
