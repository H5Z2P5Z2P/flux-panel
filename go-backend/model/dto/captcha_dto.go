package dto

type CaptchaVerifyDto struct {
	ID   string      `json:"id"`
	Data interface{} `json:"data"` // Generic for now, frontend sends ImageCaptchaTrack
}
