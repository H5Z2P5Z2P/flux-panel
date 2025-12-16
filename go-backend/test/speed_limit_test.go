package test

import (
	"testing"
)

// TestSpeedLimitModule 限速模块集成测试
func TestSpeedLimitModule(t *testing.T) {
	InitTestEnvironment()
	helper := NewTestHelper(t)
	InitTestDatabase()
	defer CleanupTestDatabase()

	testCases, err := LoadTestCases("SpeedLimit")
	if err != nil {
		t.Fatalf("Failed to load SpeedLimit test cases: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t
			helper.ExecuteTestCase(tc)
		})
	}
}
