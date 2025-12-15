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

		// Public Routes
		api.POST("/user/login", userController.Login)

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
			}

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
			}

			// Forward
			forward := auth.Group("/forward")
			{
				forward.POST("/create", forwardController.Create)
				forward.POST("/list", forwardController.List)
				forward.POST("/delete", forwardController.Delete)
			}

			// System Info (WebSocket) - Auth handled internally
			// auth.GET("/system-info", websocket.HandleWebSocket)
		}

		// WebSocket (Public endpoint, auth inside)
		api.GET("/system-info", func(c *gin.Context) {
			websocket.HandleWebSocket(c)
		})

		// Captcha
		captcha := api.Group("/captcha")
		{
			c := controller.CaptchaController{}
			captcha.POST("/check", c.Check)
			captcha.POST("/generate", c.Generate)
			captcha.POST("/verify", c.Verify)
		}
	}

	return r
}
