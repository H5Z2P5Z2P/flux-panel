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
