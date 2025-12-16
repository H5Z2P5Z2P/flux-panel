package test

import (
	"testing"
)

// TestForwardModule 转发模块集成测试
func TestForwardModule(t *testing.T) {
	InitTestEnvironment()
	helper := NewTestHelper(t)
	InitTestDatabase()
	defer CleanupTestDatabase()

	testCases, err := LoadTestCases("Forward")
	if err != nil {
		t.Fatalf("Failed to load Forward test cases: %v", err)
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t
			helper.ExecuteTestCase(tc)
		})
	}
}
