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
