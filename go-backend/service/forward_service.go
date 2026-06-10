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

type ForwardService struct {
	SkipGostSync bool
}

var Forward = new(ForwardService)

// --- Public Methods ---

func (s *ForwardService) CreateForward(dto dto.ForwardDto, ctxUser *utils.UserClaims) *result.Result {
	// 1. Check Tunnel
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, dto.TunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}
	if tunnel.Status != 1 {
		return result.Err(-1, "隧道已禁用")
	}

	// Determine Target User
	var targetUserId int64
	var targetUserName string
	var targetUserRole int

	if ctxUser.RoleId == 0 && dto.UserId != nil {
		// Admin creating for specific user
		targetUserId = *dto.UserId
		var targetUser model.User
		if err := global.DB.First(&targetUser, targetUserId).Error; err != nil {
			return result.Err(-1, "指定用户不存在")
		}
		targetUserName = targetUser.User
		targetUserRole = targetUser.RoleId
	} else {
		// Self creation
		targetUserId = ctxUser.GetUserId()
		targetUserName = ctxUser.User
		targetUserRole = ctxUser.RoleId
	}

	// 2. Permissions & Limits
	var limiter *int
	var userTunnel *model.UserTunnel

	// Check limits if target user is not Admin (RoleId != 0)
	// Even if actor is Admin, if they are creating for a normal user, user limits apply (or should they?)
	// Usually Admin can override, but for "Managing User's Forwards", we probably want to ensure consistency with User's limits or Tunnels.
	// But critically, we must check if User has permission for the Tunnel.

	if targetUserRole != 0 {
		// A. Check User Limits (Global)
		var user model.User
		if err := global.DB.First(&user, targetUserId).Error; err != nil {
			return result.Err(-1, "用户异常")
		}
		now := time.Now().UnixMilli()
		if reason := userRuntimeBlockReason(&user, now); reason != "" {
			return result.Err(-1, reason)
		}

		// Check Forward Num Limit (Global)
		if user.Num > 0 {
			var currentCount int64
			global.DB.Model(&model.Forward{}).Where("user_id = ?", targetUserId).Count(&currentCount)
			if int(currentCount) >= user.Num {
				return result.Err(-1, fmt.Sprintf("转发数量已达上限(%d个)", user.Num))
			}
		}

		// B. Check Tunnel Permission
		var ut model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", targetUserId, dto.TunnelId).First(&ut).Error; err != nil {
			// If Admin is operating, maybe we allow assigning to ANY tunnel?
			// But the prompt says "managing user's port forwarding".
			// If we assign a forward on a tunnel the user DOESN'T have access to, it breaks the model (UserTunnel link needed for speed limit etc).
			// So we should enforce UserTunnel existence.
			return result.Err(-1, "该用户没有该隧道权限")
		}
		if reason := userTunnelRuntimeBlockReason(&ut, now); reason != "" {
			return result.Err(-1, reason)
		}
		if ut.Num > 0 {
			var currentTunnelCount int64
			global.DB.Model(&model.Forward{}).Where("user_id = ? AND tunnel_id = ?", targetUserId, dto.TunnelId).Count(&currentTunnelCount)
			if int(currentTunnelCount) >= ut.Num {
				return result.Err(-1, fmt.Sprintf("该隧道转发数量已达上限(%d个)", ut.Num))
			}
		}

		userTunnel = &ut
		limiter = &ut.SpeedId
	} else {
		// Target is Admin (or Admin creating for themselves) - No limits
	}

	// 3. Allocate Port
	portAlloc, err := s.allocatePorts(&tunnel, dto.InPort, nil)
	if err != nil {
		return result.Err(-1, err.Error())
	}

	// 3.5 检查端口自环（防止远端地址指向入口端口导致崩溃）
	if err := s.checkLoopbackAddress(dto.RemoteAddr, &tunnel, portAlloc.InPort); err != nil {
		return result.Err(-1, err.Error())
	}

	// 4. Create Entity
	forward := model.Forward{
		UserId:        targetUserId,
		UserName:      targetUserName,
		Name:          dto.Name,
		TunnelId:      dto.TunnelId,
		InPort:        portAlloc.InPort,
		OutPort:       portAlloc.OutPort, // For Tunnel Forward
		RemoteAddr:    dto.RemoteAddr,
		InterfaceName: dto.InterfaceName,
		Strategy:      dto.Strategy,
		Status:        1,
		CreatedTime:   time.Now().UnixMilli(),
		UpdatedTime:   time.Now().UnixMilli(),
	}

	// 5. Save to DB
	if err := global.DB.Create(&forward).Error; err != nil {
		return result.Err(-1, "转发创建失败: "+err.Error())
	}

	// 6. Gost Sync
	if !s.SkipGostSync {
		if err := s.createGostServices(&forward, &tunnel, limiter, userTunnel); err != nil {
			if isTransientNodeSyncError(err) {
				return result.OkMsg("端口转发创建成功，节点恢复后将自动同步")
			}
			global.DB.Delete(&forward) // Rollback
			return result.Err(-1, "Gost服务创建失败: "+err.Error())
		}
	}

	return result.Ok("端口转发创建成功")
}

