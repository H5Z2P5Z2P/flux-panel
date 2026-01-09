package service

import (
	"fmt"
	"strings"

	"go-backend/global"
	"go-backend/model"
	"go-backend/utils"
)

// MigrationResult 迁移结果
type MigrationResult struct {
	MigratedCount int      // 成功迁移的记录数
	SkippedCount  int      // 跳过的记录数（节点离线）
	Errors        []string // 错误信息
}

// MigrateOutPortsWithSync 迁移历史数据中缺少 OutPort 的隧道转发记录，并同步 Gost 配置
// 适用场景：数据库中存在 tunnel.type=2 但 forward.out_port=0 或 NULL 的记录
// syncGost: 是否同步 Gost 配置（true=同步，false=仅更新数据库）
// 返回：迁移结果
func MigrateOutPortsWithSync(syncGost bool) *MigrationResult {
	result := &MigrationResult{}

	// 1. 查找所有隧道转发类型的隧道
	var tunnels []model.Tunnel
	global.DB.Where("type = 2").Find(&tunnels)

	if len(tunnels) == 0 {
		return result
	}

	// 2. 对每个隧道，查找缺少 OutPort 的转发
	for _, tunnel := range tunnels {
		var forwards []model.Forward
		global.DB.Where("tunnel_id = ? AND (out_port = 0 OR out_port IS NULL)", tunnel.ID).Find(&forwards)

		if len(forwards) == 0 {
			continue
		}

		// 检查节点状态
		var inNode, outNode model.Node
		global.DB.First(&inNode, tunnel.InNodeId)
		global.DB.First(&outNode, tunnel.OutNodeId)

		nodesOnline := inNode.Status == 1 && outNode.Status == 1

		if syncGost && !nodesOnline {
			fmt.Printf("[迁移] 隧道 %s (ID=%d): 节点离线，跳过 %d 个转发\n", tunnel.Name, tunnel.ID, len(forwards))
			result.SkippedCount += len(forwards)
			continue
		}

		fmt.Printf("[迁移] 隧道 %s (ID=%d) 有 %d 个转发需要分配 OutPort\n", tunnel.Name, tunnel.ID, len(forwards))

		// 3. 为每个缺少 OutPort 的转发分配端口
		for _, forward := range forwards {
			outPort, err := findFreePortForNode(tunnel.OutNodeId, forward.ID)
			if err != nil {
				errMsg := fmt.Sprintf("隧道 %s 转发 %s (ID=%d): 分配 OutPort 失败 - %v",
					tunnel.Name, forward.Name, forward.ID, err)
				result.Errors = append(result.Errors, errMsg)
				continue
			}

			// 4. 更新数据库
			if err := global.DB.Model(&forward).Update("out_port", outPort).Error; err != nil {
				errMsg := fmt.Sprintf("隧道 %s 转发 %s (ID=%d): 更新 OutPort 失败 - %v",
					tunnel.Name, forward.Name, forward.ID, err)
				result.Errors = append(result.Errors, errMsg)
				continue
			}

			// 更新内存中的 forward 对象
			forward.OutPort = outPort

			// 5. 如果需要同步 Gost 配置
			if syncGost && nodesOnline {
				if err := syncForwardGostConfig(&forward, &tunnel, &inNode, &outNode); err != nil {
					errMsg := fmt.Sprintf("隧道 %s 转发 %s (ID=%d): Gost 同步失败 - %v",
						tunnel.Name, forward.Name, forward.ID, err)
					result.Errors = append(result.Errors, errMsg)
					// 不 continue，数据库已更新成功
				} else {
					fmt.Printf("  ✅ 转发 %s (ID=%d): OutPort=%d [Gost 已同步]\n", forward.Name, forward.ID, outPort)
				}
			} else {
				fmt.Printf("  ✅ 转发 %s (ID=%d): OutPort=%d [仅数据库]\n", forward.Name, forward.ID, outPort)
			}

			result.MigratedCount++
		}
	}

	return result
}

// MigrateOutPorts 迁移历史数据（仅更新数据库，不同步 Gost）
// 保持向后兼容
func MigrateOutPorts() (int, []string) {
	result := MigrateOutPortsWithSync(false)
	return result.MigratedCount, result.Errors
}

