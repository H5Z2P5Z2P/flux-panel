package main

import (
	"fmt"

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

	// 3. 初始化路由
	r := router.InitRouter()

	// 4. 启动服务
	addr := fmt.Sprintf(":%d", config.AppConfig.Server.Port)
	fmt.Printf("🚀 Server running on %s\n", addr)
	r.Run(addr)
}
