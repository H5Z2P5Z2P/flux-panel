package main

import (
	"fmt"
	"go-backend/global"
	"go-backend/model"
	"log"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	// Initialize DB
	var err error
	global.DB, err = gorm.Open(sqlite.Open("../data/flux.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("Connection failed:", err)
	}

	userId := 1

	// Check UserTunnels
	var userTunnels []model.UserTunnel
	global.DB.Where("user_id = ?", userId).Find(&userTunnels)
	fmt.Printf("UserTunnels for user %d: %d\n", userId, len(userTunnels))
	for _, ut := range userTunnels {
		fmt.Printf("- UT: ID=%d, TunnelId=%d, Status=%d\n", ut.ID, ut.TunnelId, ut.Status)

		var tunnel model.Tunnel
		if err := global.DB.First(&tunnel, ut.TunnelId).Error; err != nil {
			fmt.Printf("  -> Tunnel %d not found: %v\n", ut.TunnelId, err)
			continue
		}
		fmt.Printf("  -> Tunnel: ID=%d, Name=%s, Status=%d, InNodeId=%d\n", tunnel.ID, tunnel.Name, tunnel.Status, tunnel.InNodeId)

		var node model.Node
		if err := global.DB.First(&node, tunnel.InNodeId).Error; err != nil {
			fmt.Printf("    -> Node %d not found: %v\n", tunnel.InNodeId, err)
			continue
		}
		fmt.Printf("    -> Node: ID=%d, Name=%s, MinPort=%d, MaxPort=%d\n", node.ID, node.Name, node.PortSta, node.PortEnd)
	}
}