func (s *ForwardService) UpdateForward(id int64, dto dto.ForwardDto, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	// Permission Check
	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权修改此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, dto.TunnelId).Error; err != nil {
		return result.Err(-1, "新隧道不存在")
	}
	if tunnel.Status != 1 {
		return result.Err(-1, "新隧道已禁用")
	}

	// Check if Tunnel Changed
	tunnelChanged := forward.TunnelId != dto.TunnelId

	var owner model.User
	if err := global.DB.First(&owner, forward.UserId).Error; err != nil {
		return result.Err(-1, "用户异常")
	}
	if owner.RoleId != 0 {
		now := time.Now().UnixMilli()
		if reason := userRuntimeBlockReason(&owner, now); reason != "" {
			return result.Err(-1, reason)
		}
		var ownerTunnel model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, dto.TunnelId).First(&ownerTunnel).Error; err != nil {
			return result.Err(-1, "该用户没有该隧道权限")
		}
		if reason := userTunnelRuntimeBlockReason(&ownerTunnel, now); reason != "" {
			return result.Err(-1, reason)
		}
		if tunnelChanged && ownerTunnel.Num > 0 {
			var currentTunnelCount int64
			global.DB.Model(&model.Forward{}).Where("user_id = ? AND tunnel_id = ?", forward.UserId, dto.TunnelId).Count(&currentTunnelCount)
			if int(currentTunnelCount) >= ownerTunnel.Num {
				return result.Err(-1, fmt.Sprintf("该隧道转发数量已达上限(%d个)", ownerTunnel.Num))
			}
		}
	}

	// 获取转发所属用户的新隧道权限信息（用于 Gost 服务创建）
	// 注意：这里使用 forward.UserId，确保管理员修改时也能正确获取目标用户的信息
	var userTunnel *model.UserTunnel
	var limiter *int
	var newUT model.UserTunnel
	if err := global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, dto.TunnelId).First(&newUT).Error; err == nil {
		userTunnel = &newUT
		limiter = &newUT.SpeedId
	}

	// Update Port Allocation if needed
	var portAlloc *PortAllocResult
	var err error
	if tunnelChanged || (dto.InPort != nil && forward.InPort != *dto.InPort) { // If InPort changed user-side or tunnel changed
		portAlloc, err = s.allocatePorts(&tunnel, dto.InPort, &id)
		if err != nil {
			return result.Err(-1, err.Error())
		}
	} else {
		portAlloc = &PortAllocResult{InPort: forward.InPort, OutPort: forward.OutPort}
	}

	// 检查端口自环（防止远端地址指向入口端口导致崩溃）
	if err := s.checkLoopbackAddress(dto.RemoteAddr, &tunnel, portAlloc.InPort); err != nil {
		return result.Err(-1, err.Error())
	}

	// Update Entity Wrapper (Pre-save for Gost)
	updatedForward := forward
	updatedForward.Name = dto.Name
	updatedForward.TunnelId = dto.TunnelId
	updatedForward.InPort = portAlloc.InPort
	updatedForward.OutPort = portAlloc.OutPort
	updatedForward.RemoteAddr = dto.RemoteAddr
	updatedForward.InterfaceName = dto.InterfaceName
	updatedForward.Strategy = dto.Strategy
	updatedForward.UpdatedTime = time.Now().UnixMilli()
	updatedForward.Status = forward.Status

	syncDeferred := false

	// Gost Sync - 根据入口节点是否相同采用不同策略
	if tunnelChanged {
		// 获取旧隧道信息
		var oldTunnel model.Tunnel
		global.DB.First(&oldTunnel, forward.TunnelId)
		var oldUT model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, oldTunnel.ID).First(&oldUT)

		if oldTunnel.InNodeId == tunnel.InNodeId {
			// 入口节点相同：必须先删后创（否则监听同一端口会冲突）
			// 风险：如果新服务创建失败，需要尝试恢复旧服务
			deleteErr := s.deleteGostServices(&forward, &oldTunnel, &oldUT)
			if deleteErr != nil && !isTransientNodeSyncError(deleteErr) && !isGostNotFoundError(deleteErr) {
				return result.Err(-1, "旧服务删除失败: "+deleteErr.Error())
			}

			if err := s.createGostServices(&updatedForward, &tunnel, limiter, userTunnel); err != nil {
				if isTransientNodeSyncError(err) {
					syncDeferred = true
				} else {
					// 创建失败，尝试恢复旧服务
					var oldLimiter *int
					if oldUT.ID != 0 {
						oldLimiter = &oldUT.SpeedId
					}
					restoreErr := s.createGostServices(&forward, &oldTunnel, oldLimiter, &oldUT)
					if restoreErr != nil && !isTransientNodeSyncError(restoreErr) {
						return result.Err(-1, "新服务创建失败且无法恢复旧服务: "+err.Error()+"; 恢复错误: "+restoreErr.Error())
					}
					if forward.Status != 1 {
						pauseErr := s.pauseGostServices(&forward, &oldTunnel, &oldUT)
						if pauseErr != nil && !isTransientNodeSyncError(pauseErr) {
							return result.Err(-1, "新服务创建失败且旧服务恢复后无法暂停: "+err.Error()+"; 暂停错误: "+pauseErr.Error())
						}
					}
					return result.Err(-1, "新服务创建失败(已恢复旧服务): "+err.Error())
				}
			}
		} else {
			// 入口节点不同：先创后删（确保修改失败时旧服务仍可用）
			if err := s.createGostServices(&updatedForward, &tunnel, limiter, userTunnel); err != nil {
				if isTransientNodeSyncError(err) {
					syncDeferred = true
				} else {
					return result.Err(-1, "新隧道服务创建失败: "+err.Error())
				}
			}
			// 新服务创建成功后，删除旧服务（删除失败不致命）
			s.deleteGostServices(&forward, &oldTunnel, &oldUT)
		}
	} else {
		// Update Same Tunnel
		if err := s.updateGostServices(&updatedForward, &tunnel, limiter, userTunnel); err != nil {
			if isTransientNodeSyncError(err) {
				syncDeferred = true
			} else {
				return result.Err(-1, "Gost服务更新失败: "+err.Error())
			}
		}
	}

	// Save to DB
	// Use map to update specific fields to avoid zero values if any
	if err := global.DB.Model(&forward).Updates(map[string]interface{}{
		"name":           updatedForward.Name,
		"tunnel_id":      updatedForward.TunnelId,
		"in_port":        updatedForward.InPort,
		"out_port":       updatedForward.OutPort,
		"remote_addr":    updatedForward.RemoteAddr,
		"interface_name": updatedForward.InterfaceName,
		"strategy":       updatedForward.Strategy,
		"updated_time":   updatedForward.UpdatedTime,
	}).Error; err != nil {
		return result.Err(-1, "转发更新失败: "+err.Error())
	}

	if syncDeferred {
		return result.OkMsg("端口转发更新成功，节点恢复后将自动同步")
	}
	if forward.Status != 1 {
		if err := s.pauseGostServices(&updatedForward, &tunnel, userTunnel); err != nil {
			if isTransientNodeSyncError(err) {
				return result.OkMsg("端口转发更新成功，节点恢复后将自动同步")
			}
			return result.Err(-1, "Gost服务暂停失败: "+err.Error())
		}
	}
	return result.Ok("端口转发更新成功")
}

