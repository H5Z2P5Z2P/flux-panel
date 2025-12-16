package dto

// SpeedLimitDto 限速规则创建 DTO
type SpeedLimitDto struct {
	Name       string `json:"name" binding:"required"`
	Speed      int    `json:"speed" binding:"required,min=1"`
	TunnelId   int64  `json:"tunnelId" binding:"required"`
	TunnelName string `json:"tunnelName" binding:"required"`
}

// SpeedLimitUpdateDto 限速规则更新 DTO
type SpeedLimitUpdateDto struct {
	ID         int64  `json:"id" binding:"required"`
	Name       string `json:"name" binding:"required"`
	Speed      int    `json:"speed" binding:"required,min=1"`
	TunnelId   int64  `json:"tunnelId" binding:"required"`
	TunnelName string `json:"tunnelName" binding:"required"`
}
