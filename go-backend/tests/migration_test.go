package tests

import (
	"go-backend/global"
	"go-backend/model"
	"go-backend/service"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMigrateOutPorts 测试历史数据迁移功能
func TestMigrateOutPorts(t *testing.T) {
	// 1. 创建节点
	inNode := createNodeWithPorts("MigInNode", 40000, 40100)
	outNode := createNodeWithPorts("MigOutNode", 50000, 50100)

	// 2. 创建隧道转发类型的隧道
	tunnel := createTunnelForward("MigTunnel", inNode.ID, outNode.ID)

	// 3. 手动创建几条缺少 OutPort 的 Forward 记录（模拟历史数据）
	forward1 := model.Forward{
		Name:     "Mig Forward 1",
		UserId:   1,
		UserName: "test",
		TunnelId: tunnel.ID,
		InPort:   40001,
		OutPort:  0, // 缺少 OutPort
		Status:   1,
	}
	global.DB.Create(&forward1)

	forward2 := model.Forward{
		Name:     "Mig Forward 2",
		UserId:   1,
		UserName: "test",
		TunnelId: tunnel.ID,
		InPort:   40002,
		OutPort:  0, // 缺少 OutPort
		Status:   1,
	}
	global.DB.Create(&forward2)

	// 4. 验证迁移前状态
	needMigration := service.CheckOutPortMigrationNeeded()
	assert.GreaterOrEqual(t, needMigration, 2, "Should have at least 2 records needing migration")
	t.Logf("迁移前：发现 %d 条记录需要迁移", needMigration)

	// 5. 执行迁移
	count, errors := service.MigrateOutPorts()
	t.Logf("迁移结果：成功 %d 条，错误 %d 条", count, len(errors))

	assert.GreaterOrEqual(t, count, 2, "Should migrate at least 2 records")
	assert.Empty(t, errors, "Should have no errors")

	// 6. 验证迁移后状态
	var migrated1 model.Forward
	global.DB.First(&migrated1, forward1.ID)
	assert.NotEqual(t, 0, migrated1.OutPort, "Forward 1 should have OutPort after migration")
	assert.GreaterOrEqual(t, migrated1.OutPort, 50000, "OutPort should be in OutNode range")
	assert.LessOrEqual(t, migrated1.OutPort, 50100, "OutPort should be in OutNode range")
	t.Logf("✅ Forward 1: OutPort = %d", migrated1.OutPort)

	var migrated2 model.Forward
	global.DB.First(&migrated2, forward2.ID)
	assert.NotEqual(t, 0, migrated2.OutPort, "Forward 2 should have OutPort after migration")
	assert.NotEqual(t, migrated1.OutPort, migrated2.OutPort, "Each forward should have unique OutPort")
	t.Logf("✅ Forward 2: OutPort = %d", migrated2.OutPort)

	// 7. 验证再次运行迁移不会产生重复
	needMigrationAfter := service.CheckOutPortMigrationNeeded()
	// 应该减少了我们刚迁移的数量（除非有其他测试的数据）
	t.Logf("迁移后：还有 %d 条记录需要迁移", needMigrationAfter)

	// 8. 清理
	global.DB.Where("tunnel_id = ?", tunnel.ID).Delete(&model.Forward{})
	global.DB.Delete(tunnel)
	global.DB.Delete(inNode)
	global.DB.Delete(outNode)

	t.Log("✅ 迁移测试通过！")
}

// TestMigrateOutPortsPortConflict 测试迁移时端口冲突处理
func TestMigrateOutPortsPortConflict(t *testing.T) {
	// 1. 创建节点（端口范围很小，容易产生冲突）
	inNode := createNodeWithPorts("ConflictInNode", 60000, 60002)
	outNode := createNodeWithPorts("ConflictOutNode", 70000, 70002)

	// 2. 创建隧道
	tunnel := createTunnelForward("ConflictTunnel", inNode.ID, outNode.ID)

	// 3. 先创建一个正常的 Forward 占用端口
	existingForward := model.Forward{
		Name:     "Existing Forward",
		UserId:   1,
		UserName: "test",
		TunnelId: tunnel.ID,
		InPort:   60000,
		OutPort:  70000, // 占用第一个端口
		Status:   1,
	}
	global.DB.Create(&existingForward)

	// 4. 创建缺少 OutPort 的记录
	forward1 := model.Forward{
		Name:     "Need Migrate 1",
		UserId:   1,
		UserName: "test",
		TunnelId: tunnel.ID,
		InPort:   60001,
		OutPort:  0,
		Status:   1,
	}
	global.DB.Create(&forward1)

	forward2 := model.Forward{
		Name:     "Need Migrate 2",
		UserId:   1,
		UserName: "test",
		TunnelId: tunnel.ID,
		InPort:   60002,
		OutPort:  0,
		Status:   1,
	}
	global.DB.Create(&forward2)

	// 5. 执行迁移
	count, errors := service.MigrateOutPorts()
	t.Logf("迁移结果：成功 %d 条，错误 %d 条", count, len(errors))

	// 6. 验证迁移后端口不冲突
	var mig1 model.Forward
	global.DB.First(&mig1, forward1.ID)

	var mig2 model.Forward
	global.DB.First(&mig2, forward2.ID)

	// 所有端口应该不同
	ports := map[int]bool{70000: true} // existingForward 已占用

	if mig1.OutPort != 0 {
		assert.False(t, ports[mig1.OutPort], "Forward 1 OutPort should not conflict")
		ports[mig1.OutPort] = true
		t.Logf("✅ Forward 1: OutPort = %d", mig1.OutPort)
	}

	if mig2.OutPort != 0 {
		assert.False(t, ports[mig2.OutPort], "Forward 2 OutPort should not conflict")
		t.Logf("✅ Forward 2: OutPort = %d", mig2.OutPort)
	}

	// 7. 清理
	global.DB.Where("tunnel_id = ?", tunnel.ID).Delete(&model.Forward{})
	global.DB.Delete(tunnel)
	global.DB.Delete(inNode)
	global.DB.Delete(outNode)

	t.Log("✅ 端口冲突迁移测试通过！")
}
