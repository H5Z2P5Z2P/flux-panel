package dto

// UserTunnelDto 用户隧道权限分配 DTO
type UserTunnelDto struct {
	UserId   int64 `json:"userId" binding:"required"`
	TunnelId int64 `json:"tunnelId" binding:"required"`
	SpeedId  int   `json:"speedId"` // 0表示不限速
}

// UserTunnelQueryDto 用户隧道查询 DTO
type UserTunnelQueryDto struct {
	UserId   *int64 `json:"userId"`
	TunnelId *int64 `json:"tunnelId"`
}

// UserTunnelUpdateDto 用户隧道更新 DTO
type UserTunnelUpdateDto struct {
	ID      int  `json:"id" binding:"required"`
	SpeedId int  `json:"speedId"` // 0表示不限速
	Status  *int `json:"status"`
}
