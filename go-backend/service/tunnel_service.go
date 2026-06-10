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

	"go-backend/websocket"
)

type TunnelService struct{}

var Tunnel = new(TunnelService)

// ... (Existing methods) ...

func (s *TunnelService) CreateTunnel(dto dto.TunnelDto) *result.Result {
	// 1. Verify Name
	var count int64
	global.DB.Model(&model.Tunnel{}).Where("name = ?", dto.Name).Count(&count)
	if count > 0 {
		return result.Err(-1, "隧道名称已存在")
	}

	// 2. Validate Type 2 params
	if dto.Type == 2 {
		if dto.OutNodeId == nil {
			return result.Err(-1, "出口节点不能为空")
		}
	}

	// 3. Validate InNode
	var inNode model.Node
	if err := global.DB.First(&inNode, dto.InNodeId).Error; err != nil {
		return result.Err(-1, "入口节点不存在")
	}

	tunnel := model.Tunnel{
		Name:          dto.Name,
		InNodeId:      dto.InNodeId,
		InIp:          inNode.Ip,
		Type:          dto.Type,
		Flow:          dto.Flow,
		TcpListenAddr: "0.0.0.0", // Default
		UdpListenAddr: "0.0.0.0", // Default
		InterfaceName: dto.InterfaceName,
	}
	if dto.TcpListenAddr != "" {
		tunnel.TcpListenAddr = dto.TcpListenAddr
	}
	if dto.UdpListenAddr != "" {
		tunnel.UdpListenAddr = dto.UdpListenAddr
	}

	// Traffic Ratio
	if dto.TrafficRatio.IsZero() {
		tunnel.TrafficRatio = 1.0
	} else {
		f, _ := dto.TrafficRatio.Float64()
		tunnel.TrafficRatio = f
	}

	// Protocol
	if dto.Type == 2 {
		if dto.Protocol == "" {
			return result.Err(-1, "协议类型必选")
		}
		tunnel.Protocol = dto.Protocol
	}

	// 4. Setup Out Node
	if dto.Type == 1 {
		tunnel.OutNodeId = dto.InNodeId
		tunnel.OutIp = inNode.ServerIp
	} else {
		if dto.InNodeId == *dto.OutNodeId {
			return result.Err(-1, "隧道转发模式下，入口和出口不能是同一个节点")
		}
		var outNode model.Node
		if err := global.DB.First(&outNode, *dto.OutNodeId).Error; err != nil {
			return result.Err(-1, "出口节点不存在")
		}
		tunnel.OutNodeId = *dto.OutNodeId
		tunnel.OutIp = outNode.ServerIp
	}

	// Defaults
	tunnel.Status = 1
	tunnel.CreatedTime = time.Now().UnixMilli()
	tunnel.UpdatedTime = time.Now().UnixMilli()

	// Type 2 隧道：分配共享出口端口
	if dto.Type == 2 {
		outPort, err := s.allocateTunnelOutPort(tunnel.OutNodeId, nil)
		if err != nil {
			return result.Err(-1, "出口端口分配失败: "+err.Error())
		}
		tunnel.OutPort = outPort
	}

	if err := global.DB.Create(&tunnel).Error; err != nil {
		return result.Err(-1, "隧道创建失败: "+err.Error())
	}

	// Type 2 隧道：创建共享 chain 和 relay service
	if tunnel.Type == 2 {
		if err := s.createTunnelSharedServices(&tunnel); err != nil {
			if isTransientNodeSyncError(err) {
				return result.OkMsg("隧道创建成功，节点恢复后将自动同步")
			}
			// 回滚：删除数据库记录
			global.DB.Delete(&tunnel)
			return result.Err(-1, "共享服务创建失败: "+err.Error())
		}
	}

	return result.Ok("隧道创建成功")
}

