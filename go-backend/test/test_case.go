package test

import (
	"encoding/json"
)

// TestCase 测试用例数据结构
type TestCase struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Endpoint      string            `json:"endpoint"`
	Method        string            `json:"method"`
	Headers       map[string]string `json:"headers"`
	Body          interface{}       `json:"body"`
	Expected      ExpectedResult    `json:"expected"`
	Assertions    []string          `json:"assertions"`
	RequiresAuth  bool              `json:"requiresAuth"`
	RequiresAdmin bool              `json:"requiresAdmin"`
}

// ExpectedResult 预期结果
type ExpectedResult struct {
	Status   int                    `json:"status"`
	Response map[string]interface{} `json:"response"`
	JSONPath map[string]interface{} `json:"jsonPath"`
}

// TestSuite 测试套件
type TestSuite struct {
	Module    string     `json:"module"`
	Priority  string     `json:"priority"`
	TestCases []TestCase `json:"testCases"`
}

// TestDataContainer 测试数据容器
type TestDataContainer struct {
	TestSuites []TestSuite `json:"testSuites"`
}

// ToJSON 将测试用例body转换为JSON字符串
func (tc *TestCase) ToJSON() ([]byte, error) {
	if tc.Body == nil {
		return nil, nil
	}
	return json.Marshal(tc.Body)
}
