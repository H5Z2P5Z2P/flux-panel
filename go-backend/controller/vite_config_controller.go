package controller

import (
	"go-backend/result"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
)

type ViteConfigController struct{}

var ViteConfig = new(ViteConfigController)

func (c *ViteConfigController) GetConfigs(ctx *gin.Context) {
	ctx.JSON(200, service.ViteConfig.GetConfigs())
}

func (c *ViteConfigController) GetConfigByName(ctx *gin.Context) {
	var body map[string]interface{}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(200, result.Err(-1, "参数错误"))
		return
	}
	name, ok := body["name"].(string)
	if !ok {
		ctx.JSON(200, result.Err(-1, "缺少name参数"))
		return
	}
	ctx.JSON(200, service.ViteConfig.GetConfigByName(name))
}

func (c *ViteConfigController) UpdateConfigs(ctx *gin.Context) {
	// Require Admin Role
	claims, _ := ctx.Get("claims")
	userClaims := claims.(*utils.UserClaims)
	if userClaims.RoleId != 0 {
		ctx.JSON(200, result.Err(-1, "无权操作"))
		return
	}

	var body map[string]string
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(200, result.Err(-1, "参数错误"))
		return
	}
	ctx.JSON(200, service.ViteConfig.UpdateConfigs(body))
}

func (c *ViteConfigController) UpdateConfig(ctx *gin.Context) {
	// Require Admin Role
	claims, _ := ctx.Get("claims")
	userClaims := claims.(*utils.UserClaims)
	if userClaims.RoleId != 0 {
		ctx.JSON(200, result.Err(-1, "无权操作"))
		return
	}

	var body map[string]interface{}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(200, result.Err(-1, "参数错误"))
		return
	}
	name, ok1 := body["name"].(string)
	value, ok2 := body["value"].(string)
	if !ok1 || !ok2 {
		ctx.JSON(200, result.Err(-1, "参数错误"))
		return
	}
	ctx.JSON(200, service.ViteConfig.UpdateConfig(name, value))
}
