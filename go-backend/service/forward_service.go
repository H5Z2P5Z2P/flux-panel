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

type ForwardService struct{}

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

	// 2. Permissions & Limits
	var limiter *int
	var userTunnel *model.UserTunnel
	if ctxUser.RoleId != 0 {
		var ut model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", ctxUser.GetUserId(), dto.TunnelId).First(&ut).Error; err != nil {
			return result.Err(-1, "你没有该隧道权限")
		}
		if ut.Status != 1 {
			return result.Err(-1, "隧道被禁用")
		}
		if ut.ExpTime > 0 && ut.ExpTime <= time.Now().UnixMilli() {
			return result.Err(-1, "该隧道权限已到期")
		}
		// Check limit
		if ut.Num > 0 {
			var currentCount int64
			global.DB.Model(&model.Forward{}).Where("user_id = ? AND tunnel_id = ?", ctxUser.GetUserId(), dto.TunnelId).Count(&currentCount)
			if int(currentCount) >= ut.Num {
				return result.Err(-1, fmt.Sprintf("转发数量已达上限(%d个)", ut.Num))
			}
		}

		userTunnel = &ut
		limiter = &ut.SpeedId
	} else {
		// Admin: try to find user tunnel wrapper if exists for target user
		// But Wait, CreateForward doesn't specify Target User ID in DTO if Admin creates?
		// Assuming Admin creates for themselves or context user. DTO doesn't have UserId.
	}

	// 3. Allocate Port
	portAlloc, err := s.allocatePorts(&tunnel, dto.InPort, nil)
	if err != nil {
		return result.Err(-1, err.Error())
	}

	// 4. Create Entity
	forward := model.Forward{
		UserId:        ctxUser.GetUserId(),
		UserName:      ctxUser.User,
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
	if err := s.createGostServices(&forward, &tunnel, limiter, userTunnel); err != nil {
		global.DB.Delete(&forward) // Rollback
		return result.Err(-1, "Gost服务创建失败: "+err.Error())
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

	// Permissions for new tunnel if changed
	var userTunnel *model.UserTunnel
	var limiter *int
	if ctxUser.RoleId != 0 {
		var ut model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", ctxUser.GetUserId(), dto.TunnelId).First(&ut).Error; err != nil {
			return result.Err(-1, "你没有该隧道权限")
		}
		if ut.Status != 1 {
			return result.Err(-1, "隧道被禁用")
		}
		// Check ExpTime, Flow, etc.
		userTunnel = &ut
		limiter = &ut.SpeedId
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
	updatedForward.Status = 1

	// Gost Sync
	if tunnelChanged {
		// Delete Old
		var oldTunnel model.Tunnel
		global.DB.First(&oldTunnel, forward.TunnelId)
		var oldUT model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, oldTunnel.ID).First(&oldUT)
		s.deleteGostServices(&forward, &oldTunnel, &oldUT)

		// Create New
		if err := s.createGostServices(&updatedForward, &tunnel, limiter, userTunnel); err != nil {
			return result.Err(-1, "Gost服务更新失败: "+err.Error())
		}
	} else {
		// Update Same Tunnel
		if err := s.updateGostServices(&updatedForward, &tunnel, limiter, userTunnel); err != nil {
			return result.Err(-1, "Gost服务更新失败: "+err.Error())
		}
	}

	// Save to DB
	// Use map to update specific fields to avoid zero values if any
	global.DB.Model(&forward).Updates(map[string]interface{}{
		"name":           updatedForward.Name,
		"tunnel_id":      updatedForward.TunnelId,
		"in_port":        updatedForward.InPort,
		"out_port":       updatedForward.OutPort,
		"remote_addr":    updatedForward.RemoteAddr,
		"interface_name": updatedForward.InterfaceName,
		"strategy":       updatedForward.Strategy,
		"updated_time":   updatedForward.UpdatedTime,
	})

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
		return result.Err(-1, "Gost服务删除失败: "+err.Error())
	}

	global.DB.Delete(&forward)
	return result.Ok("删除成功")
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
	inNode, outNode, err := s.getRequiredNodes(tunnel)
	if err != nil {
		return err
	}

	// Type 2: Tunnel Forward
	if tunnel.Type == 2 {
		remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, forward.OutPort)
		if strings.Contains(tunnel.OutIp, ":") {
			remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, forward.OutPort)
		}
		if res := utils.AddChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName); res.Msg != "OK" {
			return fmt.Errorf("Chain Error: " + res.Msg)
		}
		if res := utils.AddRemoteService(outNode.ID, serviceName, forward.OutPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName); res.Msg != "OK" {
			utils.DeleteChains(inNode.ID, serviceName)
			return fmt.Errorf("Remote Error: " + res.Msg)
		}
	}

	interfaceName := ""
	if tunnel.Type == 1 {
		interfaceName = forward.InterfaceName
	}

	if res := utils.AddService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName); res.Msg != "OK" {
		utils.DeleteChains(inNode.ID, serviceName)
		if outNode != nil {
			utils.DeleteRemoteService(outNode.ID, serviceName)
		}
		return fmt.Errorf("Service Error: " + res.Msg)
	}
	return nil
}