func (s *ForwardService) DeleteForward(id int64, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	// Permission Check
	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权删除此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		// If tunnel deleted, still delete forward from DB but skip Gost
		global.DB.Delete(&forward)
		return result.Ok("转发已删除")
	}

	// Get UserTunnel for identifying service name
	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)

	// Delete Gost Service
	if err := s.deleteGostServices(&forward, &tunnel, &userTunnel); err != nil {
		if isTransientNodeSyncError(err) || isGostNotFoundError(err) {
			global.DB.Delete(&forward)
			return result.Ok("删除成功")
		}
		return result.Err(-1, "Gost服务删除失败: "+err.Error())
	}

	global.DB.Delete(&forward)
	return result.Ok("删除成功")
}

func (s *ForwardService) ReconcileNodeConfig(nodeId int64, config *dto.GostConfigDto) error {
	if config == nil {
		return nil
	}

	reportedServices := make(map[string]dto.GostService, len(config.Services))
	for _, svc := range config.Services {
		reportedServices[svc.Name] = svc
	}

	var forwards []model.Forward
	if err := global.DB.Joins("JOIN tunnel ON forward.tunnel_id = tunnel.id").
		Where("tunnel.in_node_id = ?", nodeId).
		Find(&forwards).Error; err != nil {
		return err
	}

	for _, forward := range forwards {
		var tunnel model.Tunnel
		if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
			continue
		}

		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)

		serviceName := s.buildServiceName(forward.ID, forward.UserId, &userTunnel)
		var limiter *int
		if userTunnel.ID != 0 {
			limiter = &userTunnel.SpeedId
		}

		tcpSvc, hasTCP := reportedServices[serviceName+"_tcp"]
		udpSvc, hasUDP := reportedServices[serviceName+"_udp"]
		shouldRun := forward.Status == 1 && tunnel.Status == 1
		if !hasTCP || !hasUDP {
			partialServices := existingForwardServiceNames(serviceName, hasTCP, hasUDP)
			if !shouldRun {
				if len(partialServices) > 0 {
					if res := utils.PauseServiceNames(tunnel.InNodeId, partialServices); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
						return fmt.Errorf("修复半残留暂停服务失败: %s", res.Msg)
					}
				}
				continue
			}
			if len(partialServices) > 0 {
				if res := utils.DeleteServiceNames(tunnel.InNodeId, partialServices); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
					return fmt.Errorf("清理半残留服务失败: %s", res.Msg)
				}
			}
			if err := s.createGostServices(&forward, &tunnel, limiter, &userTunnel); err != nil && !isTransientNodeSyncError(err) {
				return err
			}
			continue
		}

		if !shouldRun {
			if !isGostServicePaused(tcpSvc) || !isGostServicePaused(udpSvc) {
				if res := utils.PauseService(tunnel.InNodeId, serviceName); res.Msg != "OK" && !isTransientNodeSyncMessage(res.Msg) {
					return fmt.Errorf("修复暂停服务失败: %s", res.Msg)
				}
			}
			continue
		}

		if isGostServicePaused(tcpSvc) || isGostServicePaused(udpSvc) {
			if res := utils.ResumeService(tunnel.InNodeId, serviceName); res.Msg != "OK" {
				if isGostNotFoundMessage(res.Msg) {
					if err := s.createGostServices(&forward, &tunnel, limiter, &userTunnel); err != nil && !isTransientNodeSyncError(err) {
						return err
					}
				} else if !isTransientNodeSyncMessage(res.Msg) {
					return fmt.Errorf("修复恢复服务失败: %s", res.Msg)
				}
			}
		}
	}

	return nil
}

