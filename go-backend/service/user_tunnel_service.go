package service

import (
	"fmt"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/utils"
)

type UserTunnelService struct{}

var UserTunnel = new(UserTunnelService)

// AssignUserTunnel 分配用户隧道权限
func (s *UserTunnelService) AssignUserTunnel(userTunnelDto dto.UserTunnelDto) *result.Result {
	// 检查权限是否已存在
	var count int64
	global.DB.Model(&model.UserTunnel{}).Where("user_id = ? AND tunnel_id = ?", userTunnelDto.UserId, userTunnelDto.TunnelId).Count(&count)
	if count > 0 {
		return result.Err(-1, "该用户已拥有此隧道权限")
	}

	// 创建权限记录
	userTunnel := model.UserTunnel{
		UserId:        int(userTunnelDto.UserId),
		TunnelId:      int(userTunnelDto.TunnelId),
		Flow:          userTunnelDto.Flow,
		Num:           userTunnelDto.Num,
		FlowResetTime: int64(userTunnelDto.FlowResetTime),
		ExpTime:       userTunnelDto.ExpTime,
		SpeedId:       userTunnelDto.SpeedId,
		Status:        1, // 默认启用
	}

	if err := global.DB.Create(&userTunnel).Error; err != nil {
		return result.Err(-1, "用户隧道权限分配失败: "+err.Error())
	}

	return result.Ok("用户隧道权限分配成功")
}

// GetUserTunnelList 获取用户隧道权限列表
func (s *UserTunnelService) GetUserTunnelList(queryDto dto.UserTunnelQueryDto) *result.Result {
	var userTunnels []model.UserTunnel
	query := global.DB.Model(&model.UserTunnel{})

	if queryDto.UserId != nil {
		query = query.Where("user_id = ?", *queryDto.UserId)
	}
	if queryDto.TunnelId != nil {
		query = query.Where("tunnel_id = ?", *queryDto.TunnelId)
	}

	query.Find(&userTunnels)

	// 扩展隧道信息
	type UserTunnelDetail struct {
		model.UserTunnel
		TunnelName string `json:"tunnelName"`
	}

	var details []UserTunnelDetail
	for _, ut := range userTunnels {
		var tunnel model.Tunnel
		global.DB.First(&tunnel, ut.TunnelId)
		details = append(details, UserTunnelDetail{
			UserTunnel: ut,
			TunnelName: tunnel.Name,
		})
	}

	return result.Ok(details)
}

// RemoveUserTunnel 删除用户隧道权限
func (s *UserTunnelService) RemoveUserTunnel(id int) *result.Result {
	var userTunnel model.UserTunnel
	if err := global.DB.First(&userTunnel, id).Error; err != nil {
		return result.Err(-1, "未找到对应的用户隧道权限记录")
	}

	// 删除该用户在该隧道下的所有转发
	s.removeUserTunnelForwards(int64(userTunnel.UserId), int64(userTunnel.TunnelId))

	// 删除权限记录
	if err := global.DB.Delete(&userTunnel).Error; err != nil {
		return result.Err(-1, "用户隧道权限删除失败")
	}

	return result.Ok("用户隧道权限删除成功")
}

// UpdateUserTunnel 更新用户隧道权限
func (s *UserTunnelService) UpdateUserTunnel(updateDto dto.UserTunnelUpdateDto) *result.Result {
	var userTunnel model.UserTunnel
	if err := global.DB.First(&userTunnel, updateDto.ID).Error; err != nil {
		return result.Err(-1, "用户隧道权限不存在")
	}

	// 检查限速是否变化
	oldSpeedId := userTunnel.SpeedId
	speedChanged := (oldSpeedId != updateDto.SpeedId)

	// 更新属性
	userTunnel.Flow = updateDto.Flow
	userTunnel.Num = updateDto.Num
	userTunnel.FlowResetTime = int64(updateDto.FlowResetTime)
	userTunnel.ExpTime = updateDto.ExpTime
	userTunnel.SpeedId = updateDto.SpeedId
	if updateDto.Status != nil {
		userTunnel.Status = *updateDto.Status
	}

	if err := global.DB.Save(&userTunnel).Error; err != nil {
		return result.Err(-1, "用户隧道权限更新失败")
	}

	// 如果限速变化，更新所有转发的限速
	if speedChanged {
		s.updateUserTunnelForwardsSpeed(int64(userTunnel.UserId), int64(userTunnel.TunnelId), updateDto.SpeedId)
	}

	return result.Ok("用户隧道权限更新成功")
}

// CountUserTunnelsByTunnelId 统计隧道的用户权限数量
func (s *UserTunnelService) CountUserTunnelsByTunnelId(tunnelId int64) int64 {
	var count int64
	global.DB.Model(&model.UserTunnel{}).Where("tunnel_id = ?", tunnelId).Count(&count)
	return count
}

// --- Private Helper Methods ---

// removeUserTunnelForwards 删除用户在指定隧道下的所有转发
func (s *UserTunnelService) removeUserTunnelForwards(userId int64, tunnelId int64) {
	var forwards []model.Forward
	global.DB.Where("user_id = ? AND tunnel_id = ?", userId, tunnelId).Find(&forwards)

	if len(forwards) == 0 {
		return
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", userId, tunnelId).First(&userTunnel)

	for _, forward := range forwards {
		s.stopForwardService(&forward, userId, userTunnel.ID)
		global.DB.Delete(&forward)
	}
}

// stopForwardService 停止转发服务
func (s *UserTunnelService) stopForwardService(forward *model.Forward, userId int64, userTunnelId int) {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return
	}

	serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, userId, userTunnelId)

	// 删除主服务
	utils.DeleteService(tunnel.InNodeId, serviceName)

	// 如果是隧道转发，删除远程服务和链
	if tunnel.Type == 2 {
		utils.DeleteRemoteService(tunnel.OutNodeId, serviceName)
		utils.DeleteChains(tunnel.InNodeId, serviceName)
	}
}

// updateUserTunnelForwardsSpeed 更新用户隧道下所有转发的限速
func (s *UserTunnelService) updateUserTunnelForwardsSpeed(userId int64, tunnelId int64, speedId int) {
	var forwards []model.Forward
	global.DB.Where("user_id = ? AND tunnel_id = ?", userId, tunnelId).Find(&forwards)

	if len(forwards) == 0 {
		return
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, tunnelId).Error; err != nil {
		return
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", userId, tunnelId).First(&userTunnel)

	for _, forward := range forwards {
		serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, userId, userTunnel.ID)
		interfaceName := ""
		if tunnel.Type != 2 {
			interfaceName = forward.InterfaceName
		}
		speedIdPtr := &speedId
		if speedId == 0 {
			speedIdPtr = nil
		}
		utils.UpdateService(tunnel.InNodeId, serviceName, forward.InPort, speedIdPtr, forward.RemoteAddr, tunnel.Type, tunnel, forward.Strategy, interfaceName)
	}
}
