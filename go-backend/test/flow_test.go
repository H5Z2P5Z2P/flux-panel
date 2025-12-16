package test

import (
	"testing"
)

// TestFlowModule 流量模块集成测试
func TestFlowModule(t *testing.T) {
	InitTestEnvironment()
	helper := NewTestHelper(t)
	// Flow模块测试可能不需要清理数据库，甚至可能需要保留状态
	// 但为了一致性，还是重置
	InitTestDatabase()
	defer CleanupTestDatabase()

	testCases, err := LoadTestCases("Flow")
	if err != nil {
		t.Fatalf("Failed to load Flow test cases: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t
			helper.ExecuteTestCase(tc)
		})
	}
}