func existingForwardServiceNames(serviceName string, hasTCP bool, hasUDP bool) []string {
	var services []string
	if hasTCP {
		services = append(services, serviceName+"_tcp")
	}
	if hasUDP {
		services = append(services, serviceName+"_udp")
	}
	return services
}

func isGostServicePaused(svc dto.GostService) bool {
	if svc.Metadata == nil {
		return false
	}
	paused, ok := svc.Metadata["paused"]
	if !ok {
		return false
	}
	switch v := paused.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func (s *ForwardService) GetAllForwards(ctxUser *utils.UserClaims) *result.Result {
	var forwards []model.Forward
	tx := global.DB.Model(&model.Forward{})
	if ctxUser.RoleId != 0 {
		tx = tx.Where("user_id = ?", ctxUser.GetUserId())
	}
	tx.Find(&forwards)

	var response []dto.ForwardResponseDto
	for _, f := range forwards {
		// Fetch Tunnel info
		var tunnel model.Tunnel
		var inIp string
		var tunnelName string
		if err := global.DB.First(&tunnel, f.TunnelId).Error; err == nil {
			tunnelName = tunnel.Name
			inIp = tunnel.InIp
		}

		resDto := dto.ForwardResponseDto{
			ID:            f.ID,
			Name:          f.Name,
			InPort:        f.InPort,
			RemoteAddr:    f.RemoteAddr,
			Status:        f.Status,
			CreatedTime:   f.CreatedTime,
			UpdatedTime:   f.UpdatedTime,
			TunnelName:    tunnelName,
			InIp:          inIp,
			UserName:      f.UserName,
			UserId:        f.UserId,
			TunnelId:      f.TunnelId,
			InFlow:        f.InFlow,
			OutFlow:       f.OutFlow,
			Strategy:      f.Strategy,
			Inx:           f.Inx,
			InterfaceName: f.InterfaceName,
		}
		response = append(response, resDto)
	}

	return result.Ok(response)
}

// --- Gost Integration Logic ---

func (s *ForwardService) createGostServices(forward *model.Forward, tunnel *model.Tunnel, limiter *int, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, _, err := s.getRequiredNodes(tunnel)
	if err != nil {
		return err
	}

	// Type 2 现在使用 tunnel 级别的共享 chain 和 relay service
	// 不再为每个 forward 单独创建 chain 和 remote service

	interfaceName := ""
	if tunnel.Type == 1 {
		interfaceName = forward.InterfaceName
	}

	if res := utils.AddService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName); res.Msg != "OK" {
		return fmt.Errorf("Service Error: %s", res.Msg)
	}
	return nil
}

