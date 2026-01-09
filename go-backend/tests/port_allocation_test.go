package tests

import (
	"fmt"
	"go-backend/global"
	"go-backend/model"
	"go-backend/model/dto"
	"go-backend/service"
	"go-backend/utils"
	"strconv"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// TestPortAllocationForTunnelForward 测试隧道转发类型的端口分配逻辑
// 验证：
// 1. InPort 从入口节点分配
// 2. OutPort 从出口节点分配（隧道转发专用）
// 3. 端口分配后正确保存到数据库
// 4. 已使用端口不会被重复分配
func TestPortAllocationForTunnelForward(t *testing.T) {
	service.Forward.SkipGostSync = true

	// 1. 创建两个不同的节点（模拟入口和出口）
	inNode := createNodeWithPorts("InNode", 10000, 10100)
	outNode := createNodeWithPorts("OutNode", 20000, 20100)

	// 2. 创建隧道转发类型的隧道 (Type=2)
	tunnel := createTunnelForward("TunnelForward", inNode.ID, outNode.ID)

	// 3. 创建用户和权限
	user := CreateTestUser("port_test_user", 1, 10, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	global.DB.Create(&model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
	})

	claims := &utils.UserClaims{
		User:   user.User,
		RoleId: user.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(user.ID, 10),
		},
	}

	// 4. 创建第一个转发（自动分配端口）
	dto1 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward Auto Port 1",
		RemoteAddr: "1.1.1.1:80",
		// 不指定端口，让系统自动分配
	}
	res1 := service.Forward.CreateForward(dto1, claims)
	assert.Equal(t, 0, res1.Code, "First forward creation should succeed: "+res1.Msg)

	// 验证端口已分配
	var forward1 model.Forward
	global.DB.Where("name = ?", "Forward Auto Port 1").First(&forward1)
	assert.Equal(t, 10000, forward1.InPort, "InPort should be first available in InNode range")
	assert.Equal(t, 20000, forward1.OutPort, "OutPort should be first available in OutNode range")
	t.Logf("✅ Forward 1: InPort=%d, OutPort=%d", forward1.InPort, forward1.OutPort)

	// 5. 创建第二个转发（自动分配端口）
	dto2 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward Auto Port 2",
		RemoteAddr: "1.1.1.2:80",
	}
	res2 := service.Forward.CreateForward(dto2, claims)
	assert.Equal(t, 0, res2.Code, "Second forward creation should succeed: "+res2.Msg)

	var forward2 model.Forward
	global.DB.Where("name = ?", "Forward Auto Port 2").First(&forward2)
	assert.Equal(t, 10001, forward2.InPort, "InPort should be next available")
	assert.Equal(t, 20001, forward2.OutPort, "OutPort should be next available")
	t.Logf("✅ Forward 2: InPort=%d, OutPort=%d", forward2.InPort, forward2.OutPort)

	// 6. 创建第三个转发（指定端口）
	inPort3 := 10050
	outPort3 := 20050
	dto3 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward Specified Port",
		RemoteAddr: "1.1.1.3:80",
		InPort:     &inPort3,
		OutPort:    outPort3,
	}
	res3 := service.Forward.CreateForward(dto3, claims)
	assert.Equal(t, 0, res3.Code, "Third forward with specified ports should succeed: "+res3.Msg)

	var forward3 model.Forward
	global.DB.Where("name = ?", "Forward Specified Port").First(&forward3)
	assert.Equal(t, 10050, forward3.InPort, "InPort should be as specified")
	assert.Equal(t, 20050, forward3.OutPort, "OutPort should be as specified")
	t.Logf("✅ Forward 3: InPort=%d, OutPort=%d", forward3.InPort, forward3.OutPort)

	// 7. 尝试创建使用已占用端口的转发 -> 应该失败
	dto4 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward Conflict Port",
		RemoteAddr: "1.1.1.4:80",
		InPort:     &inPort3, // 已被 forward3 使用
	}
	res4 := service.Forward.CreateForward(dto4, claims)
	assert.NotEqual(t, 0, res4.Code, "Creating forward with occupied port should fail")
	assert.Contains(t, res4.Msg, "占用", "Error should mention port is occupied")
	t.Logf("✅ Port conflict detected: %s", res4.Msg)

	// 8. 验证 getUsedPorts 逻辑 - 通过尝试自动分配下一个端口
	dto5 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "Forward Auto Port 3",
		RemoteAddr: "1.1.1.5:80",
	}
	res5 := service.Forward.CreateForward(dto5, claims)
	assert.Equal(t, 0, res5.Code, "Next auto-allocated forward should succeed")

	var forward5 model.Forward
	global.DB.Where("name = ?", "Forward Auto Port 3").First(&forward5)
	// 应该跳过 10000, 10001, 10050 (已使用)，分配 10002
	assert.Equal(t, 10002, forward5.InPort, "InPort should skip used ports")
	assert.Equal(t, 20002, forward5.OutPort, "OutPort should skip used ports")
	t.Logf("✅ Forward 5 (auto after gap): InPort=%d, OutPort=%d", forward5.InPort, forward5.OutPort)

	// 清理测试数据
	global.DB.Where("tunnel_id = ?", tunnel.ID).Delete(&model.Forward{})
	global.DB.Delete(tunnel)
	global.DB.Delete(user)
	global.DB.Delete(inNode)
	global.DB.Delete(outNode)

	t.Log("✅ 所有端口分配测试通过！")
}

