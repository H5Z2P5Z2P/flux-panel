package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
)

type TunnelController struct{}

func (u *TunnelController) Create(c *gin.Context) {
	var dto dto.TunnelDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.Tunnel.CreateTunnel(dto))
}

func (u *TunnelController) List(c *gin.Context) {
	c.JSON(http.StatusOK, service.Tunnel.GetAllTunnels())
}

func (u *TunnelController) Update(c *gin.Context) {
	var dto dto.TunnelUpdateDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.Tunnel.UpdateTunnel(dto))
}

func (u *TunnelController) Delete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	c.JSON(http.StatusOK, service.Tunnel.DeleteTunnel(id))
}

// --- UserTunnel Management ---

func (u *TunnelController) AssignUserTunnel(c *gin.Context) {
	var userTunnelDto dto.UserTunnelDto
	if err := c.ShouldBindJSON(&userTunnelDto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.UserTunnel.AssignUserTunnel(userTunnelDto))
}

func (u *TunnelController) ListUserTunnels(c *gin.Context) {
	var queryDto dto.UserTunnelQueryDto
	c.ShouldBindJSON(&queryDto)
	c.JSON(http.StatusOK, service.UserTunnel.GetUserTunnelList(queryDto))
}

func (u *TunnelController) RemoveUserTunnel(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int(params["id"].(float64))
	c.JSON(http.StatusOK, service.UserTunnel.RemoveUserTunnel(id))
}

func (u *TunnelController) UpdateUserTunnel(c *gin.Context) {
	var updateDto dto.UserTunnelUpdateDto
	if err := c.ShouldBindJSON(&updateDto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.UserTunnel.UpdateUserTunnel(updateDto))
}

func (u *TunnelController) GetUserTunnels(c *gin.Context) {
	// 获取当前用户的所有隧道权限 (下拉列表用)
	claims, _ := c.Get("claims")
	userId := claims.(*utils.UserClaims).GetUserId()
	// Call Tunnel Service, not UserTunnel Service
	c.JSON(http.StatusOK, service.Tunnel.UserTunnel(userId))
}

func (u *TunnelController) DiagnoseTunnel(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	tunnelId := int64(params["tunnelId"].(float64))
	c.JSON(http.StatusOK, service.Tunnel.DiagnoseTunnel(tunnelId))
}
