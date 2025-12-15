package main

import (
	"fmt"

	"go-backend/config"
	"go-backend/global"
	"go-backend/router"
	"go-backend/service"
)

func main() {
	// 1. 初始化配置
	config.InitConfig()

	// 2. 初始化数据库
	global.InitDB()

	// Start Statistics Task
	service.StatisticsFlow.StartScheduledTask()

	// 3. 初始化路由
	r := router.InitRouter()

	// 4. 启动服务
	addr := fmt.Sprintf(":%d", config.AppConfig.Server.Port)
	fmt.Printf("🚀 Server running on %s\n", addr)
	r.Run(addr)
}
