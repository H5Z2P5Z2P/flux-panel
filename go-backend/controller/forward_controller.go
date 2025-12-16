package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
)

type ForwardController struct{}

func (u *ForwardController) Create(c *gin.Context) {
	var dto dto.ForwardDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.CreateForward(dto, claims))
}

func (u *ForwardController) List(c *gin.Context) {
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.GetAllForwards(claims))
}

func (u *ForwardController) Update(c *gin.Context) {
	var updateDto dto.ForwardUpdateDto
	if err := c.ShouldBindJSON(&updateDto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}

	// Convert UpdateDto to ForwardDto for service call
	// Or even better, update service to accept ForwardUpdateDto?
	// Given service accepts ForwardDto currently:
	forwardDto := dto.ForwardDto{
		TunnelId:      updateDto.TunnelId,
		Name:          updateDto.Name,
		RemoteAddr:    updateDto.RemoteAddr,
		InPort:        updateDto.InPort,
		InterfaceName: updateDto.InterfaceName,
		Strategy:      updateDto.Strategy,
	}

	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.UpdateForward(updateDto.ID, forwardDto, claims))
}

func (u *ForwardController) Delete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.DeleteForward(id, claims))
}

func (u *ForwardController) Pause(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.PauseForward(id, claims))
}

func (u *ForwardController) Resume(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.ResumeForward(id, claims))
}

func (u *ForwardController) ForceDelete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.ForceDeleteForward(id, claims))
}

func (u *ForwardController) Diagnose(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}

	// Support both id and forwardId
	var idVal interface{}
	if v, ok := params["forwardId"]; ok {
		idVal = v
	} else if v, ok := params["id"]; ok {
		idVal = v
	} else {
		service.ResponseError(c, -1, "缺少forwardId参数")
		return
	}

	var id int64
	if v, ok := idVal.(float64); ok {
		id = int64(v)
	} else {
		service.ResponseError(c, -1, "参数格式错误")
		return
	}

	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.DiagnoseForward(id, claims))
}

func (u *ForwardController) UpdateOrder(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.Forward.UpdateForwardOrder(params, claims))
}
