package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	JwtSecret string
	LogDir    string
}

type ServerConfig struct {
	Port int
}

type DatabaseConfig struct {
	Type     string // mysql or sqlite
	Host     string
	Port     int
	Name     string
	User     string
	Password string
}

var AppConfig Config

func InitConfig() {
	viper.SetDefault("server.port", 6365)
	viper.SetDefault("jwt-secret", "your-secret-key")
	viper.SetDefault("log-dir", "./logs")

	// 数据库默认值(优先读取环境变量)
	viper.BindEnv("database.type", "DB_TYPE")
	viper.BindEnv("database.host", "DB_HOST")
	viper.BindEnv("database.name", "DB_NAME")
	viper.BindEnv("database.user", "DB_USER")
	viper.BindEnv("database.password", "DB_PASSWORD")

	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("jwt-secret", "JWT_SECRET")
	viper.BindEnv("log-dir", "LOG_DIR")

	viper.SetDefault("database.host", "127.0.0.1")
	viper.SetDefault("database.port", 3306)
	viper.SetDefault("database.name", "flux_panel")
	viper.SetDefault("database.user", "root")
	viper.SetDefault("database.password", "123456")

	// 尝试读取配置文件，如果存在
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Printf("Error reading config file: %v\n", err)
		}
	}

	AppConfig.Server.Port = viper.GetInt("server.port")
	AppConfig.JwtSecret = viper.GetString("jwt-secret")
	AppConfig.LogDir = viper.GetString("log-dir")

	AppConfig.Database.Type = viper.GetString("database.type")
	AppConfig.Database.Host = viper.GetString("database.host")
	AppConfig.Database.Port = viper.GetInt("database.port")
	AppConfig.Database.Name = viper.GetString("database.name")
	AppConfig.Database.User = viper.GetString("database.user")
	AppConfig.Database.Password = viper.GetString("database.password")
}
