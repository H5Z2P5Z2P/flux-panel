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

func (s *TunnelService) CreateTunnel(tunnelDto dto.TunnelDto) *result.Result {
	// 1. Verify Name
	var count int64
	global.DB.Model(&model.Tunnel{}).Where("name = ?", tunnelDto.Name).Count(&count)
	if count > 0 {
		return result.Err(-1, "隧道名称已存在")
	}

	// 2. 准备节点列表 (处理兼容性)
	if len(tunnelDto.Nodes) == 0 {
		// 向下兼容旧格式
		if tunnelDto.Type == 2 {
			if tunnelDto.OutNodeId == nil {
				return result.Err(-1, "出口节点不能为空")
			}
			tunnelDto.Nodes = []dto.TunnelNodeDto{
				{NodeId: tunnelDto.InNodeId, Protocol: tunnelDto.Protocol, TcpListenAddr: tunnelDto.TcpListenAddr, UdpListenAddr: tunnelDto.UdpListenAddr, InterfaceName: tunnelDto.InterfaceName},
				{NodeId: *tunnelDto.OutNodeId, Protocol: "relay"},
			}
		} else {
			tunnelDto.Nodes = []dto.TunnelNodeDto{
				{NodeId: tunnelDto.InNodeId, Protocol: tunnelDto.Protocol, TcpListenAddr: tunnelDto.TcpListenAddr, UdpListenAddr: tunnelDto.UdpListenAddr, InterfaceName: tunnelDto.InterfaceName},
			}
		}
	}

	// 3. 验证所有节点状态
	var nodeIds []int64
	for _, n := range tunnelDto.Nodes {
		nodeIds = append(nodeIds, n.NodeId)
	}
	var nodes []model.Node
	global.DB.Where("id IN ?", nodeIds).Find(&nodes)
	if len(nodes) != len(uniqueInt64(nodeIds)) {
		return result.Err(-1, "部分节点不存在")
	}
	nodeMap := make(map[int64]model.Node)
	for _, n := range nodes {
		if n.Status != 1 {
			return result.Err(-1, fmt.Sprintf("节点 %s 当前离线", n.Name))
		}
		nodeMap[n.ID] = n
	}

	// 4. 创建隧道记录
	tunnel := model.Tunnel{
		Name:         tunnelDto.Name,
		Type:         tunnelDto.Type,
		Flow:         tunnelDto.Flow,
		Protocol:     tunnelDto.Protocol,
		CreatedTime:  time.Now().UnixMilli(),
		UpdatedTime:  time.Now().UnixMilli(),
		Status:       1,
		TrafficRatio: 1.0,
	}
	if !tunnelDto.TrafficRatio.IsZero() {
		f, _ := tunnelDto.TrafficRatio.Float64()
		tunnel.TrafficRatio = f
	}

	if err := global.DB.Create(&tunnel).Error; err != nil {
		return result.Err(-1, "隧道创建失败: "+err.Error())
	}

	// 5. 创建节点记录
	tunnelNodes := make([]model.TunnelNode, 0)
	for i, n := range tunnelDto.Nodes {
		nodeType := 2 // 默认中转
		if i == 0 {
			nodeType = 1 // 入口
		} else if i == len(tunnelDto.Nodes)-1 {
			nodeType = 3 // 出口
		}

		tn := model.TunnelNode{
			TunnelId:      tunnel.ID,
			NodeId:        n.NodeId,
			Type:          nodeType,
			Inx:           i,
			Protocol:      n.Protocol,
			TcpListenAddr: n.TcpListenAddr,
			UdpListenAddr: n.UdpListenAddr,
			InterfaceName: n.InterfaceName,
		}

		// 为非入口分配端口
		if nodeType != 1 {
			port, err := s.allocateTunnelOutPort(n.NodeId, nil)
			if err != nil {
				global.DB.Delete(&tunnel)
				return result.Err(-1, "端口分配失败: "+err.Error())
			}
			tn.Port = port
		}
		tunnelNodes = append(tunnelNodes, tn)
	}

	if err := global.DB.Create(&tunnelNodes).Error; err != nil {
		global.DB.Delete(&tunnel)
		return result.Err(-1, "隧道节点创建失败: "+err.Error())
	}

	// 6. 创建 GOST 服务 (仅 Type 2 需要链路编排)
	if tunnel.Type == 2 {
		tunnel.Nodes = tunnelNodes
		if err := s.createTunnelSharedServices(&tunnel, nodeMap); err != nil {
			// 回滚逻辑 (简单清理)
			global.DB.Delete(&tunnelNodes)
			global.DB.Delete(&tunnel)
			return result.Err(-1, "共享服务创建失败: "+err.Error())
		}
	}

	return result.Ok("隧道创建成功")
}

