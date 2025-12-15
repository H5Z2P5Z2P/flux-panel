package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"

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
