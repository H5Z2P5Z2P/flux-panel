package utils

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PortRange 表示一个端口范围 [Start, End]
type PortRange struct {
	Start int
	End   int
}

// ParsePortRanges 解析端口范围字符串
// 输入: "1080,1090,2080-3080,12300-12311"
// 输出: []PortRange{{1080,1080}, {1090,1090}, {2080,3080}, {12300,12311}}
func ParsePortRanges(input string) ([]PortRange, error) {
	if input == "" {
		return nil, fmt.Errorf("端口范围不能为空")
	}

	var ranges []PortRange
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			// 范围格式: "2080-3080"
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的端口范围格式: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("无效的起始端口: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("无效的结束端口: %s", rangeParts[1])
			}

			if start > end {
				return nil, fmt.Errorf("起始端口不能大于结束端口: %d-%d", start, end)
			}

			ranges = append(ranges, PortRange{Start: start, End: end})
		} else {
			// 单个端口: "1080"
			port, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("无效的端口: %s", part)
			}
			ranges = append(ranges, PortRange{Start: port, End: port})
		}
	}

	if len(ranges) == 0 {
		return nil, fmt.Errorf("端口范围不能为空")
	}

	return ranges, nil
}

// ValidatePortRanges 验证端口范围有效性
func ValidatePortRanges(ranges []PortRange) error {
	for _, r := range ranges {
		if r.Start < 1 || r.Start > 65535 {
			return fmt.Errorf("端口 %d 不在有效范围内 (1-65535)", r.Start)
		}
		if r.End < 1 || r.End > 65535 {
			return fmt.Errorf("端口 %d 不在有效范围内 (1-65535)", r.End)
		}
		if r.Start > r.End {
			return fmt.Errorf("起始端口 %d 不能大于结束端口 %d", r.Start, r.End)
		}
	}
	return nil
}

// ValidatePortRangesString 验证端口范围字符串
func ValidatePortRangesString(input string) error {
	ranges, err := ParsePortRanges(input)
	if err != nil {
		return err
	}
	return ValidatePortRanges(ranges)
}

// IsPortInRanges 检查端口是否在范围内
func IsPortInRanges(port int, ranges []PortRange) bool {
	for _, r := range ranges {
		if port >= r.Start && port <= r.End {
			return true
		}
	}
	return false
}

// GetAllPorts 获取所有端口列表（用于迭代查找可用端口）
func GetAllPorts(ranges []PortRange) []int {
	var ports []int
	for _, r := range ranges {
		for p := r.Start; p <= r.End; p++ {
			ports = append(ports, p)
		}
	}
	// 排序并去重
	sort.Ints(ports)
	return uniqueInts(ports)
}

func uniqueInts(sorted []int) []int {
	if len(sorted) == 0 {
		return sorted
	}
	result := []int{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i] != sorted[i-1] {
			result = append(result, sorted[i])
		}
	}
	return result
}

// FormatPortRanges 格式化为字符串
func FormatPortRanges(ranges []PortRange) string {
	var parts []string
	for _, r := range ranges {
		if r.Start == r.End {
			parts = append(parts, strconv.Itoa(r.Start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.Start, r.End))
		}
	}
	return strings.Join(parts, ",")
}

// ConvertLegacyPortRange 将旧的 PortSta/PortEnd 转换为新格式
func ConvertLegacyPortRange(portSta, portEnd int) string {
	if portSta == 0 && portEnd == 0 {
		return ""
	}
	if portSta == portEnd {
		return strconv.Itoa(portSta)
	}
	return fmt.Sprintf("%d-%d", portSta, portEnd)
}