// TestPortAllocationForPortForward 测试端口转发类型（Type=1）的端口分配
// 验证 OutPort == InPort（端口转发不需要独立的出口端口）
func TestPortAllocationForPortForward(t *testing.T) {
	service.Forward.SkipGostSync = true

	// 1. 创建单节点（端口转发入口出口相同）
	node := createNodeWithPorts("PortForwardNode", 30000, 30100)

	// 2. 创建端口转发类型的隧道 (Type=1)
	tunnel := model.Tunnel{
		Name:      "PortForward",
		Type:      1,
		Status:    1,
		InNodeId:  node.ID,
		OutNodeId: node.ID, // 同一节点
	}
	global.DB.Create(&tunnel)

	// 3. 创建用户和权限
	user := CreateTestUser("port_forward_user", 1, 10, 999999, time.Now().Add(24*time.Hour).UnixMilli())
	global.DB.Create(&model.UserTunnel{
		UserId:   int(user.ID),
		TunnelId: int(tunnel.ID),
		Status:   1,
	})

	claims := &utils.UserClaims{
		User:   user.User,
		RoleId: user.RoleId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(user.ID, 10),
		},
	}

	// 4. 创建转发
	dto1 := dto.ForwardDto{
		TunnelId:   tunnel.ID,
		Name:       "PortForward Test",
		RemoteAddr: "8.8.8.8:53",
	}
	res := service.Forward.CreateForward(dto1, claims)
	assert.Equal(t, 0, res.Code, "Port forward creation should succeed: "+res.Msg)

	var forward model.Forward
	global.DB.Where("name = ?", "PortForward Test").First(&forward)
	assert.Equal(t, forward.InPort, forward.OutPort, "For Type=1, OutPort should equal InPort")
	t.Logf("✅ Port Forward: InPort=%d, OutPort=%d (should be equal)", forward.InPort, forward.OutPort)

	// 清理
	global.DB.Where("tunnel_id = ?", tunnel.ID).Delete(&model.Forward{})
	global.DB.Delete(&tunnel)
	global.DB.Delete(user)
	global.DB.Delete(node)

	t.Log("✅ 端口转发类型测试通过！")
}

// --- Helper Functions ---

// createNodeWithPorts 创建带端口范围的节点
func createNodeWithPorts(name string, portSta, portEnd int) *model.Node {
	node := model.Node{
		Name:     name,
		Status:   1,
		Ip:       "127.0.0.1",
		ServerIp: "127.0.0.1",
		PortSta:  portSta,
		PortEnd:  portEnd,
	}
	global.DB.Create(&node)
	fmt.Printf("Created Node: ID=%d, Name=%s, Ports=%d-%d\n", node.ID, name, portSta, portEnd)
	return &node
}

// createTunnelForward 创建隧道转发类型的隧道 (Type=2)
func createTunnelForward(name string, inNodeId, outNodeId int64) *model.Tunnel {
	tunnel := model.Tunnel{
		Name:      name,
		Type:      2, // 隧道转发
		Status:    1,
		InNodeId:  inNodeId,
		OutNodeId: outNodeId,
		Protocol:  "tls",
	}
	global.DB.Create(&tunnel)
	fmt.Printf("Created Tunnel: ID=%d, Name=%s, Type=2 (TunnelForward), InNode=%d, OutNode=%d\n",
		tunnel.ID, name, inNodeId, outNodeId)
	return &tunnel
}
