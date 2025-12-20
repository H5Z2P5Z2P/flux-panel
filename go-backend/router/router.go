package router

import (
	"go-backend/controller"
	"go-backend/middleware"
	"go-backend/websocket"

	"github.com/gin-gonic/gin"
)

func InitRouter() *gin.Engine {
	r := gin.Default()

	// CORS
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	api := r.Group("/api/v1")
	{
		userController := new(controller.UserController)
		nodeController := new(controller.NodeController)
		tunnelController := new(controller.TunnelController)
		forwardController := new(controller.ForwardController)
		openAPIController := new(controller.OpenAPIController)
		speedLimitController := new(controller.SpeedLimitController)

		// Public Routes
		api.POST("/user/login", userController.Login)

		guestController := new(controller.GuestController)
		api.GET("/guest/dashboard", guestController.GetDashboard)
		api.GET("/guest/debug_crash", guestController.DebugCrash)

		// Protected Routes
		auth := api.Group("/")
		auth.Use(middleware.Auth())
		{
			// User
			user := auth.Group("/user")
			{
				user.POST("/create", middleware.RequireRole(0), userController.Create)
				user.POST("/list", userController.List)
				user.POST("/update", userController.Update) // Check permission inside controller/service
				user.POST("/updatePassword", userController.UpdatePassword)
				user.POST("/delete", middleware.RequireRole(0), userController.Delete)
				user.POST("/package", userController.Package)
				user.POST("/reset", middleware.RequireRole(0), userController.Reset)
				user.GET("/guest_link", userController.GenerateGuestLink)
			}

			// Guest
			// user route group is for /user, we need a separate for guest?
			// The hierarchy is auth -> user.

			// Let's add Public Guest Route outside of auth group

			// Node
			node := auth.Group("/node")
			{
				node.POST("/create", middleware.RequireRole(0), nodeController.Create)
				node.POST("/list", middleware.RequireRole(0), nodeController.List)
				node.POST("/update", middleware.RequireRole(0), nodeController.Update)
				node.POST("/delete", middleware.RequireRole(0), nodeController.Delete)
				node.POST("/install", middleware.RequireRole(0), nodeController.Install)
			}

			// Tunnel
			tunnel := auth.Group("/tunnel")
			{
				tunnel.POST("/create", middleware.RequireRole(0), tunnelController.Create)
				tunnel.POST("/list", tunnelController.List) // All for admin, authorized for user (impl in service)
				tunnel.POST("/update", middleware.RequireRole(0), tunnelController.Update)
				tunnel.POST("/delete", middleware.RequireRole(0), tunnelController.Delete)

				// UserTunnel Management (Admin only)
				tunnel.POST("/user/assign", middleware.RequireRole(0), tunnelController.AssignUserTunnel)
				tunnel.POST("/user/list", middleware.RequireRole(0), tunnelController.ListUserTunnels)
				tunnel.POST("/user/remove", middleware.RequireRole(0), tunnelController.RemoveUserTunnel)
				tunnel.POST("/user/update", middleware.RequireRole(0), tunnelController.UpdateUserTunnel)

				// User visible tunnels (All users)
				tunnel.POST("/user/tunnel", tunnelController.GetUserTunnels)

				// Tunnel Diagnose (Admin only)
				tunnel.POST("/diagnose", middleware.RequireRole(0), tunnelController.DiagnoseTunnel)
			}

			// Forward
			forward := auth.Group("/forward")
			{
				forward.POST("/create", forwardController.Create)
				forward.POST("/list", forwardController.List)
				forward.POST("/update", forwardController.Update)
				forward.POST("/delete", forwardController.Delete)
				forward.POST("/pause", forwardController.Pause)
				forward.POST("/resume", forwardController.Resume)
				forward.POST("/force-delete", forwardController.ForceDelete)
				forward.POST("/diagnose", forwardController.Diagnose)
				forward.POST("/update-order", forwardController.UpdateOrder)
			}

			// System Info (WebSocket) - Auth handled internally
			// auth.GET("/system-info", websocket.HandleWebSocket)
		}

		// Speed Limit (Admin only)
		speedLimit := api.Group("/speed-limit")
		speedLimit.Use(middleware.Auth())
		speedLimit.Use(middleware.RequireRole(0))
		{
			speedLimit.POST("/create", speedLimitController.Create)
			speedLimit.POST("/list", speedLimitController.List)
			speedLimit.POST("/update", speedLimitController.Update)
			speedLimit.POST("/delete", speedLimitController.Delete)
			speedLimit.POST("/tunnels", speedLimitController.Tunnels)
		}

		// WebSocket (Public endpoint, auth inside)
		api.GET("/system-info", func(c *gin.Context) {
			websocket.HandleWebSocket(c)
		})

		// Public Settings (e.g. site title, captcha enabled)
		// No Auth needed for list/get? Java Controller uses @LogAnnotation but no @RequireRole for Get?
		// Java: @PostMapping("/list") public R getConfigs() -> No role check.
		// Java: @PostMapping("/get") public R getConfigByName(...) -> No role check.
		configGroup := api.Group("/config")
		{
			configGroup.POST("/list", controller.ViteConfig.GetConfigs)
			configGroup.POST("/get", controller.ViteConfig.GetConfigByName)

			// Admin only
			configGroup.POST("/update", middleware.Auth(), controller.ViteConfig.UpdateConfigs)
			configGroup.POST("/update-single", middleware.Auth(), controller.ViteConfig.UpdateConfig)
		}

		// Open API
		openAPI := api.Group("/open_api")
		{
			openAPI.GET("/sub_store", openAPIController.SubStore)
		}

		// Captcha
		captcha := api.Group("/captcha")
		{
			c := controller.CaptchaController{}
			captcha.POST("/check", c.Check)
			captcha.POST("/generate", c.Generate)
			captcha.POST("/verify", c.Verify)
		}
	}

	// Flow routes (Attached to root, not /api/v1)
	flowController := controller.FlowController{}
	r.POST("/flow/config", flowController.Config)
	r.POST("/flow/upload", flowController.Upload)
	r.POST("/flow/test", flowController.Test)

	// WebSocket Routes (Compatible with both /system-info and /api/v1/system-info)
	r.GET("/system-info", func(c *gin.Context) {
		websocket.HandleWebSocket(c)
	})

	return r
}