// UserTunnel 获取当前用户可用的隧道列表 (API: /api/v1/tunnel/user/tunnel)
func (s *TunnelService) UserTunnel(userId int64) *result.Result {
	var user model.User
	if err := global.DB.First(&user, userId).Error; err != nil {
		return result.Err(-1, "用户不存在")
	}

	var tunnels []model.Tunnel

	if user.RoleId == 0 { // Admin
		global.DB.Where("status = 1").Find(&tunnels)
	} else {
		// 1. Get User Permissions
		var userTunnels []model.UserTunnel
		global.DB.Where("user_id = ? AND status = 1", userId).Find(&userTunnels)

		for _, ut := range userTunnels {
			if ut.ExpTime > 0 && ut.ExpTime <= time.Now().UnixMilli() {
				continue // Expired
			}
			var t model.Tunnel
			// Check Tunnel Status
			if err := global.DB.Where("id = ? AND status = 1", ut.TunnelId).First(&t).Error; err == nil {
				tunnels = append(tunnels, t)
			}
		}
	}

	var response []dto.UserTunnelResponseDto
	for _, tunnel := range tunnels {
		var node model.Node
		if err := global.DB.First(&node, tunnel.InNodeId).Error; err != nil {
			continue
		}

		dto := dto.UserTunnelResponseDto{
			ID:               tunnel.ID,
			Name:             tunnel.Name,
			Ip:               tunnel.InIp,
			Type:             tunnel.Type,
			Protocol:         tunnel.Protocol,
			InNodePortRanges: node.PortRanges,
		}
		response = append(response, dto)
	}

	return result.Ok(response)
}

func (s *TunnelService) GetAllTunnels() *result.Result {
	var tunnels []model.Tunnel
	global.DB.Find(&tunnels)
	return result.Ok(tunnels)
}