// syncForwardGostConfig 同步单个转发的 Gost 配置
func syncForwardGostConfig(forward *model.Forward, tunnel *model.Tunnel, inNode *model.Node, outNode *model.Node) error {
	// 获取 UserTunnel 用于构建 serviceName
	var userTunnel model.UserTunnel
	global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)

	serviceName := buildMigrationServiceName(forward.ID, forward.UserId, &userTunnel)

	// 隧道转发需要：Chain + RemoteService + Service
	// 1. 添加/更新 Chain（入口节点 -> 出口节点）
	remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, forward.OutPort)
	if strings.Contains(tunnel.OutIp, ":") {
		remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, forward.OutPort)
	}

	// 尝试更新 Chain，如果不存在则添加
	chainRes := utils.UpdateChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName)
	if chainRes.Msg != "OK" {
		if strings.Contains(chainRes.Msg, "not found") {
			chainRes = utils.AddChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName)
		}
		if chainRes.Msg != "OK" {
			return fmt.Errorf("Chain 同步失败: %s", chainRes.Msg)
		}
	}

	// 2. 添加/更新 RemoteService（出口节点监听）
	remoteRes := utils.UpdateRemoteService(outNode.ID, serviceName, forward.OutPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName)
	if remoteRes.Msg != "OK" {
		if strings.Contains(remoteRes.Msg, "not found") {
			remoteRes = utils.AddRemoteService(outNode.ID, serviceName, forward.OutPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName)
		}
		if remoteRes.Msg != "OK" {
			return fmt.Errorf("RemoteService 同步失败: %s", remoteRes.Msg)
		}
	}

	// 3. 更新入口服务（可能需要更新 Chain 引用）
	var limiter *int
	if userTunnel.ID != 0 {
		limiter = &userTunnel.SpeedId
	}
	serviceRes := utils.UpdateService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, "")
	if serviceRes.Msg != "OK" {
		if strings.Contains(serviceRes.Msg, "not found") {
			serviceRes = utils.AddService(inNode.ID, serviceName, forward.InPort, limiter, forward.RemoteAddr, tunnel.Type, *tunnel, forward.Strategy, "")
		}
		if serviceRes.Msg != "OK" {
			return fmt.Errorf("Service 同步失败: %s", serviceRes.Msg)
		}
	}

	return nil
}

// buildMigrationServiceName 构建服务名称
func buildMigrationServiceName(forwardId int64, userId int64, userTunnel *model.UserTunnel) string {
	utId := int64(0)
	if userTunnel != nil {
		utId = int64(userTunnel.ID)
	}
	return fmt.Sprintf("%d_%d_%d", forwardId, userId, utId)
}

// findFreePortForNode 在指定节点上查找可用端口（排除指定的 forward）
func findFreePortForNode(nodeId int64, excludeForwardId int64) (int, error) {
	var node model.Node
	if err := global.DB.First(&node, nodeId).Error; err != nil {
		return 0, fmt.Errorf("节点不存在")
	}

	// 获取该节点作为出口时已使用的端口
	usedPorts := make(map[int]bool)

	// 查询所有以该节点为出口节点的隧道
	var outTunnels []int64
	global.DB.Model(&model.Tunnel{}).Where("out_node_id = ?", nodeId).Pluck("id", &outTunnels)

	if len(outTunnels) > 0 {
		var forwards []model.Forward
		global.DB.Where("tunnel_id IN ? AND id != ? AND out_port > 0", outTunnels, excludeForwardId).Find(&forwards)
		for _, f := range forwards {
			usedPorts[f.OutPort] = true
		}
	}

	// 同时检查该节点作为入口节点时的 InPort
	var inTunnels []int64
	global.DB.Model(&model.Tunnel{}).Where("in_node_id = ?", nodeId).Pluck("id", &inTunnels)

	if len(inTunnels) > 0 {
		var forwards []model.Forward
		global.DB.Where("tunnel_id IN ? AND id != ?", inTunnels, excludeForwardId).Find(&forwards)
		for _, f := range forwards {
			usedPorts[f.InPort] = true
		}
	}

	// 从节点端口范围内查找可用端口
	for port := node.PortSta; port <= node.PortEnd; port++ {
		if !usedPorts[port] {
			return port, nil
		}
	}

	return 0, fmt.Errorf("节点 %s 无可用端口", node.Name)
}

// CheckOutPortMigrationNeeded 检查是否需要迁移 OutPort
// 返回需要迁移的记录数
func CheckOutPortMigrationNeeded() int {
	var count int64

	// 查询所有隧道转发类型的隧道 ID
	var tunnelIds []int64
	global.DB.Model(&model.Tunnel{}).Where("type = 2").Pluck("id", &tunnelIds)

	if len(tunnelIds) == 0 {
		return 0
	}

	// 统计缺少 OutPort 的转发数量
	global.DB.Model(&model.Forward{}).
		Where("tunnel_id IN ? AND (out_port = 0 OR out_port IS NULL)", tunnelIds).
		Count(&count)

	return int(count)
}

// PrintMigrationReport 打印迁移报告
func PrintMigrationReport() {
	needMigration := CheckOutPortMigrationNeeded()

	if needMigration == 0 {
		fmt.Println("✅ 所有隧道转发记录的 OutPort 已正确配置，无需迁移")
		return
	}

	fmt.Printf("⚠️  发现 %d 条隧道转发记录缺少 OutPort，需要迁移\n", needMigration)
	fmt.Println("执行迁移命令:")
	fmt.Println("  仅更新数据库: service.MigrateOutPorts()")
	fmt.Println("  同步 Gost:    service.MigrateOutPortsWithSync(true)")
}
