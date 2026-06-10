package tests

import (
	"go-backend/config"
	"go-backend/global"
	"go-backend/model"
	"go-backend/utils"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const (
	TestDBPath = "file:flux_test?mode=memory&cache=shared"
)

// SetupTestDB initializes an isolated in-memory DB for deterministic tests.
func SetupTestDB() {
	config.AppConfig.Database.Type = "sqlite"
	config.AppConfig.Database.Name = TestDBPath
	config.AppConfig.Server.Port = 8888

	var err error
	global.DB, err = gorm.Open(sqlite.Open(config.AppConfig.Database.Name), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		panic(err)
	}

	if err := global.DB.AutoMigrate(
		&model.User{},
		&model.Node{},
		&model.Tunnel{},
		&model.Forward{},
		&model.SpeedLimit{},
		&model.UserTunnel{},
		&model.StatisticsFlow{},
		&model.ViteConfig{},
		&model.GuestLink{},
	); err != nil {
		panic(err)
	}

	CreateTestNode(1, "Test Node")
}

// TeardownTestDB cleans up
func TeardownTestDB() {
	sqlDB, err := global.DB.DB()
	if err == nil {
		sqlDB.Close()
	}
}

func TestMain(m *testing.M) {
	// Setup
	SetupTestDB()

	// Run Tests
	code := m.Run()

	// Teardown
	TeardownTestDB()

	os.Exit(code)
}

// Helper to create a user (if needed for extra test data)
func CreateTestUser(username string, roleId int, num int, flow int64, expTime int64) *model.User {
	user := model.User{
		User:          username,
		Pwd:           utils.Md5("123456"),
		RoleId:        roleId,
		Status:        1,
		Num:           num,
		Flow:          flow,
		ExpTime:       expTime,
		FlowResetTime: 1,
		InFlow:        0,
		OutFlow:       0,
	}
	global.DB.Create(&user)
	return &user
}

// Helper to create a data tunnel
func CreateTestTunnel(name string) *model.Tunnel {
	tunnel := model.Tunnel{
		Name:      name,
		Type:      1, // 1-TCP
		Status:    1,
		InNodeId:  1,
		OutNodeId: 1,
	}
	global.DB.Create(&tunnel)
	return &tunnel
}

// Helper to create a node
func CreateTestNode(id int64, name string) *model.Node {
	node := model.Node{
		ID:         id,
		Name:       name,
		Status:     1,
		Ip:         "127.0.0.1",
		ServerIp:   "127.0.0.1",
		PortRanges: "10000-65535",
	}
	global.DB.Create(&node)
	return &node
}