func (s *TunnelService) UpdateTunnel(req dto.TunnelUpdateDto) *result.Result {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, req.ID).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}
	oldTunnel := tunnel

	var count int64
	global.DB.Model(&model.Tunnel{}).Where("name = ? AND id != ?", req.Name, req.ID).Count(&count)
	if count > 0 {
		return result.Err(-1, "隧道名称已存在")
	}

	newInNodeId := tunnel.InNodeId
	if req.InNodeId != nil {
		newInNodeId = *req.InNodeId
	}
	var inNode model.Node
	if err := global.DB.First(&inNode, newInNodeId).Error; err != nil {
		return result.Err(-1, "入口节点不存在")
	}

	newOutNodeId := tunnel.OutNodeId
	var outNode model.Node
	if tunnel.Type == 1 {
		newOutNodeId = newInNodeId
		outNode = inNode
	} else {
		if req.OutNodeId != nil {
			newOutNodeId = *req.OutNodeId
		}
		if newOutNodeId == 0 {
			return result.Err(-1, "出口节点不能为空")
		}
		if newInNodeId == newOutNodeId {
			return result.Err(-1, "隧道转发模式下，入口和出口不能是同一个节点")
		}
		if err := global.DB.First(&outNode, newOutNodeId).Error; err != nil {
			return result.Err(-1, "出口节点不存在")
		}
	}

	inNodeChanged := oldTunnel.InNodeId != newInNodeId
	outNodeChanged := oldTunnel.OutNodeId != newOutNodeId

	newOutPort := tunnel.OutPort
	if tunnel.Type == 2 && outNodeChanged {
		outPort, err := s.allocateTunnelOutPort(newOutNodeId, &tunnel.ID)
		if err != nil {
			return result.Err(-1, "出口端口分配失败: "+err.Error())
		}
		newOutPort = outPort
	}

	if inNodeChanged {
		var forwards []model.Forward
		global.DB.Where("tunnel_id = ? AND status = ?", tunnel.ID, 1).Find(&forwards)
		for _, forward := range forwards {
			if err := Forward.checkPortAvailable(newInNodeId, forward.InPort, &forward.ID); err != nil {
				return result.Err(-1, fmt.Sprintf("入口节点切换失败，转发 %s 的端口 %d 不可用: %s", forward.Name, forward.InPort, err.Error()))
			}
		}
	}

	criticalChange := inNodeChanged || outNodeChanged ||
		tunnel.TcpListenAddr != req.TcpListenAddr ||
		tunnel.UdpListenAddr != req.UdpListenAddr ||
		tunnel.Protocol != req.Protocol ||
		tunnel.InterfaceName != req.InterfaceName ||
		tunnel.OutPort != newOutPort

	tunnel.Name = req.Name
	tunnel.Flow = req.Flow
	tunnel.Protocol = req.Protocol
	tunnel.InterfaceName = req.InterfaceName
	tunnel.TcpListenAddr = req.TcpListenAddr
	tunnel.UdpListenAddr = req.UdpListenAddr
	tunnel.InNodeId = newInNodeId
	tunnel.InIp = inNode.Ip
	tunnel.OutNodeId = newOutNodeId
	tunnel.OutIp = outNode.ServerIp
	tunnel.OutPort = newOutPort
	if !req.TrafficRatio.IsZero() {
		f, _ := req.TrafficRatio.Float64()
		tunnel.TrafficRatio = f
	}
	tunnel.UpdatedTime = time.Now().UnixMilli()

	// 如果是 Type 2 隧道且有关键变更，先更新共享服务
	syncDeferred := false
	if tunnel.Type == 2 && criticalChange {
		if err := s.syncTunnelSharedServicesAfterUpdate(&oldTunnel, &tunnel); err != nil {
			if isTransientNodeSyncError(err) {
				syncDeferred = true
			} else {
				s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
				return result.Err(-1, "更新隧道共享服务失败: "+err.Error())
			}
		}
	}
	if inNodeChanged {
		if err := SpeedLimit.syncTunnelLimitersAfterEntryChange(&oldTunnel, &tunnel); err != nil {
			if isTransientNodeSyncError(err) {
				syncDeferred = true
			} else {
				s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
				return result.Err(-1, "同步限速配置失败: "+err.Error())
			}
		}
	}

	// Sync Forwards if needed
	if criticalChange {
		var forwards []model.Forward
		global.DB.Where("tunnel_id = ?", tunnel.ID).Find(&forwards)
		for _, f := range forwards {
			var userTunnel model.UserTunnel
			global.DB.Where("user_id = ? AND tunnel_id = ?", f.UserId, f.TunnelId).First(&userTunnel)
			var limiter *int
			if userTunnel.ID != 0 {
				limiter = &userTunnel.SpeedId
			}

			if inNodeChanged {
				if err := Forward.createGostServices(&f, &tunnel, limiter, &userTunnel); err != nil {
					if isTransientNodeSyncError(err) {
						syncDeferred = true
					} else {
						s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
						return result.Err(-1, fmt.Sprintf("隧道更新成功，但在新入口节点同步转发 %s 时失败: %s", f.Name, err.Error()))
					}
				}
				if f.Status != 1 {
					if err := Forward.pauseGostServices(&f, &tunnel, &userTunnel); err != nil {
						if isTransientNodeSyncError(err) {
							syncDeferred = true
						} else {
							s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
							return result.Err(-1, fmt.Sprintf("隧道更新成功，但在新入口节点暂停转发 %s 时失败: %s", f.Name, err.Error()))
						}
					}
				}
			} else {
				if err := Forward.updateGostServices(&f, &tunnel, limiter, &userTunnel); err != nil {
					if isTransientNodeSyncError(err) {
						syncDeferred = true
					} else {
						s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
						return result.Err(-1, fmt.Sprintf("隧道更新成功，但在同步转发 %s 时失败: %s", f.Name, err.Error()))
					}
				}
				if f.Status != 1 {
					if err := Forward.pauseGostServices(&f, &tunnel, &userTunnel); err != nil {
						if isTransientNodeSyncError(err) {
							syncDeferred = true
						} else {
							s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
							return result.Err(-1, fmt.Sprintf("隧道更新成功，但在同步转发 %s 后暂停失败: %s", f.Name, err.Error()))
						}
					}
				}
			}
		}
	}

	if err := global.DB.Save(&tunnel).Error; err != nil {
		s.cleanupFailedTunnelUpdate(&oldTunnel, &tunnel)
		return result.Err(-1, "隧道更新失败: "+err.Error())
	}

	if syncDeferred {
		return result.OkMsg("隧道更新成功，节点恢复后将自动同步")
	}

	s.cleanupOldTunnelConfigAfterSuccessfulUpdate(&oldTunnel, &tunnel)
	return result.Ok("隧道更新成功")
}
func (s *TunnelService) DiagnoseTunnel(tunnelId int64) *result.Result {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, tunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	var inNode model.Node
	if err := global.DB.First(&inNode, tunnel.InNodeId).Error; err != nil {
		return result.Err(-1, "入口节点不存在")
	}

	results := []map[string]interface{}{}

	if tunnel.Type == 1 {
		results = append(results, s.performForwardTargetPings(&inNode, tunnel.ID, "转发->目标")...)
		if len(results) == 0 {
			res := s.PerformTcpPing(&inNode, "www.google.com", 443, "入口->外网")
			results = append(results, res)
		}
	} else {
		// Tunnel Forward
		var outNode model.Node
		if err := global.DB.First(&outNode, tunnel.OutNodeId).Error; err != nil {
			return result.Err(-1, "出口节点不存在")
		}

		// In -> Out
		res1 := s.PerformTcpPing(&inNode, outNode.ServerIp, tunnel.OutPort, "入口->出口")
		results = append(results, res1)

		targetResults := s.performForwardTargetPings(&outNode, tunnel.ID, "出口->目标")
		if len(targetResults) == 0 {
			res2 := s.PerformTcpPing(&outNode, "www.google.com", 443, "出口->外网")
			targetResults = append(targetResults, res2)
		}
		results = append(results, targetResults...)
	}

	report := map[string]interface{}{
		"tunnelId":   tunnel.ID,
		"tunnelName": tunnel.Name,
		"tunnelType": "端口转发", // Default
		"results":    results,
		"timestamp":  time.Now().UnixMilli(),
	}
	if tunnel.Type == 2 {
		report["tunnelType"] = "隧道转发"
	}

	return result.Ok(report)
}

