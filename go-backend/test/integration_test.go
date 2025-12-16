package test

import (
	"testing"
)

// TestIntegration 集成测试套件入口
// 运行所有模块测试
func TestIntegration(t *testing.T) {
	t.Run("UserModule", TestUserModule)
	t.Run("NodeModule", TestNodeModule)
	t.Run("TunnelModule", TestTunnelModule)
	t.Run("ForwardModule", TestForwardModule)
	t.Run("SpeedLimitModule", TestSpeedLimitModule)
}
