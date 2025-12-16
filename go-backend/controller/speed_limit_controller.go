package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"

	"github.com/gin-gonic/gin"
)

type SpeedLimitController struct{}

func (c *SpeedLimitController) Create(ctx *gin.Context) {
	var speedLimitDto dto.SpeedLimitDto
	if err := ctx.ShouldBindJSON(&speedLimitDto); err != nil {
		service.ResponseError(ctx, -1, "参数错误: "+err.Error())
		return
	}
	ctx.JSON(http.StatusOK, service.SpeedLimit.CreateSpeedLimit(speedLimitDto))
}

func (c *SpeedLimitController) List(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, service.SpeedLimit.GetAllSpeedLimits())
}

func (c *SpeedLimitController) Update(ctx *gin.Context) {
	var updateDto dto.SpeedLimitUpdateDto
	if err := ctx.ShouldBindJSON(&updateDto); err != nil {
		service.ResponseError(ctx, -1, "参数错误: "+err.Error())
		return
	}
	ctx.JSON(http.StatusOK, service.SpeedLimit.UpdateSpeedLimit(updateDto))
}

func (c *SpeedLimitController) Delete(ctx *gin.Context) {
	var params map[string]interface{}
	if err := ctx.ShouldBindJSON(&params); err != nil {
		service.ResponseError(ctx, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	ctx.JSON(http.StatusOK, service.SpeedLimit.DeleteSpeedLimit(id))
}

func (c *SpeedLimitController) Tunnels(ctx *gin.Context) {
	// 返回所有隧道列表（用于限速规则关联）
	ctx.JSON(http.StatusOK, service.Tunnel.GetAllTunnels())
}
