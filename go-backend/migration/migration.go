package migration

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// MigrationFunc 定义迁移函数类型
type MigrationFunc func(*gorm.DB) error

// Migration 表示一个迁移记录
type Migration struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"uniqueIndex;size:255"`
	AppliedAt int64
}

// migrations 按顺序定义所有迁移
var migrations = []struct {
	Name string
	Fn   MigrationFunc
}{
	{"001_tunnel_out_port", migrate001TunnelOutPort},
	{"002_node_port_ranges", migrate002NodePortRanges},
	{"003_chain_tunnel", migrate003ChainTunnel},
}

// RunMigrations 在程序启动时执行所有待处理的迁移
func RunMigrations(db *gorm.DB) error {
	// 确保 migrations 表存在
	if err := db.AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	for _, m := range migrations {
		// 检查迁移是否已执行
		var count int64
		db.Model(&Migration{}).Where("name = ?", m.Name).Count(&count)
		if count > 0 {
			continue
		}

		// 执行迁移
		log.Printf("[Migration] Applying: %s", m.Name)
		if err := m.Fn(db); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.Name, err)
		}

		// 记录迁移
		db.Create(&Migration{
			Name:      m.Name,
			AppliedAt: nowMilli(),
		})
		log.Printf("[Migration] Applied: %s", m.Name)
	}

	return nil
}

func nowMilli() int64 {
	return time.Now().UnixMilli()
}