func (s *ForwardService) updateGostServices(forward *model.Forward, tunnel *model.Tunnel, limiter *int, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, _, err := s.getRequiredNodes(tunnel)
	if err != nil {
		return err
	}

	// Type 2 现在使用 tunnel 级别的共享 chain 和 relay service
	// 不再为每个 forward 单独更新 chain 和 remote service

	interfaceName := ""
	if tunnel.Type == 1 {
		interfaceName = forward.InterfaceName
	}

	res := utils.UpdateService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName)
	if res.Msg != "OK" {
		if strings.Contains(res.Msg, "not found") {
			addRes := utils.AddService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName)
			if addRes.Msg != "OK" {
				return fmt.Errorf("Add Service Error: %s", addRes.Msg)
			}
		} else {
			return fmt.Errorf("Update Service Error: %s", res.Msg)
		}
	}
	return nil
}

func (s *ForwardService) pauseGostServices(forward *model.Forward, tunnel *model.Tunnel, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, _, err := s.getRequiredNodes(tunnel)
	if err != nil {
		return err
	}
	if res := utils.PauseService(inNode.ID, serviceName); res.Msg != "OK" {
		if isGostNotFoundMessage(res.Msg) {
			return nil
		}
		return fmt.Errorf("%s", res.Msg)
	}
	return nil
}

func (s *ForwardService) deleteGostServices(forward *model.Forward, tunnel *model.Tunnel, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, _, _ := s.getRequiredNodes(tunnel)

	// 只删除入口节点的 service
	// Type 2 的共享 chain 和 relay service 由 tunnel 删除时清理
	if inNode != nil {
		res := utils.DeleteService(inNode.ID, serviceName)
		if res.Msg != "OK" {
			return fmt.Errorf("%s", res.Msg)
		}
	}

	return nil
}

// --- Helpers ---

type PortAllocResult struct {
	InPort  int
	OutPort int
}

func (s *ForwardService) allocatePorts(tunnel *model.Tunnel, specifiedInPort *int, excludeForwardId *int64) (*PortAllocResult, error) {
	// Allocate InPort
	var inPort int
	if specifiedInPort != nil {
		if err := s.checkPortAvailable(tunnel.InNodeId, *specifiedInPort, excludeForwardId); err != nil {
			return nil, err
		}
		inPort = *specifiedInPort
	} else {
		p, err := s.findFreePort(tunnel.InNodeId, excludeForwardId)
		if err != nil {
			return nil, fmt.Errorf("入口节点无可用端口")
		}
		inPort = p
	}

	// OutPort 处理
	var outPort int
	if tunnel.Type == 2 {
		// Type 2 隧道转发：使用 tunnel 级别的共享 OutPort
		// 不再为每个 forward 单独分配端口
		outPort = tunnel.OutPort
	} else {
		// Port Forward: OutPort same as InPort (or irrelevant)
		outPort = inPort
	}

	return &PortAllocResult{InPort: inPort, OutPort: outPort}, nil
}

