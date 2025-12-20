package controller

import (
	"net/http"
	"strconv"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/service"
	"go-backend/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func (u *UserController) GenerateGuestLink(c *gin.Context) {
	claims := c.MustGet("claims").(*utils.UserClaims)
	targetUserId := claims.GetUserId()

	// If userId param is provided and user is admin, use that userId
	userIdStr := c.Query("userId")
	if userIdStr != "" && claims.RoleId == 0 {
		var err error
		targetUserId, err = strconv.ParseInt(userIdStr, 10, 64)
		if err != nil {
			service.ResponseError(c, -1, "Invalid parameters")
			return
		}
	}

	// Check if link exists
	var link model.GuestLink
	if err := global.DB.Where("user_id = ?", targetUserId).First(&link).Error; err == nil {
		c.JSON(http.StatusOK, result.Ok(dto.GuestLinkDto{Token: link.Token}))
		return
	}

	// Create new link
	newLink := model.GuestLink{
		UserID:      targetUserId,
		Token:       uuid.New().String(),
		CreatedTime: time.Now().UnixMilli(),
	}

	if err := global.DB.Create(&newLink).Error; err != nil {
		c.JSON(http.StatusOK, result.Err(-1, "Failed to create guest link"))
		return
	}

	c.JSON(http.StatusOK, result.Ok(dto.GuestLinkDto{Token: newLink.Token}))
}