func (s *TunnelService) performForwardTargetPings(node *model.Node, tunnelId int64, desc string) []map[string]interface{} {
	if node == nil {
		return nil
	}

	var forwards []model.Forward
	global.DB.Where("tunnel_id = ? AND status = ?", tunnelId, 1).Find(&forwards)

	results := make([]map[string]interface{}, 0)
	seen := map[string]bool{}
	for _, forward := range forwards {
		for _, addr := range strings.Split(forward.RemoteAddr, ",") {
			addr = strings.TrimSpace(addr)
			if addr == "" || seen[addr] {
				continue
			}
			seen[addr] = true

			targetIp := utils.ExtractIp(addr)
			targetPort := utils.ExtractPort(addr)
			if targetIp == "" || targetPort == -1 {
				continue
			}
			results = append(results, s.PerformTcpPing(node, targetIp, targetPort, desc))
		}
	}
	return results
}

func (s *TunnelService) PerformTcpPing(node *model.Node, targetIp string, port int, desc string) map[string]interface{} {
	payload := map[string]interface{}{
		"ip":      targetIp,
		"port":    port,
		"count":   1,
		"timeout": 3000,
	}
	gostRes := websocket.SendMsg(node.ID, payload, "TcpPing")

	res := map[string]interface{}{
		"nodeId":      node.ID,
		"nodeName":    node.Name,
		"targetIp":    targetIp,
		"targetPort":  port,
		"description": desc,
		"success":     false,
		"message":     "节点无响应",
		"timestamp":   time.Now().UnixMilli(),
	}

	if gostRes != nil && gostRes.Msg == "OK" {
		res["success"] = true
		if gostRes.Data != nil {
			if dataMap, ok := gostRes.Data.(map[string]interface{}); ok {
				res["message"] = "TCP连接成功"
				res["averageTime"] = dataMap["averageTime"]
				res["packetLoss"] = dataMap["packetLoss"]
			} else {
				res["message"] = "解析响应失败"
			}
		} else {
			// Fallback simple success
			res["success"] = true
			res["message"] = "TCP连接成功"
			res["averageTime"] = 0.0
			res["packetLoss"] = 0.0
		}
	} else if gostRes != nil {
		res["message"] = gostRes.Msg
	}

	return res
}

