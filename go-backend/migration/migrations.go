package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// migrate001TunnelOutPort 添加 tunnel.out_port 列
func migrate001TunnelOutPort(db *gorm.DB) error {
	// 检查列是否已存在
	if columnExists(db, "tunnel", "out_port") {
		return nil
	}

	// SQLite 语法
	return db.Exec("ALTER TABLE tunnel ADD COLUMN out_port INTEGER DEFAULT 0").Error
}

// migrate002NodePortRanges 将 port_sta/port_end 迁移到 port_ranges
func migrate002NodePortRanges(db *gorm.DB) error {
	// 1. 检查 port_ranges 列是否已存在
	if !columnExists(db, "node", "port_ranges") {
		if err := db.Exec("ALTER TABLE node ADD COLUMN port_ranges TEXT DEFAULT ''").Error; err != nil {
			return fmt.Errorf("failed to add port_ranges column: %w", err)
		}
	}

	// 2. 将旧数据迁移到新格式
	// 使用 CASE 处理 port_sta == port_end 的情况
	migrateSql := `
		UPDATE node 
		SET port_ranges = CASE 
			WHEN port_sta = port_end THEN CAST(port_sta AS TEXT)
			ELSE CAST(port_sta AS TEXT) || '-' || CAST(port_end AS TEXT)
		END
		WHERE port_ranges = '' OR port_ranges IS NULL
	`
	if err := db.Exec(migrateSql).Error; err != nil {
		return fmt.Errorf("failed to migrate port data: %w", err)
	}

	return nil
}

// columnExists 检查列是否存在 (SQLite)
func columnExists(db *gorm.DB, tableName, columnName string) bool {
	var count int64
	db.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", tableName, columnName).Scan(&count)
	return count > 0
}

// migrate003ChainTunnel 迁移隧道数据到新的链路节点表
func migrate003ChainTunnel(db *gorm.DB) error {
	// 1. 创建新表
	type TunnelNode struct {
		ID            int64  `gorm:"primaryKey;autoIncrement"`
		TunnelId      int64  `gorm:"index"`
		NodeId        int64  `gorm:"index"`
		Type          int    // 1: 入口, 2: 中转, 3: 出口
		Inx           int    // 0, 1, 2...
		Port          int    // 对于中继和出口
		Protocol      string // 节点协议
		Strategy      string // 负载策略
		TcpListenAddr string
		UdpListenAddr string
		InterfaceName string
	}

	if err := db.AutoMigrate(&TunnelNode{}); err != nil {
		return fmt.Errorf("failed to auto migrate tunnel_node: %w", err)
	}

	// 2. 检查是否有存量数据需要迁移
	var totalTunnels int64
	db.Table("tunnel").Count(&totalTunnels)
	if totalTunnels == 0 {
		return nil
	}

	// 3. 迁移数据
	rows, err := db.Table("tunnel").Rows()
	if err != nil {
		return fmt.Errorf("failed to query tunnels: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t struct {
			ID            int64
			InNodeId      int64
			OutNodeId     int64
			Type          int
			Protocol      string
			TcpListenAddr string
			UdpListenAddr string
			InterfaceName string
			OutPort       int
		}
		if err := db.ScanRows(rows, &t); err != nil {
			continue
		}

		// 检查该隧道是否已有节点（防止重复迁移）
		var nodeCount int64
		db.Table("tunnel_node").Where("tunnel_id = ?", t.ID).Count(&nodeCount)
		if nodeCount > 0 {
			continue
		}

		// 创建入口节点 (inx=0, type=1始终存在)
		entryNode := TunnelNode{
			TunnelId:      t.ID,
			NodeId:        t.InNodeId,
			Type:          1,
			Inx:           0,
			Protocol:      t.Protocol, // 默认继承隧道协议
			TcpListenAddr: t.TcpListenAddr,
			UdpListenAddr: t.UdpListenAddr,
			InterfaceName: t.InterfaceName,
		}
		if err := db.Create(&entryNode).Error; err != nil {
			return fmt.Errorf("failed to create entry node for tunnel %d: %w", t.ID, err)
		}

		// 如果是 Type 2 隧道，创建出口节点 (inx=1, type=3)
		if t.Type == 2 {
			exitNode := TunnelNode{
				TunnelId: t.ID,
				NodeId:   t.OutNodeId,
				Type:     3,
				Inx:      1,
				Port:     t.OutPort,
				Protocol: "relay", // 出口节点默认为 relay
			}
			if err := db.Create(&exitNode).Error; err != nil {
				return fmt.Errorf("failed to create exit node for tunnel %d: %w", t.ID, err)
			}
		}
	}

	return nil
}
