package test

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadTestCases 加载指定模块的测试用例
func LoadTestCases(module string) ([]TestCase, error) {
	data, err := os.ReadFile("../../test-data/api_test_cases.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file: %w", err)
	}

	var container TestDataContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	for _, suite := range container.TestSuites {
		if suite.Module == module {
			return suite.TestCases, nil
		}
	}

	return []TestCase{}, nil
}

// LoadTestCase 加载单个测试用例
func LoadTestCase(testCaseID string) (*TestCase, error) {
	data, err := os.ReadFile("../test-data/api_test_cases.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file: %w", err)
	}

	var container TestDataContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	for _, suite := range container.TestSuites {
		for _, tc := range suite.TestCases {
			if tc.ID == testCaseID {
				return &tc, nil
			}
		}
	}

	return nil, fmt.Errorf("test case not found: %s", testCaseID)
}

// LoadAllTestCases 加载所有测试用例
func LoadAllTestCases() ([]TestCase, error) {
	data, err := os.ReadFile("../test-data/api_test_cases.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read test data file: %w", err)
	}

	var container TestDataContainer
	if err := json.Unmarshal(data, &container); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	var allCases []TestCase
	for _, suite := range container.TestSuites {
		allCases = append(allCases, suite.TestCases...)
	}

	return allCases, nil
}