func (s *TunnelService) getOutNodeTcpPort(tunnelId int64) int {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, tunnelId).Error; err == nil {
		return tunnel.OutPort
	}
	return 0
}

func (s *TunnelService) DeleteTunnel(id int64) *result.Result {
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, id).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	if count := Forward.CountForwardsByTunnelId(id); count > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个转发在使用，请先删除相关转发", count))
	}
	if count := UserTunnel.CountUserTunnelsByTunnelId(id); count > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个用户权限关联，请先取消用户权限分配", count))
	}

	// Type 2 隧道：删除共享服务
	if tunnel.Type == 2 {
		s.deleteTunnelSharedServices(&tunnel)
	}

	if err := global.DB.Delete(&model.Tunnel{}, id).Error; err != nil {
		return result.Err(-1, "隧道删除失败")
	}
	return result.Ok("隧道删除成功")
}

// --- Tunnel Type 2 共享服务管理 ---

// allocateTunnelOutPort 为 Type 2 隧道分配出口节点端口
func (s *TunnelService) allocateTunnelOutPort(outNodeId int64, excludeTunnelId *int64) (int, error) {
	var node model.Node
	if err := global.DB.First(&node, outNodeId).Error; err != nil {
		return 0, fmt.Errorf("出口节点不存在")
	}

	// 解析端口范围
	ranges, err := utils.ParsePortRanges(node.PortRanges)
	if err != nil {
		return 0, fmt.Errorf("出口节点端口配置错误: %s", err.Error())
	}
	allPorts := utils.GetAllPorts(ranges)
	used := s.getUsedTunnelOutPorts(outNodeId, excludeTunnelId)

	for _, p := range allPorts {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("出口节点无可用端口")
}

// getUsedTunnelOutPorts 获取出口节点已被占用的所有端口
func (s *TunnelService) getUsedTunnelOutPorts(outNodeId int64, excludeTunnelId *int64) map[int]bool {
	used := make(map[int]bool)

	// 1. Type 2 隧道的共享 relay service 端口
	var tunnels []model.Tunnel
	query := global.DB.Where("out_node_id = ? AND type = 2", outNodeId)
	if excludeTunnelId != nil {
		query = query.Where("id != ?", *excludeTunnelId)
	}
	query.Find(&tunnels)

	for _, t := range tunnels {
		if t.OutPort != 0 {
			used[t.OutPort] = true
		}
	}

	// 2. Forward 使用的出口端口（包括旧数据和 Type 1 转发）
	var forwardOutPorts []int
	global.DB.Model(&model.Forward{}).
		Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.out_node_id = ?", outNodeId).
		Pluck("forward.out_port", &forwardOutPorts)
	for _, p := range forwardOutPorts {
		if p != 0 {
			used[p] = true
		}
	}

	// 3. 同节点入口监听端口也会占用节点端口，避免共享 relay 和入口 service 抢端口
	var forwardInPorts []int
	global.DB.Model(&model.Forward{}).
		Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ?", outNodeId).
		Pluck("forward.in_port", &forwardInPorts)
	for _, p := range forwardInPorts {
		if p != 0 {
			used[p] = true
		}
	}

	return used
}

// createTunnelSharedServices 为 Type 2 隧道创建共享的 chain 和 relay service
func (s *TunnelService) createTunnelSharedServices(tunnel *model.Tunnel) error {
	// 获取入口和出口节点
	var inNode, outNode model.Node
	if err := global.DB.First(&inNode, tunnel.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}
	if err := global.DB.First(&outNode, tunnel.OutNodeId).Error; err != nil {
		return fmt.Errorf("出口节点不存在")
	}

	// 1. 在入口节点创建共享 chain
	var syncErr error
	if res := s.addTunnelChain(tunnel); res.Msg != "OK" {
		err := fmt.Errorf("创建共享 Chain 失败: %s", res.Msg)
		if !isTransientNodeSyncMessage(res.Msg) {
			return err
		}
		syncErr = err
	}

	// 2. 在出口节点创建共享 relay service
	if res := s.addTunnelRelayService(tunnel); res.Msg != "OK" {
		err := fmt.Errorf("创建共享 Relay Service 失败: %s", res.Msg)
		if !isTransientNodeSyncMessage(res.Msg) {
			// 回滚：删除已创建的 chain
			utils.DeleteTunnelChain(inNode.ID, tunnel.ID)
			return err
		}
		if syncErr == nil {
			syncErr = err
		}
	}

	return syncErr
}

// deleteTunnelSharedServices 删除 Type 2 隧道的共享 chain 和 relay service
func (s *TunnelService) deleteTunnelSharedServices(tunnel *model.Tunnel) error {
	var inNode, outNode model.Node
	global.DB.First(&inNode, tunnel.InNodeId)
	global.DB.First(&outNode, tunnel.OutNodeId)

	// 删除入口节点的共享 chain
	if inNode.ID != 0 {
		s.deleteTunnelChain(tunnel)
	}

	// 删除出口节点的共享 relay service
	if outNode.ID != 0 {
		s.deleteTunnelRelayService(tunnel)
	}

	return nil
}

func (s *TunnelService) ReconcileNodeSharedConfig(nodeId int64, config *dto.GostConfigDto) error {
	if config == nil {
		return nil
	}

	reportedChains := make(map[string]bool, len(config.Chains))
	for _, chain := range config.Chains {
		reportedChains[chain.Name] = true
	}
	reportedServices := make(map[string]bool, len(config.Services))
	for _, svc := range config.Services {
		reportedServices[svc.Name] = true
	}

	var inTunnels []model.Tunnel
	global.DB.Where("type = ? AND status = ? AND in_node_id = ?", 2, 1, nodeId).Find(&inTunnels)
	for _, tunnel := range inTunnels {
		chainName := utils.BuildTunnelChainName(tunnel.ID)
		if reportedChains[chainName] {
			continue
		}

		if res := s.addTunnelChain(&tunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
			return fmt.Errorf("修复共享 Chain 失败: %s", res.Msg)
		}
	}

	var outTunnels []model.Tunnel
	global.DB.Where("type = ? AND status = ? AND out_node_id = ?", 2, 1, nodeId).Find(&outTunnels)
	for _, tunnel := range outTunnels {
		serviceName := fmt.Sprintf("tunnel_%d_relay", tunnel.ID)
		if reportedServices[serviceName] {
			continue
		}

		if res := s.addTunnelRelayService(&tunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
			return fmt.Errorf("修复共享 Relay Service 失败: %s", res.Msg)
		}
	}

	return nil
}

// updateTunnelSharedServices 更新 Type 2 隧道的共享服务配置
func (s *TunnelService) updateTunnelSharedServices(tunnel *model.Tunnel) error {
	var inNode, outNode model.Node
	if err := global.DB.First(&inNode, tunnel.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}
	if err := global.DB.First(&outNode, tunnel.OutNodeId).Error; err != nil {
		return fmt.Errorf("出口节点不存在")
	}

	// 1. 更新入口节点的共享 chain
	var syncErr error
	if res := s.updateTunnelChain(tunnel); res.Msg != "OK" {
		err := fmt.Errorf("更新共享 Chain 失败: %s", res.Msg)
		if !isTransientNodeSyncMessage(res.Msg) {
			return err
		}
		syncErr = err
	}

	// 2. 更新出口节点的共享 relay service
	if res := s.updateTunnelRelayService(tunnel); res.Msg != "OK" {
		err := fmt.Errorf("更新共享 Relay Service 失败: %s", res.Msg)
		if !isTransientNodeSyncMessage(res.Msg) {
			return err
		}
		if syncErr == nil {
			syncErr = err
		}
	}

	return syncErr
}

func (s *TunnelService) syncTunnelSharedServicesAfterUpdate(oldTunnel *model.Tunnel, tunnel *model.Tunnel) error {
	if oldTunnel == nil || tunnel == nil {
		return nil
	}

	inNodeChanged := oldTunnel.InNodeId != tunnel.InNodeId
	outNodeChanged := oldTunnel.OutNodeId != tunnel.OutNodeId || oldTunnel.OutPort != tunnel.OutPort

	if !inNodeChanged && !outNodeChanged {
		return s.updateTunnelSharedServices(tunnel)
	}

	var syncErr error

	if outNodeChanged {
		if res := s.addTunnelRelayService(tunnel); res.Msg != "OK" {
			err := fmt.Errorf("创建共享 Relay Service 失败: %s", res.Msg)
			if !isTransientNodeSyncMessage(res.Msg) {
				return err
			}
			syncErr = err
		}
	} else {
		if res := s.updateTunnelRelayService(tunnel); res.Msg != "OK" {
			err := fmt.Errorf("更新共享 Relay Service 失败: %s", res.Msg)
			if !isTransientNodeSyncMessage(res.Msg) {
				return err
			}
			syncErr = err
		}
	}

	if inNodeChanged {
		if res := s.addTunnelChain(tunnel); res.Msg != "OK" {
			err := fmt.Errorf("创建共享 Chain 失败: %s", res.Msg)
			if !isTransientNodeSyncMessage(res.Msg) {
				return err
			}
			if syncErr == nil {
				syncErr = err
			}
		}
	} else {
		if res := s.updateTunnelChain(tunnel); res.Msg != "OK" {
			err := fmt.Errorf("更新共享 Chain 失败: %s", res.Msg)
			if !isTransientNodeSyncMessage(res.Msg) {
				return err
			}
			if syncErr == nil {
				syncErr = err
			}
		}
	}

	return syncErr
}

func (s *TunnelService) cleanupFailedTunnelUpdate(oldTunnel *model.Tunnel, tunnel *model.Tunnel) {
	if oldTunnel == nil || tunnel == nil {
		return
	}

	inNodeChanged := oldTunnel.InNodeId != tunnel.InNodeId
	outNodeChanged := oldTunnel.OutNodeId != tunnel.OutNodeId || oldTunnel.OutPort != tunnel.OutPort

	if inNodeChanged {
		SpeedLimit.cleanupNewTunnelLimitersAfterEntryChange(oldTunnel, tunnel)
	}

	if tunnel.Type == 2 && inNodeChanged {
		if res := s.deleteTunnelChain(tunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 回滚清理新共享 Chain 失败: %s\n", res.Msg)
		}
	} else if tunnel.Type == 2 {
		if res := s.updateTunnelChain(oldTunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 回滚恢复共享 Chain 失败: %s\n", res.Msg)
		}
	}
	if tunnel.Type == 2 && outNodeChanged {
		if res := s.deleteTunnelRelayService(tunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 回滚清理新共享 Relay Service 失败: %s\n", res.Msg)
		}
	} else if tunnel.Type == 2 {
		if res := s.updateTunnelRelayService(oldTunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 回滚恢复共享 Relay Service 失败: %s\n", res.Msg)
		}
	}

	var forwards []model.Forward
	global.DB.Where("tunnel_id = ?", oldTunnel.ID).Find(&forwards)
	for _, f := range forwards {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", f.UserId, f.TunnelId).First(&userTunnel)
		if inNodeChanged {
			if err := Forward.deleteGostServices(&f, tunnel, &userTunnel); err != nil && !isTransientNodeSyncError(err) && !isGostNotFoundError(err) {
				fmt.Printf("[TunnelUpdate] 回滚清理新入口节点转发 %d 失败: %v\n", f.ID, err)
			}
		} else {
			var limiter *int
			if userTunnel.ID != 0 {
				limiter = &userTunnel.SpeedId
			}
			if err := Forward.updateGostServices(&f, oldTunnel, limiter, &userTunnel); err != nil && !isTransientNodeSyncError(err) {
				fmt.Printf("[TunnelUpdate] 回滚恢复转发 %d 失败: %v\n", f.ID, err)
			}
			if f.Status != 1 {
				if err := Forward.pauseGostServices(&f, oldTunnel, &userTunnel); err != nil && !isTransientNodeSyncError(err) {
					fmt.Printf("[TunnelUpdate] 回滚恢复暂停转发 %d 失败: %v\n", f.ID, err)
				}
			}
		}
	}
}

func (s *TunnelService) cleanupOldTunnelConfigAfterSuccessfulUpdate(oldTunnel *model.Tunnel, tunnel *model.Tunnel) {
	if oldTunnel == nil || tunnel == nil {
		return
	}

	inNodeChanged := oldTunnel.InNodeId != tunnel.InNodeId
	outNodeChanged := oldTunnel.OutNodeId != tunnel.OutNodeId || oldTunnel.OutPort != tunnel.OutPort

	if tunnel.Type == 2 && inNodeChanged {
		if res := s.deleteTunnelChain(oldTunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 清理旧共享 Chain 失败: %s\n", res.Msg)
		}
	}
	if tunnel.Type == 2 && outNodeChanged {
		if res := s.deleteTunnelRelayService(oldTunnel); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
			fmt.Printf("[TunnelUpdate] 清理旧共享 Relay Service 失败: %s\n", res.Msg)
		}
	}

	if inNodeChanged {
		SpeedLimit.cleanupTunnelLimitersAfterEntryChange(oldTunnel, tunnel)
	}

	if !inNodeChanged {
		return
	}

	var forwards []model.Forward
	global.DB.Where("tunnel_id = ?", tunnel.ID).Find(&forwards)
	for _, f := range forwards {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", f.UserId, f.TunnelId).First(&userTunnel)
		if err := Forward.deleteGostServices(&f, oldTunnel, &userTunnel); err != nil && !isTransientNodeSyncError(err) && !isGostNotFoundError(err) {
			fmt.Printf("[TunnelUpdate] 清理旧入口节点转发 %d 失败: %v\n", f.ID, err)
		}
	}
}

