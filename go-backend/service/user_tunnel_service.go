package service

import (
	"go-backend/global"
	"go-backend/model"
)

type UserTunnelService struct{}

var UserTunnel = new(UserTunnelService)

func (s *UserTunnelService) CountUserTunnelsByTunnelId(tunnelId int64) int64 {
	var count int64
	global.DB.Model(&model.UserTunnel{}).Where("tunnel_id = ?", tunnelId).Count(&count)
	return count
}
