package test

import (
	"testing"
)

// TestUserModule 用户模块集成测试
func TestUserModule(t *testing.T) {
	// 初始化测试环境（数据库连接等）
	InitTestEnvironment()

	// 初始化测试环境
	helper := NewTestHelper(t)
	InitTestDatabase()
	defer CleanupTestDatabase()

	// 加载测试用例
	testCases, err := LoadTestCases("User")
	if err != nil {
		t.Fatalf("Failed to load User test cases: %v", err)
	}

	// 执行每个测试用例
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			helper.t = t // 更新测试上下文
			helper.ExecuteTestCase(tc)
		})
	}
}