func (s *ForwardService) updateGostServices(forward *model.Forward, tunnel *model.Tunnel, limiter *int, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, outNode, err := s.getRequiredNodes(tunnel)
	if err != nil {
		return err
	}

	if tunnel.Type == 2 {
		remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, forward.OutPort)
		if strings.Contains(tunnel.OutIp, ":") {
			remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, forward.OutPort)
		}
		if res := utils.UpdateChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName); res.Msg != "OK" {
			// Fallback Add if not found
			if strings.Contains(res.Msg, "not found") {
				utils.AddChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName)
			} else {
				return fmt.Errorf("Update Chain Error: " + res.Msg)
			}
		}
		if res := utils.UpdateRemoteService(outNode.ID, serviceName, forward.OutPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName); res.Msg != "OK" {
			if strings.Contains(res.Msg, "not found") {
				utils.AddRemoteService(outNode.ID, serviceName, forward.OutPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName)
			} else {
				return fmt.Errorf("Update Remote Service Error: " + res.Msg)
			}
		}
	}

	interfaceName := ""
	if tunnel.Type == 1 {
		interfaceName = forward.InterfaceName
	}

	res := utils.UpdateService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName)
	if res.Msg != "OK" {
		if strings.Contains(res.Msg, "not found") {
			utils.AddService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, interfaceName)
		} else {
			return fmt.Errorf("Update Service Error: " + res.Msg)
		}
	}
	return nil
}

func (s *ForwardService) deleteGostServices(forward *model.Forward, tunnel *model.Tunnel, userTunnel *model.UserTunnel) error {
	serviceName := s.buildServiceName(forward.ID, forward.UserId, userTunnel)
	inNode, outNode, _ := s.getRequiredNodes(tunnel)

	if inNode != nil {
		res := utils.DeleteService(inNode.ID, serviceName)
		if res.Msg != "OK" {
			return fmt.Errorf(res.Msg)
		}
	}

	if tunnel.Type == 2 {
		if inNode != nil {
			utils.DeleteChains(inNode.ID, serviceName)
		}
		if outNode != nil {
			utils.DeleteRemoteService(outNode.ID, serviceName)
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

	// Allocate OutPort (for Tunnel Forward)
	var outPort int
	if tunnel.Type == 2 {
		// Tunnel Forward needs output node port
		p, err := s.findFreePort(tunnel.OutNodeId, excludeForwardId)
		if err != nil {
			return nil, fmt.Errorf("出口节点无可用端口")
		}
		outPort = p
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
	if port < node.PortSta || port > node.PortEnd {
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
	used := s.getUsedPorts(nodeId, excludeForwardId)
	for p := node.PortSta; p <= node.PortEnd; p++ {
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

	// Pause入口服务
	if res := utils.PauseService(tunnel.InNodeId, serviceName); res.Msg != "OK" {
		return result.Err(-1, "暂停服务失败: "+res.Msg)
	}

	// 如果是隧道转发，暂停远程服务
	if tunnel.Type == 2 {
		if res := utils.PauseRemoteService(tunnel.OutNodeId, serviceName); res.Msg != "OK" {
			return result.Err(-1, "暂停远程服务失败: "+res.Msg)
		}
	}

	// 更新状态
	forward.Status = 0
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

	// 普通用户需要检查流量和账户状态
	if ctxUser.RoleId != 0 {
		var user model.User
		global.DB.First(&user, ctxUser.GetUserId())

		// 检查用户流量
		totalFlow := user.InFlow + user.OutFlow
		if user.Flow > 0 && totalFlow >= user.Flow*1024*1024*1024 {
			return result.Err(-1, "用户流量已超限")
		}

		// 检查用户到期
		if user.ExpTime > 0 && user.ExpTime <= time.Now().UnixMilli() {
			return result.Err(-1, "当前账号已到期")
		}

		// 检查用户状态
		if user.Status != 1 {
			return result.Err(-1, "用户账号已禁用")
		}

		// 检查隧道权限
		var userTunnel model.UserTunnel
		if err := global.DB.Where("user_id = ? AND tunnel_id = ?", ctxUser.GetUserId(), tunnel.ID).First(&userTunnel).Error; err != nil {
			return result.Err(-1, "你没有该隧道权限")
		}

		if userTunnel.Status != 1 {
			return result.Err(-1, "隧道权限已禁用")
		}
	}

	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)

	serviceName := s.buildServiceName(forward.ID, forward.UserId, &userTunnel)

	// Resume入口服务
	if res := utils.ResumeService(tunnel.InNodeId, serviceName); res.Msg != "OK" {
		return result.Err(-1, "恢复服务失败: "+res.Msg)
	}

	// 如果是隧道转发，恢复远程服务
	if tunnel.Type == 2 {
		if res := utils.ResumeRemoteService(tunnel.OutNodeId, serviceName); res.Msg != "OK" {
			return result.Err(-1, "恢复远程服务失败: "+res.Msg)
		}
	}

	// 更新状态
	forward.Status = 1
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

	// 直接删除，跳过 Gost 服务删除
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
		resIn := Tunnel.PerformTcpPing(inNode, outNode.ServerIp, forward.OutPort, "入口->出口")
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
