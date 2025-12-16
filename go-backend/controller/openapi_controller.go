package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"go-backend/global"
	"go-backend/model"
	"go-backend/result"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type OpenAPIController struct{}

const gigaBytes = 1024 * 1024 * 1024

func (o *OpenAPIController) SubStore(c *gin.Context) {
	username := c.Query("user")
	password := c.Query("pwd")
	if username == "" {
		c.JSON(http.StatusOK, result.Err(-1, "用户不能为空"))
		return
	}
	if password == "" {
		c.JSON(http.StatusOK, result.Err(-1, "密码不能为空"))
		return
	}

	var user model.User
	if err := global.DB.Where("user = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusOK, result.Err(-1, "鉴权失败"))
		} else {
			c.JSON(http.StatusOK, result.Err(-1, "查询用户失败"))
		}
		return
	}

	if utils.Md5(password) != user.Pwd {
		c.JSON(http.StatusOK, result.Err(-1, "鉴权失败"))
		return
	}

	tunnelParam := c.DefaultQuery("tunnel", "-1")
	headerValue, err := o.buildHeaderValue(user, tunnelParam)
	if err != nil {
		c.JSON(http.StatusOK, result.Err(-1, err.Error()))
		return
	}

	c.Header("subscription-userinfo", headerValue)
	c.String(http.StatusOK, headerValue)
}

func (o *OpenAPIController) buildHeaderValue(user model.User, tunnelParam string) (string, error) {
	if tunnelParam == "-1" {
		return buildSubscriptionHeader(
			user.OutFlow,
			user.InFlow,
			user.Flow*int64(gigaBytes),
			user.ExpTime/1000,
		), nil
	}

	tunnelID, err := strconv.ParseInt(tunnelParam, 10, 64)
	if err != nil {
		return "", fmt.Errorf("隧道不存在")
	}

	var userTunnel model.UserTunnel
	if err := global.DB.First(&userTunnel, tunnelID).Error; err != nil {
		return "", fmt.Errorf("隧道不存在")
	}

	if int64(userTunnel.UserId) != user.ID {
		return "", fmt.Errorf("隧道不存在")
	}

	return buildSubscriptionHeader(
		userTunnel.OutFlow,
		userTunnel.InFlow,
		userTunnel.Flow*int64(gigaBytes),
		userTunnel.ExpTime/1000,
	), nil
}

func buildSubscriptionHeader(upload, download, total, expire int64) string {
	return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", download, upload, total, expire)
}
