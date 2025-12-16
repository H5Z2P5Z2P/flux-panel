# Go Backend Integration Tests

## 概述

本目录包含Go后端的集成测试套件，用于验证API行为与Java后端的一致性。

## 文件结构

```
test/
├── test_case.go           # 测试用例数据结构
├── test_loader.go         # JSON测试数据加载器
├── test_helper.go         # 测试辅助工具（HTTP执行、断言）
├── integration_test.go    # 测试套件入口
├── user_test.go          # 用户模块测试
├── node_test.go          # 节点模块测试
├── tunnel_test.go        # 隧道模块测试
├── forward_test.go       # 转发模块测试
└── speed_limit_test.go   # 限速模块测试
```

## 运行测试

### 运行所有测试
```bash
cd go-backend
go test -v ./test/... -count=1
```

### 运行单个模块测试
```bash
go test -v ./test/ -run TestUserModule -count=1
go test -v ./test/ -run TestNodeModule -count=1
```

### 运行单个测试用例
```bash
go test -v ./test/ -run "TestUserModule/管理员登录成功" -count=1
```

## 测试数据

测试用例定义在 `../test-data/api_test_cases.json` 中，与Java测试共享。

## 关键功能

### 1. 测试用例加载
- `LoadTestCases(module)` - 加载指定模块的测试用例
- `LoadTestCase(id)` - 加载单个测试用例
- `LoadAllTestCases()` - 加载所有测试用例

### 2. HTTP执行
- 自动处理请求body的JSON序列化
- 支持自定义headers
- 自动添加认证Token

### 3. 响应验证
- HTTP状态码验证
- JSON响应内容深度比较
- 支持通配符匹配 (`"*"`)
- 自定义断言支持（JSONPath风格）

### 4. 断言格式
```
$.path operator value

示例:
$.code == 0
$.data.token != null
$.data.role_id == 0
```

## 数据初始化

每个测试模块会自动：
1. 初始化测试数据库（`InitTestDatabase()`）
2. 执行测试用例
3. 清理测试数据（`CleanupTestDatabase()`）

## 示例

```go
func TestUserModule(t *testing.T) {
    helper := NewTestHelper(t)
    InitTestDatabase()
    defer CleanupTestDatabase()
    
    testCases, _ := LoadTestCases("User")
    
    for _, tc := range testCases {
        t.Run(tc.Name, func(t *testing.T) {
            helper.t = t
            helper.ExecuteTestCase(tc)
        })
    }
}
```

## 注意事项

1. 每次测试后会清理数据库，保证测试隔离性
2. 需要在测试环境运行，避免影响生产数据
3. 测试需要数据库连接配置正确