func (s *ForwardService) checkPortAvailable(nodeId int64, port int, excludeForwardId *int64) error {
	var node model.Node
	if err := global.DB.First(&node, nodeId).Error; err != nil {
		return fmt.Errorf("节点不存在")
	}
	// 解析端口范围
	ranges, err := utils.ParsePortRanges(node.PortRanges)
	if err != nil {
		return fmt.Errorf("节点端口配置错误: %s", err.Error())
	}
	if !utils.IsPortInRanges(port, ranges) {
		return fmt.Errorf("端口不在允许范围内")
	}
	if s.isPortUsed(nodeId, port, excludeForwardId) {
		return fmt.Errorf("端口 %d 已被占用", port)
	}
	return nil
}

func (s *ForwardService) findFreePort(nodeId int64, excludeForwardId *int64) (int, error) {
	var node model.Node
	if err := global.DB.First(&node, nodeId).Error; err != nil {
		return 0, err
	}
	// 解析端口范围
	ranges, err := utils.ParsePortRanges(node.PortRanges)
	if err != nil {
		return 0, fmt.Errorf("节点端口配置错误: %s", err.Error())
	}
	allPorts := utils.GetAllPorts(ranges)
	used := s.getUsedPorts(nodeId, excludeForwardId)
	for _, p := range allPorts {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("无可用端口")
}

func (s *ForwardService) getUsedPorts(nodeId int64, excludeForwardId *int64) map[int]bool {
	used := make(map[int]bool)
	// 1. InTunnels -> Forwards (InPort)
	var inTunnels []int64
	global.DB.Model(&model.Tunnel{}).Where("in_node_id = ?", nodeId).Pluck("id", &inTunnels)
	if len(inTunnels) > 0 {
		var forwards []model.Forward
		query := global.DB.Where("tunnel_id IN ?", inTunnels)
		if excludeForwardId != nil {
			query = query.Where("id != ?", *excludeForwardId)
		}
		query.Find(&forwards)
		for _, f := range forwards {
			used[f.InPort] = true
		}
	}

	// 2. OutTunnels -> Forwards (OutPort)
	var outTunnels []int64
	global.DB.Model(&model.Tunnel{}).Where("out_node_id = ?", nodeId).Pluck("id", &outTunnels)
	if len(outTunnels) > 0 {
		var forwards []model.Forward
		query := global.DB.Where("tunnel_id IN ?", outTunnels)
		if excludeForwardId != nil {
			query = query.Where("id != ?", *excludeForwardId)
		}
		query.Find(&forwards)
		for _, f := range forwards {
			if f.OutPort != 0 {
				used[f.OutPort] = true
			}
		}
	}

	// 3. Type 2 Tunnels 的共享 relay service 端口（出口节点）
	var type2Tunnels []model.Tunnel
	global.DB.Where("out_node_id = ? AND type = 2", nodeId).Find(&type2Tunnels)
	for _, t := range type2Tunnels {
		if t.OutPort != 0 {
			used[t.OutPort] = true
		}
	}

	return used
}

func (s *ForwardService) isPortUsed(nodeId int64, port int, excludeForwardId *int64) bool {
	used := s.getUsedPorts(nodeId, excludeForwardId)
	return used[port]
}

func (s *ForwardService) getRequiredNodes(tunnel *model.Tunnel) (*model.Node, *model.Node, error) {
	var inNode model.Node
	if err := global.DB.First(&inNode, tunnel.InNodeId).Error; err != nil {
		return nil, nil, fmt.Errorf("入口节点不存在")
	}
	var outNode *model.Node
	if tunnel.Type == 2 {
		var node model.Node
		if err := global.DB.First(&node, tunnel.OutNodeId).Error; err != nil {
			return nil, nil, fmt.Errorf("出口节点不存在")
		}
		outNode = &node
	}
	return &inNode, outNode, nil
}

func (s *ForwardService) buildServiceName(forwardId int64, userId int64, userTunnel *model.UserTunnel) string {
	utId := int64(0)
	if userTunnel != nil {
		utId = int64(userTunnel.ID)
	}
	return fmt.Sprintf("%d_%d_%d", forwardId, userId, utId)
}

// checkLoopbackAddress 检查远端地址是否会导致自环
// 如果远端地址指向入口节点的入口端口，会导致服务器崩溃
// 注意：tunnel.InIp 可能是逗号分隔的多个IP地址
func (s *ForwardService) checkLoopbackAddress(remoteAddr string, tunnel *model.Tunnel, inPort int) error {
	// 解析入口节点所有IP
	inIps := make(map[string]bool)
	for _, ip := range strings.Split(tunnel.InIp, ",") {
		inIps[strings.TrimSpace(ip)] = true
	}

	// 检查每个远端地址
	addrs := strings.Split(remoteAddr, ",")
	for _, addr := range addrs {
		ip := utils.ExtractIp(strings.TrimSpace(addr))
		port := utils.ExtractPort(strings.TrimSpace(addr))

		// 检查是否指向入口节点的入口端口
		if inIps[ip] && port == inPort {
			return fmt.Errorf("远端地址不能指向入口节点的监听端口(%s:%d)，会导致自环", ip, port)
		}
	}
	return nil
}

// Keep the Stub method for TunnelService
// Stub kept for compatibility
func (s *ForwardService) CountForwardsByTunnelId(tunnelId int64) int64 {
	var count int64
	global.DB.Model(&model.Forward{}).Where("tunnel_id = ?", tunnelId).Count(&count)
	return count
}

func (s *ForwardService) UpdateForwardOrder(params map[string]interface{}, ctxUser *utils.UserClaims) *result.Result {
	forwardsList, ok := params["forwards"].([]interface{})
	if !ok || len(forwardsList) == 0 {
		return result.Err(-1, "forwards参数不能为空")
	}

	// Permission check handled by iterating and verifying ownership if non-admin
	// But efficiently:
	ids := make([]int64, 0, len(forwardsList))
	updates := make(map[int64]int)

	for _, item := range forwardsList {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		idVal := int64(m["id"].(float64))
		inxVal := int(m["inx"].(float64))
		ids = append(ids, idVal)
		updates[idVal] = inxVal
	}

	var forwards []model.Forward
	tx := global.DB.Where("id IN ?", ids)
	if ctxUser.RoleId != 0 {
		tx = tx.Where("user_id = ?", ctxUser.GetUserId())
	}
	tx.Find(&forwards)

	if len(forwards) != len(ids) {
		return result.Err(-1, "只能更新自己的转发排序")
	}

	// Batch Update
	// GORM doesn't support batch update with different values easily in one query without raw SQL or loop
	// Using loop for Simplicity as list shouldn't be huge
	for _, f := range forwards {
		if newInx, ok := updates[f.ID]; ok {
			f.Inx = newInx
			global.DB.Save(&f)
		}
	}
	return result.Ok("排序更新成功")
}

func (s *ForwardService) PauseForward(id int64, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	// Permission Check
	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权暂停此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)

	serviceName := s.buildServiceName(forward.ID, forward.UserId, &userTunnel)

	// 暂停入口服务（Type 1 和 Type 2 都需要）
	if res := utils.PauseService(tunnel.InNodeId, serviceName); res.Msg != "OK" {
		if !isTransientNodeSyncMessage(res.Msg) && !isGostNotFoundMessage(res.Msg) {
			return result.Err(-1, "暂停服务失败: "+res.Msg)
		}
	}

	// Type 2 隧道不再需要暂停远程服务
	// 共享的 relay service 由 tunnel 管理，forward 暂停不影响它

	// 更新状态
	forward.Status = 0
	forward.PauseReason = forwardPauseReasonManual
	forward.UpdatedTime = time.Now().UnixMilli()
	global.DB.Save(&forward)

	return result.Ok("服务已暂停")
}

