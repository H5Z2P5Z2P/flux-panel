package service

import (
	"fmt"
	"strings"
	"time"

	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/result"
	"go-backend/utils"
)

type SpeedLimitService struct{}

var SpeedLimit = new(SpeedLimitService)

// CreateSpeedLimit 创建限速规则
func (s *SpeedLimitService) CreateSpeedLimit(dto dto.SpeedLimitDto) *result.Result {
	// 1. 验证隧道
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, dto.TunnelId).Error; err != nil {
		return result.Err(-1, "指定的隧道不存在")
	}
	if tunnel.Name != dto.TunnelName {
		return result.Err(-1, "隧道名称与隧道ID不匹配")
	}

	// 2. 检查名称是否已存在
	var count int64
	global.DB.Model(&model.SpeedLimit{}).Where("name = ?", dto.Name).Count(&count)
	if count > 0 {
		return result.Err(-1, "限速规则名称已存在")
	}

	// 3. 创建实体
	speedLimit := model.SpeedLimit{
		Name:        dto.Name,
		Speed:       dto.Speed,
		TunnelId:    dto.TunnelId,
		TunnelName:  dto.TunnelName,
		Status:      1, // Active
		CreatedTime: time.Now().UnixMilli(),
		UpdatedTime: time.Now().UnixMilli(),
	}

	if err := global.DB.Create(&speedLimit).Error; err != nil {
		return result.Err(-1, "创建限速规则失败: "+err.Error())
	}

	// 4. Gost Sync
	if err := s.addGostLimiter(&speedLimit, &tunnel); err != nil {
		// Rollback
		speedLimit.Status = 0
		global.DB.Save(&speedLimit)
		// global.DB.Delete(&speedLimit) // Java implementation sets status to 0 instead of deleting?
		// Java: this.removeById(speedLimit.getId()); line 115
		// Java handleGostOperationFailure sets status to 0, BUT then removeById is cancalled?
		// Java code:
		// handleGostOperationFailure(speedLimit);
		// this.removeById(speedLimit.getId());
		// So it sets status 0 then deletes it? That seems redundant.
		// Let's just delete it to be clean or match Java exactly.
		// If I follow Java exactly: handleGostOperationFailure updates DB, then removeById deletes DB.
		// The result is the record is DELETED.
		global.DB.Delete(&speedLimit)
		return result.Err(-1, err.Error())
	}

	return result.Ok("限速规则创建成功")
}

// GetAllSpeedLimits 获取所有限速规则
func (s *SpeedLimitService) GetAllSpeedLimits() *result.Result {
	var speedLimits []model.SpeedLimit
	global.DB.Find(&speedLimits)
	return result.Ok(speedLimits)
}

// UpdateSpeedLimit 更新限速规则
func (s *SpeedLimitService) UpdateSpeedLimit(updateDto dto.SpeedLimitUpdateDto) *result.Result {
	var speedLimit model.SpeedLimit
	if err := global.DB.First(&speedLimit, updateDto.ID).Error; err != nil {
		return result.Err(-1, "限速规则不存在")
	}

	// 1. 验证隧道
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, updateDto.TunnelId).Error; err != nil {
		return result.Err(-1, "指定的隧道不存在")
	}
	if tunnel.Name != updateDto.TunnelName {
		return result.Err(-1, "隧道名称与隧道ID不匹配")
	}

	// 2. 检查名称冲突
	if updateDto.Name != speedLimit.Name {
		var count int64
		global.DB.Model(&model.SpeedLimit{}).Where("name = ? AND id != ?", updateDto.Name, updateDto.ID).Count(&count)
		if count > 0 {
			return result.Err(-1, "限速规则名称已存在")
		}
	}

	// 3. Update Properties
	speedLimit.Name = updateDto.Name
	speedLimit.Speed = updateDto.Speed
	speedLimit.TunnelId = updateDto.TunnelId
	speedLimit.TunnelName = updateDto.TunnelName
	speedLimit.UpdatedTime = time.Now().UnixMilli()

	// 4. Gost Sync
	if err := s.updateGostLimiter(&speedLimit, &tunnel); err != nil {
		return result.Err(-1, err.Error())
	}

	// 5. Save
	if err := global.DB.Save(&speedLimit).Error; err != nil {
		return result.Err(-1, "更新限速规则失败")
	}

	return result.Ok("限速规则更新成功")
}

// DeleteSpeedLimit 删除限速规则
func (s *SpeedLimitService) DeleteSpeedLimit(id int64) *result.Result {
	var speedLimit model.SpeedLimit
	if err := global.DB.First(&speedLimit, id).Error; err != nil {
		return result.Err(-1, "限速规则不存在")
	}

	// 1. Checks usage
	var count int64
	global.DB.Model(&model.UserTunnel{}).Where("speed_id = ?", id).Count(&count)
	if count > 0 {
		return result.Err(-1, "该限速规则还有用户在使用 请先取消分配")
	}

	// 2. Get Tunnel for Gost cleanup
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, speedLimit.TunnelId).Error; err == nil {
		// Only delete Gost limiter if tunnel exists
		s.deleteGostLimiter(id, &tunnel)
	}

	// 3. Delete DB
	if err := global.DB.Delete(&speedLimit).Error; err != nil {
		return result.Err(-1, "删除限速规则失败")
	}

	return result.Ok("限速规则删除成功")
}

// --- Private Helper Methods ---

func (s *SpeedLimitService) addGostLimiter(speedLimit *model.SpeedLimit, tunnel *model.Tunnel) error {
	speedMBps := fmt.Sprintf("%.1f", float64(speedLimit.Speed)/8.0)
	var node model.Node
	if err := global.DB.First(&node, tunnel.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}

	res := utils.AddLimiters(node.ID, speedLimit.ID, speedMBps)
	if res.Msg != "OK" {
		return fmt.Errorf(res.Msg)
	}
	return nil
}

func (s *SpeedLimitService) updateGostLimiter(speedLimit *model.SpeedLimit, tunnel *model.Tunnel) error {
	speedMBps := fmt.Sprintf("%.1f", float64(speedLimit.Speed)/8.0)
	var node model.Node
	if err := global.DB.First(&node, tunnel.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}

	res := utils.UpdateLimiters(node.ID, speedLimit.ID, speedMBps)
	if res.Msg != "OK" {
		if len(res.Msg) > 0 && (res.Msg == "not found" || strings.Contains(res.Msg, "not found")) {
			res = utils.AddLimiters(node.ID, speedLimit.ID, speedMBps)
			if res.Msg != "OK" {
				return fmt.Errorf(res.Msg)
			}
		} else {
			return fmt.Errorf(res.Msg)
		}
	}
	return nil
}

func (s *SpeedLimitService) deleteGostLimiter(speedId int64, tunnel *model.Tunnel) {
	var node model.Node
	if err := global.DB.First(&node, tunnel.InNodeId).Error; err != nil {
		return
	}
	utils.DeleteLimiters(node.ID, speedId)
}
