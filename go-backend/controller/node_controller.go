package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"

	"github.com/gin-gonic/gin"
)

type NodeController struct{}

func (u *NodeController) Create(c *gin.Context) {
	var dto dto.NodeDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.Node.CreateNode(dto))
}

func (u *NodeController) List(c *gin.Context) {
	c.JSON(http.StatusOK, service.Node.GetAllNodes())
}

func (u *NodeController) Update(c *gin.Context) {
	var dto dto.NodeUpdateDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.Node.UpdateNode(dto))
}

func (u *NodeController) Delete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	c.JSON(http.StatusOK, service.Node.DeleteNode(id))
}

func (u *NodeController) Install(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	c.JSON(http.StatusOK, service.Node.GetInstallCommand(id))
}
