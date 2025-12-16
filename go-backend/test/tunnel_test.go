package test

import (
	"testing"
)

// TestTunnelModule 隧道模块集成测试
func TestTunnelModule(t *testing.T) {
	InitTestEnvironment()
	helper := NewTestHelper(t)
	InitTestDatabase()
	defer CleanupTestDatabase()

	testCases, err := LoadTestCases("Tunnel")
	if err != nil {
		t.Fatalf("Failed to load Tunnel test cases: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t
			helper.ExecuteTestCase(tc)
		})
	}
}
