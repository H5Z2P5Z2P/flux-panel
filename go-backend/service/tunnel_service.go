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

	"github.com/golang-jwt/jwt/v5"
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
	if inNode.Status != 1 {
		return result.Err(-1, "入口节点当前离线，请确保节点正常运行")
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
		if outNode.Status != 1 {
			return result.Err(-1, "出口节点当前离线，请确保节点正常运行")
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

	var count int64
	global.DB.Model(&model.Tunnel{}).Where("name = ? AND id != ?", req.Name, req.ID).Count(&count)
	if count > 0 {
		return result.Err(-1, "隧道名称已存在")
	}

	// Check for critical changes
	criticalChange := false
	if tunnel.TcpListenAddr != req.TcpListenAddr ||
		tunnel.UdpListenAddr != req.UdpListenAddr ||
		tunnel.Protocol != req.Protocol ||
		tunnel.InterfaceName != req.InterfaceName {
		criticalChange = true
	}

	tunnel.Name = req.Name
	tunnel.Flow = req.Flow
	tunnel.Protocol = req.Protocol
	tunnel.InterfaceName = req.InterfaceName
	tunnel.TcpListenAddr = req.TcpListenAddr
	tunnel.UdpListenAddr = req.UdpListenAddr
	if !req.TrafficRatio.IsZero() {
		f, _ := req.TrafficRatio.Float64()
		tunnel.TrafficRatio = f
	}
	tunnel.UpdatedTime = time.Now().UnixMilli()

	// Update DB
	if err := global.DB.Save(&tunnel).Error; err != nil {
		return result.Err(-1, "隧道更新失败: "+err.Error())
	}

	// Sync Forwards if needed
	if criticalChange {
		var forwards []model.Forward
		global.DB.Where("tunnel_id = ?", tunnel.ID).Find(&forwards)
		for _, f := range forwards {
			fDto := dto.ForwardDto{
				Name:          f.Name,
				TunnelId:      f.TunnelId,
				InPort:        &f.InPort,
				RemoteAddr:    f.RemoteAddr,
				InterfaceName: f.InterfaceName,
				Strategy:      f.Strategy,
			}
			// Use admin role (0) to bypass ownership check, acting as system sync
			res := Forward.UpdateForward(f.ID, fDto, &utils.UserClaims{RoleId: 0, User: f.UserName, RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", f.UserId)}})
			if res.Code != 0 {
				return result.Err(-1, fmt.Sprintf("隧道更新成功，但在同步转发 %s 时失败: %s", f.Name, res.Msg))
			}
		}
	}

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
		// Port Forward: Check connect to google? Or just ping self?
		// Java: tcp ping www.google.com:443 from InNode
		res := s.PerformTcpPing(&inNode, "www.google.com", 443, "入口->外网")
		results = append(results, res)
	} else {
		// Tunnel Forward
		var outNode model.Node
		if err := global.DB.First(&outNode, tunnel.OutNodeId).Error; err != nil {
			return result.Err(-1, "出口节点不存在")
		}

		outPort := s.getOutNodeTcpPort(tunnel.ID)

		// In -> Out
		res1 := s.PerformTcpPing(&inNode, outNode.ServerIp, outPort, "入口->出口")
		results = append(results, res1)

		// Out -> External
		res2 := s.PerformTcpPing(&outNode, "www.google.com", 443, "出口->外网")
		results = append(results, res2)
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

func (s *TunnelService) PerformTcpPing(node *model.Node, targetIp string, port int, desc string) map[string]interface{} {
	payload := map[string]interface{}{
		"ip":      targetIp,
		"port":    port,
		"count":   4,
		"timeout": 5000,
	}

	gostRes := websocket.SendMsg(node.ID, payload, "TcpPing")

	res := map[string]interface{}{
		"nodeId":      node.ID,
		"nodeName":    node.Name,
		"targetIp":    targetIp,
		"targetPort":  port,
		"description": desc,
		"timestamp":   time.Now().UnixMilli(),
		"averageTime": -1.0,
		"packetLoss":  100.0,
		"success":     false,
		"message":     "节点无响应",
	}

	if gostRes != nil && gostRes.Msg == "OK" {
		if dataMap, ok := gostRes.Data.(map[string]interface{}); ok {
			res["success"] = dataMap["success"]
			if dataMap["success"] == true {
				res["message"] = "TCP连接成功"
				res["averageTime"] = dataMap["averageTime"]
				res["packetLoss"] = dataMap["packetLoss"]
			} else {
				res["message"] = dataMap["errorMessage"]
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
	var forward model.Forward
	if err := global.DB.Where("tunnel_id = ? AND status = 1", tunnelId).First(&forward).Error; err == nil {
		return forward.OutPort
	}
	return 22 // Default SSH
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

// getUsedTunnelOutPorts 获取出口节点已被 tunnel 占用的端口
func (s *TunnelService) getUsedTunnelOutPorts(outNodeId int64, excludeTunnelId *int64) map[int]bool {
	used := make(map[int]bool)

	// 查找所有使用该节点作为出口的 Type 2 隧道
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

	// 同时检查 forward 使用的端口（兼容旧数据）
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

	// 构建出口节点远程地址
	remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, tunnel.OutPort)
	if strings.Contains(tunnel.OutIp, ":") {
		remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, tunnel.OutPort)
	}

	// 1. 在入口节点创建共享 chain
	if res := utils.AddTunnelChain(inNode.ID, tunnel.ID, remoteAddr, tunnel.Protocol, tunnel.InterfaceName); res.Msg != "OK" {
		return fmt.Errorf("创建共享 Chain 失败: %s", res.Msg)
	}

	// 2. 在出口节点创建共享 relay service
	if res := utils.AddTunnelRelayService(outNode.ID, tunnel.ID, tunnel.OutPort, tunnel.Protocol, tunnel.InterfaceName); res.Msg != "OK" {
		// 回滚：删除已创建的 chain
		utils.DeleteTunnelChain(inNode.ID, tunnel.ID)
		return fmt.Errorf("创建共享 Relay Service 失败: %s", res.Msg)
	}

	return nil
}

// deleteTunnelSharedServices 删除 Type 2 隧道的共享 chain 和 relay service
func (s *TunnelService) deleteTunnelSharedServices(tunnel *model.Tunnel) error {
	var inNode, outNode model.Node
	global.DB.First(&inNode, tunnel.InNodeId)
	global.DB.First(&outNode, tunnel.OutNodeId)

	// 删除入口节点的共享 chain
	if inNode.ID != 0 {
		utils.DeleteTunnelChain(inNode.ID, tunnel.ID)
	}

	// 删除出口节点的共享 relay service
	if outNode.ID != 0 {
		utils.DeleteTunnelRelayService(outNode.ID, tunnel.ID)
	}

	return nil
}
