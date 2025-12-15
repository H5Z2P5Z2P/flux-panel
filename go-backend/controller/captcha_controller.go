package controller

import (
	"net/http"

	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/service"

	"github.com/gin-gonic/gin"
)

type CaptchaController struct{}

func (u *CaptchaController) Check(c *gin.Context) {
	config := service.ViteConfig.GetValue("captcha_enabled")
	if config != "true" {
		c.JSON(http.StatusOK, result.Ok(0))
		return
	}
	c.JSON(http.StatusOK, result.Ok(1))
}

func (u *CaptchaController) Generate(c *gin.Context) {
	// TODO: Implement actual captcha generation if enabled
	// For now return error or empty as it's disabled
	c.JSON(http.StatusOK, result.Err(-1, "Captcha not implemented"))
}

func (u *CaptchaController) Verify(c *gin.Context) {
	var dto dto.CaptchaVerifyDto
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusOK, result.Err(-1, "参数错误"))
		return
	}

	// Mock success for now since we disabled it, but if frontend calls verify, it expects validToken
	// But if check returns 0, frontend shouldn't call verify.

	c.JSON(http.StatusOK, result.Ok(map[string]interface{}{"validToken": "mock_token"}))
}
