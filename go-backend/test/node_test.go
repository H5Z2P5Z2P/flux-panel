package test

import (
	"testing"
)

// TestNodeModule 节点模块集成测试
func TestNodeModule(t *testing.T) {
	InitTestEnvironment()
	helper := NewTestHelper(t)
	InitTestDatabase()
	defer CleanupTestDatabase()

	testCases, err := LoadTestCases("Node")
	if err != nil {
		t.Fatalf("Failed to load Node test cases: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t
			helper.ExecuteTestCase(tc)
		})
	}
}