func (s *ForwardService) ResumeForward(id int64, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	// Permission Check
	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权恢复此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	// 检查隧道状态
	if tunnel.Status != 1 {
		return result.Err(-1, "隧道已禁用，无法恢复服务")
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)
	var ownerForResume model.User
	if err := global.DB.First(&ownerForResume, forward.UserId).Error; err != nil {
		return result.Err(-1, "用户异常")
	}
	if ownerForResume.RoleId != 0 {
		now := time.Now().UnixMilli()
		if reason := userRuntimeBlockReason(&ownerForResume, now); reason != "" {
			return result.Err(-1, reason)
		}
		if userTunnel.ID == 0 {
			return result.Err(-1, "该用户没有该隧道权限")
		}
		if reason := userTunnelRuntimeBlockReason(&userTunnel, now); reason != "" {
			return result.Err(-1, reason)
		}
	}

	serviceName := s.buildServiceName(forward.ID, forward.UserId, &userTunnel)

	// 恢复入口服务（Type 1 和 Type 2 都需要）
	if res := utils.ResumeService(tunnel.InNodeId, serviceName); res.Msg != "OK" {
		if isGostNotFoundMessage(res.Msg) {
			var limiter *int
			if userTunnel.ID != 0 {
				limiter = &userTunnel.SpeedId
			}
			if err := s.createGostServices(&forward, &tunnel, limiter, &userTunnel); err != nil && !isTransientNodeSyncError(err) {
				return result.Err(-1, "恢复服务失败: "+err.Error())
			}
		} else if !isTransientNodeSyncMessage(res.Msg) {
			return result.Err(-1, "恢复服务失败: "+res.Msg)
		}
	}

	// Type 2 隧道不再需要恢复远程服务
	// 共享的 relay service 由 tunnel 管理

	// 更新状态
	forward.Status = 1
	forward.PauseReason = forwardPauseReasonManual
	forward.UpdatedTime = time.Now().UnixMilli()
	global.DB.Save(&forward)

	return result.Ok("服务已恢复")
}

