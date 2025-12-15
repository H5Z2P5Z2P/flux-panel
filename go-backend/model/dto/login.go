package dto

type LoginDto struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	CaptchaId string `json:"captchaId"`
}