func (s *TunnelService) tunnelRemoteAddr(tunnel *model.Tunnel) string {
	remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, tunnel.OutPort)
	if strings.Contains(tunnel.OutIp, ":") {
		remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, tunnel.OutPort)
	}
	return remoteAddr
}

func (s *TunnelService) addTunnelChain(tunnel *model.Tunnel) *dto.GostDto {
	return utils.AddTunnelChain(tunnel.InNodeId, tunnel.ID, s.tunnelRemoteAddr(tunnel), tunnel.Protocol, tunnel.InterfaceName)
}

func (s *TunnelService) updateTunnelChain(tunnel *model.Tunnel) *dto.GostDto {
	return utils.UpdateTunnelChain(tunnel.InNodeId, tunnel.ID, s.tunnelRemoteAddr(tunnel), tunnel.Protocol, tunnel.InterfaceName)
}

func (s *TunnelService) deleteTunnelChain(tunnel *model.Tunnel) *dto.GostDto {
	return utils.DeleteTunnelChain(tunnel.InNodeId, tunnel.ID)
}

func (s *TunnelService) addTunnelRelayService(tunnel *model.Tunnel) *dto.GostDto {
	return utils.AddTunnelRelayService(tunnel.OutNodeId, tunnel.ID, tunnel.OutPort, tunnel.Protocol, tunnel.InterfaceName)
}

func (s *TunnelService) updateTunnelRelayService(tunnel *model.Tunnel) *dto.GostDto {
	return utils.UpdateTunnelRelayService(tunnel.OutNodeId, tunnel.ID, tunnel.OutPort, tunnel.Protocol, tunnel.InterfaceName)
}

func (s *TunnelService) deleteTunnelRelayService(tunnel *model.Tunnel) *dto.GostDto {
	return utils.DeleteTunnelRelayService(tunnel.OutNodeId, tunnel.ID)
}
