package service

import (
	"fmt"
	"strings"

	"go-backend/global"
	"go-backend/model"
	"go-backend/utils"
)

// ChainPortMigrationResult 迁移结果
type ChainPortMigrationResult struct {
	MigratedCount int      // 成功迁移的记录数
	SkippedCount  int      // 跳过的记录数（节点离线）
	Errors        []string // 错误信息
}

// MigrateTunnelChainPorts 迁移缺少 ChainPort 的隧道转发
// syncGost: 是否同步 Gost 配置
func MigrateTunnelChainPorts(syncGost bool) *ChainPortMigrationResult {
	result := &ChainPortMigrationResult{}

	// 查找所有 type=2 且 chain_port=0 的隧道
	var tunnels []model.Tunnel
	global.DB.Where("type = 2 AND (chain_port = 0 OR chain_port IS NULL)").Find(&tunnels)

	if len(tunnels) == 0 {
		fmt.Println("✅ 所有隧道转发的 ChainPort 已正确配置，无需迁移")
		return result
	}

	fmt.Printf("📦 发现 %d 个隧道需要分配 ChainPort\n", len(tunnels))

	for _, tunnel := range tunnels {
		// 检查节点状态
		var outNode model.Node
		if err := global.DB.First(&outNode, tunnel.OutNodeId).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("隧道 %s: 出口节点不存在", tunnel.Name))
			continue
		}

		if syncGost && outNode.Status != 1 {
			fmt.Printf("  ⏭️  隧道 %s: 出口节点离线，跳过\n", tunnel.Name)
			result.SkippedCount++
			continue
		}

		// 分配 ChainPort
		chainPort, err := Tunnel.allocateChainPort(outNode.ID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("隧道 %s: %v", tunnel.Name, err))
			continue
		}

		// 更新数据库
		if err := global.DB.Model(&tunnel).Update("chain_port", chainPort).Error; err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("隧道 %s: 更新失败 - %v", tunnel.Name, err))
			continue
		}

		tunnel.ChainPort = chainPort

		// 同步 Gost 配置
		if syncGost && outNode.Status == 1 {
			if err := syncTunnelGostConfig(&tunnel, &outNode); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("隧道 %s: Gost 同步失败 - %v", tunnel.Name, err))
				// 不 continue，数据库已更新成功
			} else {
				fmt.Printf("  ✅ 隧道 %s: ChainPort=%d [Gost 已同步]\n", tunnel.Name, chainPort)
			}
		} else {
			fmt.Printf("  ✅ 隧道 %s: ChainPort=%d [仅数据库]\n", tunnel.Name, chainPort)
		}

		result.MigratedCount++
	}

	return result
}

// syncTunnelGostConfig 同步隧道的 Gost 配置（更新所有使用该隧道的 Forward）
func syncTunnelGostConfig(tunnel *model.Tunnel, outNode *model.Node) error {
	var inNode model.Node
	if err := global.DB.First(&inNode, tunnel.InNodeId).Error; err != nil {
		return fmt.Errorf("入口节点不存在")
	}

	// 获取该隧道的所有转发
	var forwards []model.Forward
	global.DB.Where("tunnel_id = ?", tunnel.ID).Find(&forwards)

	for _, forward := range forwards {
		var userTunnel model.UserTunnel
		global.DB.Where("user_id = ? AND tunnel_id = ?", forward.UserId, tunnel.ID).First(&userTunnel)

		serviceName := fmt.Sprintf("%d_%d_%d", forward.ID, forward.UserId, userTunnel.ID)

		// 更新 Chain（指向新的 ChainPort）
		remoteAddr := fmt.Sprintf("%s:%d", tunnel.OutIp, tunnel.ChainPort)
		if strings.Contains(tunnel.OutIp, ":") {
			remoteAddr = fmt.Sprintf("[%s]:%d", tunnel.OutIp, tunnel.ChainPort)
		}

		chainRes := utils.UpdateChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName)
		if chainRes.Msg != "OK" {
			if strings.Contains(chainRes.Msg, "not found") {
				utils.AddChains(inNode.ID, serviceName, remoteAddr, tunnel.Protocol, tunnel.InterfaceName)
			}
		}

		// 更新 RemoteService
		remoteRes := utils.UpdateRemoteService(outNode.ID, serviceName, tunnel.ChainPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName)
		if remoteRes.Msg != "OK" {
			if strings.Contains(remoteRes.Msg, "not found") {
				utils.AddRemoteService(outNode.ID, serviceName, tunnel.ChainPort, forward.RemoteAddr, tunnel.Protocol, forward.Strategy, forward.InterfaceName)
			}
		}
	}

	return nil
}

// CheckChainPortMigrationNeeded 检查是否需要迁移 ChainPort
func CheckChainPortMigrationNeeded() int {
	var count int64
	global.DB.Model(&model.Tunnel{}).
		Where("type = 2 AND (chain_port = 0 OR chain_port IS NULL)").
		Count(&count)
	return int(count)
}

// PrintChainPortMigrationReport 打印迁移报告
func PrintChainPortMigrationReport() {
	count := CheckChainPortMigrationNeeded()
	if count == 0 {
		fmt.Println("✅ 所有隧道转发的 ChainPort 已正确配置，无需迁移")
		return
	}
	fmt.Printf("⚠️  发现 %d 个隧道需要分配 ChainPort\n", count)
	fmt.Println("\n执行迁移:")
	fmt.Println("  仅数据库:     ./go-backend migrate")
	fmt.Println("  同步 Gost:    ./go-backend migrate --sync")
}
