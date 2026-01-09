package main

import (
	"fmt"
	"os"

	"go-backend/config"
	"go-backend/global"
	"go-backend/model"
	"go-backend/router"
	"go-backend/service"
	"go-backend/utils"
)

func main() {
	// 1. 初始化配置
	config.InitConfig()

	// 2. 初始化数据库
	global.InitDB()

	// 3. 处理命令行参数
	if len(os.Args) > 1 {
		handleCommand(os.Args[1:])
		return
	}

	// 4. 正常启动服务
	startServer()
}

// handleCommand 处理命令行参数
func handleCommand(args []string) {
	switch args[0] {
	case "migrate":
		handleMigrate(args[1:])
	case "migrate:check":
		handleMigrateCheck()
	case "help", "-h", "--help":
		printHelp()
	default:
		fmt.Printf("❌ 未知命令: %s\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

// handleMigrate 处理迁移命令
func handleMigrate(args []string) {
	syncGost := false
	for _, arg := range args {
		if arg == "--sync" || arg == "-s" {
			syncGost = true
		}
	}

	fmt.Println("📦 开始数据迁移...")
	if syncGost {
		fmt.Println("   模式: 数据库 + Gost 配置同步")
		fmt.Println("   注意: 离线节点将被跳过")
	} else {
		fmt.Println("   模式: 仅更新数据库")
	}
	fmt.Println()

	result := service.MigrateOutPortsWithSync(syncGost)

	fmt.Println()
	fmt.Println("📊 迁移结果:")
	fmt.Printf("   ✅ 成功: %d\n", result.MigratedCount)
	fmt.Printf("   ⏭️  跳过: %d (节点离线)\n", result.SkippedCount)
	fmt.Printf("   ❌ 错误: %d\n", len(result.Errors))

	if len(result.Errors) > 0 {
		fmt.Println("\n❌ 错误详情:")
		for _, err := range result.Errors {
			fmt.Printf("   - %s\n", err)
		}
	}
}

// handleMigrateCheck 检查是否需要迁移
func handleMigrateCheck() {
	count := service.CheckOutPortMigrationNeeded()
	if count == 0 {
		fmt.Println("✅ 所有隧道转发记录的 OutPort 已正确配置，无需迁移")
	} else {
		fmt.Printf("⚠️  发现 %d 条隧道转发记录缺少 OutPort，需要迁移\n", count)
		fmt.Println("\n执行迁移:")
		fmt.Println("  仅数据库:     ./go-backend migrate")
		fmt.Println("  同步 Gost:    ./go-backend migrate --sync")
	}
}

// printHelp 打印帮助信息
func printHelp() {
	fmt.Println("Usage: go-backend [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  (无参数)       启动 HTTP 服务")
	fmt.Println("  migrate        迁移缺少 OutPort 的隧道转发记录")
	fmt.Println("    --sync, -s   同时同步 Gost 配置（离线节点跳过）")
	fmt.Println("  migrate:check  检查是否需要迁移")
	fmt.Println("  help           显示帮助信息")
}

// startServer 启动 HTTP 服务
func startServer() {
	// Start Scheduled Tasks
	service.StatisticsFlow.StartScheduledTask()
	service.Task.StartScheduledTasks()

	// Initialize Database Schema and Default Data (SQLite)
	if config.AppConfig.Database.Type == "sqlite" {
		fmt.Println("⚙️ Initializing SQLite Schema...")
		err := global.DB.AutoMigrate(
			&model.User{},
			&model.Node{},
			&model.Tunnel{},
			&model.Forward{},
			&model.SpeedLimit{},
			&model.UserTunnel{},
			&model.StatisticsFlow{},
			&model.ViteConfig{},
			&model.GuestLink{},
		)
		if err != nil {
			fmt.Printf("❌ AutoMigrate failed: %v\n", err)
		}

		// Seed Admin User
		var count int64
		global.DB.Model(&model.User{}).Count(&count)
		if count == 0 {
			fmt.Println("🌱 Seeding Default Admin User...")
			// Default: admin_user / admin_user (MD5: 3c85cdebade1c51cf64ca9f3c09d182d)
			admin := model.User{
				User:          "admin_user",
				Pwd:           utils.Md5("admin_user"),
				RoleId:        0, // Admin (Actually field is RoleId in struct but let's check model)
				Status:        1,
				CreatedTime:   1748914865000,
				UpdatedTime:   1754011744252,
				Num:           99999,
				Flow:          99999,
				ExpTime:       2727251700000,
				FlowResetTime: 1,
			}
			if err := global.DB.Create(&admin).Error; err != nil {
				fmt.Printf("❌ Failed to create admin user: %v\n", err)
			} else {
				fmt.Println("✅ Admin user created: admin_user / admin_user")
			}

			// Seed Default Config
			global.DB.Create(&model.ViteConfig{Name: "app_name", Value: "flux", CreatedTime: 1755147963000, UpdatedTime: 1755147963000})
			global.DB.Create(&model.ViteConfig{Name: "captcha_enabled", Value: "false", CreatedTime: 1755147963000, UpdatedTime: 1755147963000})
		}
	}

	// 初始化路由
	r := router.InitRouter()

	// 启动服务
	addr := fmt.Sprintf(":%d", config.AppConfig.Server.Port)
	fmt.Printf("🚀 Server running on %s\n", addr)
	r.Run(addr)
}
