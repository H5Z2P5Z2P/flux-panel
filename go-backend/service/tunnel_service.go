package service

import (
	"fmt"
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

		// 分配 ChainPort（出口节点上的 Chain 监听端口）
		chainPort, err := s.allocateChainPort(outNode.ID)
		if err != nil {
			return result.Err(-1, "分配 Chain 端口失败: "+err.Error())
		}
		tunnel.ChainPort = chainPort
	}

	// Defaults
	tunnel.Status = 1
	tunnel.CreatedTime = time.Now().UnixMilli()
	tunnel.UpdatedTime = time.Now().UnixMilli()

	if err := global.DB.Create(&tunnel).Error; err != nil {
		return result.Err(-1, "隧道创建失败: "+err.Error())
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
			ID:            tunnel.ID,
			Name:          tunnel.Name,
			Ip:            tunnel.InIp, // Or node.Ip? Java response uses "ip": "45.8..." which matches InIp usually
			Type:          tunnel.Type,
			Protocol:      tunnel.Protocol,
			InNodePortSta: node.PortSta,
			InNodePortEnd: node.PortEnd,
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
	var tunnel model.Tunnel
	if err := global.DB.First(&tunnel, tunnelId).Error; err == nil && tunnel.ChainPort != 0 {
		return tunnel.ChainPort
	}
	return 22 // Default SSH
}

func (s *TunnelService) DeleteTunnel(id int64) *result.Result {
	// ... (Existing implementation) ...
	if count := Forward.CountForwardsByTunnelId(id); count > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个转发在使用，请先删除相关转发", count))
	}
	if count := UserTunnel.CountUserTunnelsByTunnelId(id); count > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个用户权限关联，请先取消用户权限分配", count))
	}

	if err := global.DB.Delete(&model.Tunnel{}, id).Error; err != nil {
		return result.Err(-1, "隧道删除失败")
	}
	return result.Ok("隧道删除成功")
}

// allocateChainPort 为隧道转发分配 Chain 监听端口（在出口节点上）
func (s *TunnelService) allocateChainPort(outNodeId int64) (int, error) {
	var node model.Node
	if err := global.DB.First(&node, outNodeId).Error; err != nil {
		return 0, fmt.Errorf("出口节点不存在")
	}

	usedPorts := s.getUsedChainPorts(outNodeId, nil)

	for port := node.PortSta; port <= node.PortEnd; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("出口节点 %s 无可用端口", node.Name)
}

// getUsedChainPorts 获取出口节点上已使用的 Chain 端口
func (s *TunnelService) getUsedChainPorts(nodeId int64, excludeTunnelId *int64) map[int]bool {
	used := make(map[int]bool)

	// 统计该节点作为出口节点的隧道的 ChainPort
	var tunnels []model.Tunnel
	query := global.DB.Where("out_node_id = ? AND type = 2 AND chain_port > 0", nodeId)
	if excludeTunnelId != nil {
		query = query.Where("id != ?", *excludeTunnelId)
	}
	query.Find(&tunnels)

	for _, t := range tunnels {
		used[t.ChainPort] = true
	}

	// 同时统计该节点作为入口节点的 forward.in_port
	var inTunnels []int64
	global.DB.Model(&model.Tunnel{}).Where("in_node_id = ?", nodeId).Pluck("id", &inTunnels)
	if len(inTunnels) > 0 {
		var forwards []model.Forward
		global.DB.Where("tunnel_id IN ?", inTunnels).Find(&forwards)
		for _, f := range forwards {
			used[f.InPort] = true
		}
	}

	return used
}
