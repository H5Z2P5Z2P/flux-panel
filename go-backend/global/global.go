package global

import (
	"fmt"
	"log"

	"go-backend/config"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB

func InitDB() {
	var dialector gorm.Dialector

	if config.AppConfig.Database.Type == "sqlite" {
		// SQLite 模式 (主要用于测试)
		// 如果 Host 为空或 :memory: 则使用内存数据库
		dsn := config.AppConfig.Database.Name
		if dsn == "" {
			dsn = "file::memory:?cache=shared"
		}
		dialector = sqlite.Open(dsn)
		fmt.Printf("✅ 使用 SQLite 数据库: %s\n", dsn)
	} else {
		// MySQL 模式 (默认)
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			config.AppConfig.Database.User,
			config.AppConfig.Database.Password,
			config.AppConfig.Database.Host,
			config.AppConfig.Database.Port,
			config.AppConfig.Database.Name,
		)
		dialector = mysql.Open(dsn)
	}

	var err error
	DB, err = gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 使用单数表名
		},
	})

	if err != nil {
		log.Fatalf("❌ 数据库连接失败: %v", err)
	}

	if config.AppConfig.Database.Type == "sqlite" {
		// SQLite 需要自动迁移表结构
		// 在这里引用 models 会导致循环引用，通常在 main 或 test 中做
		// 但为了方便，我们可以在这里做一些简单的初始化，或者留给调用者
	}

	fmt.Println("✅ 数据库连接成功")
}