func uniqueInt64(ids []int64) []int64 {
	m := make(map[int64]bool)
	var res []int64
	for _, id := range ids {
		if !m[id] {
			m[id] = true
			res = append(res, id)
		}
	}
	return res
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
		var userTunnels []model.UserTunnel
		global.DB.Where("user_id = ? AND status = 1", userId).Find(&userTunnels)
		for _, ut := range userTunnels {
			if ut.ExpTime > 0 && ut.ExpTime <= time.Now().UnixMilli() {
				continue // Expired
			}
			var t model.Tunnel
			if global.DB.First(&t, ut.TunnelId).Error == nil && t.Status == 1 {
				tunnels = append(tunnels, t)
			}
		}
	}

	var response []dto.UserTunnelResponseDto
	for _, tunnel := range tunnels {
		// 获取入口节点 (inx=0)
		var entryTN model.TunnelNode
		if err := global.DB.Where("tunnel_id = ? AND inx = 0", tunnel.ID).First(&entryTN).Error; err != nil {
			continue
		}

		var node model.Node
		if err := global.DB.First(&node, entryTN.NodeId).Error; err != nil {
			continue
		}

		dto := dto.UserTunnelResponseDto{
			ID:               tunnel.ID,
			Name:             tunnel.Name,
			Ip:               node.ServerIp, // 使用物理节点的 ServerIp
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
	if err := global.DB.Preload("Nodes").First(&tunnel, req.ID).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	var count int64
	global.DB.Model(&model.Tunnel{}).Where("name = ? AND id != ?", req.Name, req.ID).Count(&count)
	if count > 0 {
		return result.Err(-1, "隧道名称已存在")
	}

	// 比较节点是否有变更
	nodesChanged := false
	if len(req.Nodes) > 0 {
		if len(req.Nodes) != len(tunnel.Nodes) {
			nodesChanged = true
		} else {
			for i, newNode := range req.Nodes {
				oldNode := tunnel.Nodes[i]
				if newNode.NodeId != oldNode.NodeId || newNode.Protocol != oldNode.Protocol {
					nodesChanged = true
					break
				}
			}
		}
	}

	// 如果节点有变，或者协议有变
	criticalChange := nodesChanged || tunnel.Protocol != req.Protocol

	if nodesChanged {
		// 1. 删除旧的共享服务
		s.deleteTunnelSharedServices(&tunnel)
		// 2. 删除旧节点记录
		global.DB.Where("tunnel_id = ?", tunnel.ID).Delete(&model.TunnelNode{})

		// 3. 准备新节点信息并分配端口 (类似于 CreateTunnel 中的逻辑)
		// 这里为了简洁，假设后续会复用逻辑或在此处展开。
		// 先获取 nodeMap
		var nodeIds []int64
		for _, n := range req.Nodes {
			nodeIds = append(nodeIds, n.NodeId)
		}
		var nodes []model.Node
		global.DB.Where("id IN ?", nodeIds).Find(&nodes)
		nodeMap := make(map[int64]model.Node)
		for _, n := range nodes {
			nodeMap[n.ID] = n
		}

		newTunnelNodes := make([]model.TunnelNode, 0)
		for i, n := range req.Nodes {
			tn := model.TunnelNode{
				TunnelId:      tunnel.ID,
				NodeId:        n.NodeId,
				Type:          2, // 默认中继
				Inx:           i,
				Protocol:      n.Protocol,
				TcpListenAddr: n.TcpListenAddr,
				UdpListenAddr: n.UdpListenAddr,
				InterfaceName: n.InterfaceName,
			}
			if i == 0 {
				tn.Type = 1 // 入口
			} else if i == len(req.Nodes)-1 {
				tn.Type = 3 // 出口
			}

			if tn.Type != 1 {
				// 分配端口
				port, _ := s.allocateTunnelOutPort(n.NodeId, &tunnel.ID)
				tn.Port = port
			}
			newTunnelNodes = append(newTunnelNodes, tn)
		}
		if err := global.DB.Create(&newTunnelNodes).Error; err != nil {
			return result.Err(-1, "新隧道节点创建失败")
		}
		tunnel.Nodes = newTunnelNodes
	}

	// 更新元数据
	tunnel.Name = req.Name
	tunnel.Flow = req.Flow
	tunnel.Protocol = req.Protocol
	tunnel.InterfaceName = req.InterfaceName
	if !req.TrafficRatio.IsZero() {
		f, _ := req.TrafficRatio.Float64()
		tunnel.TrafficRatio = f
	}
	tunnel.UpdatedTime = time.Now().UnixMilli()

	if err := global.DB.Save(&tunnel).Error; err != nil {
		return result.Err(-1, "隧道保存失败")
	}

	// 如果是 Type 2 且链路有变，重建服务
	if tunnel.Type == 2 && nodesChanged {
		// 重新构建 nodeMap (确保包含所有新节点)
		var nodes []model.Node
		global.DB.Find(&nodes)
		nodeMap := make(map[int64]model.Node)
		for _, n := range nodes {
			nodeMap[n.ID] = n
		}
		if err := s.createTunnelSharedServices(&tunnel, nodeMap); err != nil {
			return result.Err(-1, "新共享服务创建失败: "+err.Error())
		}
	}

	// 同步转发
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
			res := Forward.UpdateForward(f.ID, fDto, &utils.UserClaims{RoleId: 0, User: f.UserName, RegisteredClaims: jwt.RegisteredClaims{Subject: fmt.Sprintf("%d", f.UserId)}})
			if res.Code != 0 {
				return result.Err(-1, fmt.Sprintf("隧道更新成功，但在同步转发 %s 时失败: %s", f.Name, res.Msg))
			}
		}
	}

	return result.Ok("隧道更新成功")
}