func (s *ForwardService) ForceDeleteForward(id int64, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	// Permission Check
	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权删除此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err == nil {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, forward.TunnelId).First(&userTunnel)
		_ = s.deleteGostServices(&forward, &tunnel, &userTunnel)
	}

	global.DB.Delete(&forward)
	return result.Ok("强制删除成功")
}

func (s *ForwardService) DiagnoseForward(id int64, ctxUser *utils.UserClaims) *result.Result {
	var forward model.Forward
	if err := global.DB.First(&forward, id).Error; err != nil {
		return result.Err(-1, "转发不存在")
	}

	if ctxUser.RoleId != 0 && forward.UserId != ctxUser.GetUserId() {
		return result.Err(-1, "无权访问此转发")
	}

	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, forward.TunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	inNode, outNode, err := s.getRequiredNodes(&tunnel)
	if err != nil {
		return result.Err(-1, err.Error())
	}

	results := []map[string]interface{}{}
	remoteAddrs := strings.Split(forward.RemoteAddr, ",")

	if tunnel.Type == 1 {
		// Port Forward: InNode performs TCP Ping to Targets
		for _, addr := range remoteAddrs {
			targetIp := utils.ExtractIp(addr)
			targetPort := utils.ExtractPort(addr)
			if targetIp == "" || targetPort == -1 {
				continue
			}
			res := Tunnel.PerformTcpPing(inNode, targetIp, targetPort, "转发->目标")
			results = append(results, res)
		}
	} else {
		// Tunnel Forward: InNode -> OutNode, OutNode -> Targets
		// In -> Out
		resIn := Tunnel.PerformTcpPing(inNode, outNode.ServerIp, tunnel.OutPort, "入口->出口")
		results = append(results, resIn)

		// Out -> Targets
		for _, addr := range remoteAddrs {
			targetIp := utils.ExtractIp(addr)
			targetPort := utils.ExtractPort(addr)
			if targetIp == "" || targetPort == -1 {
				continue
			}
			res := Tunnel.PerformTcpPing(outNode, targetIp, targetPort, "出口->目标")
			results = append(results, res)
		}
	}

	report := map[string]interface{}{
		"forwardId":   forward.ID,
		"forwardName": forward.Name,
		"tunnelType":  "端口转发",
		"results":     results,
		"timestamp":   time.Now().UnixMilli(),
	}
	if tunnel.Type == 2 {
		report["tunnelType"] = "隧道转发"
	}
	return result.Ok(report)
}
