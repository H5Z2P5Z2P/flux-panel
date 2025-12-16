package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
)

type UserController struct{}

func (u *UserController) Login(c *gin.Context) {
	var loginDto dto.LoginDto
	if err := c.ShouldBindJSON(&loginDto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.User.Login(loginDto))
}

func (u *UserController) Create(c *gin.Context) {
	var dto dto.UserDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	c.JSON(http.StatusOK, service.User.CreateUser(dto))
}

func (u *UserController) List(c *gin.Context) {
	c.JSON(http.StatusOK, service.User.GetAllUsers())
}

func (u *UserController) Update(c *gin.Context) {
	var dto dto.UserUpdateDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	c.JSON(http.StatusOK, service.User.UpdateUser(dto))
}

func (u *UserController) Delete(c *gin.Context) {
	var params map[string]interface{}
	if err := c.ShouldBindJSON(&params); err != nil {
		service.ResponseError(c, -1, "参数错误")
		return
	}
	id := int64(params["id"].(float64))
	c.JSON(http.StatusOK, service.User.DeleteUser(id))
}

func (u *UserController) UpdatePassword(c *gin.Context) {
	var dto dto.ChangePasswordDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.User.UpdatePassword(dto, claims))
}

func (u *UserController) Package(c *gin.Context) {
	claims := c.MustGet("claims").(*utils.UserClaims)
	c.JSON(http.StatusOK, service.User.GetUserPackageInfo(claims))
}

func (u *UserController) Reset(c *gin.Context) {
	var dto dto.ResetFlowDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		service.ResponseError(c, -1, "参数错误: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, service.User.ResetFlow(dto))
}
