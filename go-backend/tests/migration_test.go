package tests

import (
	"go-backend/global"
	"go-backend/model"
	"go-backend/service"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMigrateTunnelChainPorts 测试隧道 ChainPort 迁移功能
func TestMigrateTunnelChainPorts(t *testing.T) {
	// 1. 创建节点
	inNode := createNodeWithPorts("MigInNode", 40000, 40100)
	outNode := createNodeWithPorts("MigOutNode", 50000, 50100)

	// 2. 手动创建缺少 ChainPort 的隧道（模拟历史数据）
	tunnel := model.Tunnel{
		Name:      "MigTunnel",
		Type:      2,
		Status:    1,
		InNodeId:  inNode.ID,
		OutNodeId: outNode.ID,
		Protocol:  "tls",
		ChainPort: 0, // 缺少 ChainPort
	}
	global.DB.Create(&tunnel)
	t.Logf("Created Tunnel: ID=%d, ChainPort=%d", tunnel.ID, tunnel.ChainPort)

	// 3. 验证迁移前状态
	needMigration := service.CheckChainPortMigrationNeeded()
	assert.GreaterOrEqual(t, needMigration, 1, "Should have at least 1 tunnel needing migration")
	t.Logf("迁移前：发现 %d 个隧道需要迁移", needMigration)

	// 4. 执行迁移（不同步 Gost）
	result := service.MigrateTunnelChainPorts(false)
	t.Logf("迁移结果：成功 %d 个，跳过 %d 个，错误 %d 个", result.MigratedCount, result.SkippedCount, len(result.Errors))

	assert.GreaterOrEqual(t, result.MigratedCount, 1, "Should migrate at least 1 tunnel")
	assert.Empty(t, result.Errors, "Should have no errors")

	// 5. 验证迁移后状态
	var migrated model.Tunnel
	global.DB.First(&migrated, tunnel.ID)
	assert.NotEqual(t, 0, migrated.ChainPort, "Tunnel should have ChainPort after migration")
	assert.GreaterOrEqual(t, migrated.ChainPort, 50000, "ChainPort should be in OutNode range")
	assert.LessOrEqual(t, migrated.ChainPort, 50100, "ChainPort should be in OutNode range")
	t.Logf("✅ Tunnel ChainPort = %d", migrated.ChainPort)

	// 6. 验证再次运行迁移不会产生重复
	needMigrationAfter := service.CheckChainPortMigrationNeeded()
	t.Logf("迁移后：还有 %d 个隧道需要迁移", needMigrationAfter)

	// 7. 清理
	global.DB.Delete(&tunnel)
	global.DB.Delete(inNode)
	global.DB.Delete(outNode)

	t.Log("✅ 隧道 ChainPort 迁移测试通过！")
}

// TestChainPortConflict 测试多个隧道使用同一出口节点时端口不冲突
func TestChainPortConflict(t *testing.T) {
	// 1. 创建节点（端口范围较小）
	inNode := createNodeWithPorts("ConflictInNode", 60000, 60100)
	outNode := createNodeWithPorts("ConflictOutNode", 70000, 70005) // 只有 6 个端口

	// 2. 创建多个隧道，模拟历史数据
	tunnels := []model.Tunnel{
		{Name: "Tunnel1", Type: 2, Status: 1, InNodeId: inNode.ID, OutNodeId: outNode.ID, Protocol: "tls", ChainPort: 0},
		{Name: "Tunnel2", Type: 2, Status: 1, InNodeId: inNode.ID, OutNodeId: outNode.ID, Protocol: "tls", ChainPort: 0},
		{Name: "Tunnel3", Type: 2, Status: 1, InNodeId: inNode.ID, OutNodeId: outNode.ID, Protocol: "tls", ChainPort: 0},
	}
	for i := range tunnels {
		global.DB.Create(&tunnels[i])
	}

	// 3. 执行迁移
	result := service.MigrateTunnelChainPorts(false)
	t.Logf("迁移结果：成功 %d 个", result.MigratedCount)

	// 4. 验证所有隧道的 ChainPort 不同
	usedPorts := make(map[int]bool)
	for _, tun := range tunnels {
		var migrated model.Tunnel
		global.DB.First(&migrated, tun.ID)
		if migrated.ChainPort != 0 {
			assert.False(t, usedPorts[migrated.ChainPort], "ChainPort should be unique")
			usedPorts[migrated.ChainPort] = true
		}
	}
	t.Logf("✅ 使用的端口: %v", usedPorts)

	// 5. 清理
	for _, tun := range tunnels {
		global.DB.Delete(&tun)
	}
	global.DB.Delete(inNode)
	global.DB.Delete(outNode)

	t.Log("✅ 端口冲突测试通过！")
}