func (s *TunnelService) DeleteTunnel(id int64) *result.Result {
	var tunnel model.Tunnel
	if err := global.DB.Preload("Nodes").First(&tunnel, id).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	// 依赖检查
	if count := Forward.CountForwardsByTunnelId(id); count > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个转发在使用，请先删除相关转发", count))
	}
	// 这里假设 UserTunnel 也是一个可访问的 service 实例
	var userTunnelCount int64
	global.DB.Model(&model.UserTunnel{}).Where("tunnel_id = ?", id).Count(&userTunnelCount)
	if userTunnelCount > 0 {
		return result.Err(-1, fmt.Sprintf("该隧道还有 %d 个用户权限关联，请先取消用户权限分配", userTunnelCount))
	}

	// Type 2 隧道：删除全链路共享服务
	if tunnel.Type == 2 {
		s.deleteTunnelSharedServices(&tunnel)
	}

	// 删除节点关联
	global.DB.Where("tunnel_id = ?", id).Delete(&model.TunnelNode{})

	if err := global.DB.Delete(&model.Tunnel{}, id).Error; err != nil {
		return result.Err(-1, "隧道删除失败")
	}
	return result.Ok("隧道删除成功")
}
func (s *TunnelService) DiagnoseTunnel(tunnelId int64) *result.Result {
	var tunnel model.Tunnel
	if err := global.DB.Preload("Nodes").First(&tunnel, tunnelId).Error; err != nil {
		return result.Err(-1, "隧道不存在")
	}

	var nodeIds []int64
	for _, n := range tunnel.Nodes {
		nodeIds = append(nodeIds, n.NodeId)
	}
	var nodes []model.Node
	global.DB.Where("id IN ?", nodeIds).Find(&nodes)
	nodeMap := make(map[int64]model.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	results := []map[string]interface{}{}

	if tunnel.Type == 1 {
		entryTN := tunnel.Nodes[0]
		entryNode := nodeMap[entryTN.NodeId]
		res := s.PerformTcpPing(&entryNode, "www.google.com", 443, "入口 -> 外网")
		results = append(results, res)
	} else {
		for i := 0; i < len(tunnel.Nodes)-1; i++ {
			currTN := tunnel.Nodes[i]
			nextTN := tunnel.Nodes[i+1]
			currNode := nodeMap[currTN.NodeId]
			nextNode := nodeMap[nextTN.NodeId]

			desc := fmt.Sprintf("第 %d 跳 (%s -> %s)", i+1, currNode.Name, nextNode.Name)
			res := s.PerformTcpPing(&currNode, nextNode.ServerIp, nextTN.Port, desc)
			results = append(results, res)
		}

		exitTN := tunnel.Nodes[len(tunnel.Nodes)-1]
		exitNode := nodeMap[exitTN.NodeId]
		res := s.PerformTcpPing(&exitNode, "www.google.com", 443, "出口 -> 外网")
		results = append(results, res)
	}

	report := map[string]interface{}{
		"tunnelId":   tunnel.ID,
		"tunnelName": tunnel.Name,
		"tunnelType": "端口转发",
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
	var tn model.TunnelNode
	// 获取出口节点 (inx 最大的节点)
	if err := global.DB.Where("tunnel_id = ?", tunnelId).Order("inx desc").First(&tn).Error; err == nil {
		return tn.Port
	}
	return 0
}

// --- Tunnel Type 2 共享服务管理 ---

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

// getUsedTunnelOutPorts 获取节点已被 tunnel_node 占用的端口
func (s *TunnelService) getUsedTunnelOutPorts(nodeId int64, excludeTunnelId *int64) map[int]bool {
	used := make(map[int]bool)

	// 查找所有使用该节点的 tunnel_node 记录 (且 port > 0)
	var tns []model.TunnelNode
	query := global.DB.Where("node_id = ? AND port > 0", nodeId)
	if excludeTunnelId != nil {
		query = query.Where("tunnel_id != ?", *excludeTunnelId)
	}
	query.Find(&tns)

	for _, tn := range tns {
		used[tn.Port] = true
	}
	return used
}

// createTunnelSharedServices 为 Type 2 隧道创建共享的 chain 和 relay service
func (s *TunnelService) createTunnelSharedServices(tunnel *model.Tunnel, nodeMap map[int64]model.Node) error {
	// 从后往前创建，确保 downstream 已就绪
	// 1. 中继与出口节点：创建 Relay Service
	for i := len(tunnel.Nodes) - 1; i > 0; i-- {
		tn := tunnel.Nodes[i]
		node := nodeMap[tn.NodeId]

		if res := utils.AddTunnelRelayService(node.ID, tunnel.ID, tn.Port, tn.Protocol, tn.InterfaceName); res.Msg != "OK" {
			return fmt.Errorf("节点 %s 创建共享 Relay 失败: %s", node.Name, res.Msg)
		}
	}

	// 2. 入口节点：创建共享 Chain (包含所有后续 Hops)
	entryTN := tunnel.Nodes[0]
	entryNode := nodeMap[entryTN.NodeId]

	// 构造带地址信息的 Hops 列表
	nodeInfos := make([]dto.TunnelNodeInfo, 0)
	for _, tn := range tunnel.Nodes {
		node := nodeMap[tn.NodeId]
		addr := node.ServerIp
		if strings.Contains(addr, ":") {
			addr = "[" + addr + "]"
		}

		info := dto.TunnelNodeInfo{
			TunnelNode: tn,
			Addr:       fmt.Sprintf("%s:%d", addr, tn.Port),
		}
		nodeInfos = append(nodeInfos, info)
	}

	if res := utils.AddTunnelChain(entryNode.ID, tunnel.ID, nodeInfos); res.Msg != "OK" {
		return fmt.Errorf("入口节点 %s 创建共享 Chain 失败: %s", entryNode.Name, res.Msg)
	}

	return nil
}

// deleteTunnelSharedServices 删除 Type 2 隧道的共享 chain 和 relay service
func (s *TunnelService) deleteTunnelSharedServices(tunnel *model.Tunnel) error {
	nodes := tunnel.Nodes
	if len(nodes) == 0 {
		global.DB.Where("tunnel_id = ?", tunnel.ID).Order("inx asc").Find(&nodes)
	}

	for _, tn := range nodes {
		if tn.Type == 1 {
			// 删除入口节点的共享 chain
			utils.DeleteTunnelChain(tn.NodeId, tunnel.ID)
		} else {
			// 删除中转/出口节点的共享 relay service
			utils.DeleteTunnelRelayService(tn.NodeId, tunnel.ID)
		}
	}

	return nil
}
