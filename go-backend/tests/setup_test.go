package tests

import (
	"fmt"
	"go-backend/config"
	"go-backend/global"
	"go-backend/model"
	"go-backend/utils"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	SourceDBPath = "../../data/flux.db"
	TestDBPath   = "./flux_test.db"
)

// SetupTestDB initializes the test DB by copying the real DB or using in-memory DB
func SetupTestDB() {

	// Check if source DB exists
	if _, err := os.Stat(SourceDBPath); os.IsNotExist(err) {
		// Use in-memory SQLite database
		fmt.Println("⚠️ Source DB not found, using in-memory database")
		global.DB, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		if err != nil {
			panic(fmt.Sprintf("Failed to create in-memory DB: %v", err))
		}

		// Auto migrate tables
		global.DB.AutoMigrate(
			&model.User{},
			&model.Node{},
			&model.Tunnel{},
			&model.Forward{},
			&model.UserTunnel{},
			&model.SpeedLimit{},
			&model.ViteConfig{},
			&model.StatisticsFlow{},
		)
		fmt.Println("✅ In-memory Test DB Initialized with schema")
	} else {
		// 1. Copy real DB to test DB
		sourceFile, err := os.Open(SourceDBPath)
		if err != nil {
			panic(fmt.Sprintf("Failed to open source DB at %s: %v", SourceDBPath, err))
		}
		defer sourceFile.Close()

		destFile, err := os.Create(TestDBPath)
		if err != nil {
			panic(fmt.Sprintf("Failed to create test DB at %s: %v", TestDBPath, err))
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, sourceFile)
		if err != nil {
			panic(fmt.Sprintf("Failed to copy DB: %v", err))
		}

		// 2. Configure App to use the test DB
		absPath, _ := filepath.Abs(TestDBPath)
		config.AppConfig.Database.Type = "sqlite"
		config.AppConfig.Database.Name = absPath
		config.AppConfig.Server.Port = 8888

		// 3. Initialize GORM with the test DB
		global.DB, err = gorm.Open(sqlite.Open(config.AppConfig.Database.Name), &gorm.Config{})
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to test DB: %v", err))
		}

		// AutoMigrate to ensure new fields are added
		global.DB.AutoMigrate(
			&model.User{},
			&model.Node{},
			&model.Tunnel{},
			&model.Forward{},
			&model.UserTunnel{},
			&model.SpeedLimit{},
			&model.ViteConfig{},
			&model.StatisticsFlow{},
		)
		fmt.Println("✅ Test DB Initialized from data/flux.db")
	}

	// Verify or Create Default Node (ID: 1) for testing
	var node model.Node
	if err := global.DB.First(&node, 1).Error; err != nil {
		CreateTestNode(1, "Test Node")
	}
}

// TeardownTestDB cleans up
func TeardownTestDB() {
	sqlDB, err := global.DB.DB()
	if err == nil {
		sqlDB.Close()
	}
	// Remove temporary test DB
	os.Remove(TestDBPath)
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
		ID:     id,
		Name:   name,
		Status: 1,
		Ip:     "127.0.0.1",
	}
	global.DB.Create(&node)
	return &node
}
