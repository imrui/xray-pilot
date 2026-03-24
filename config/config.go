package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	JWT       JWTConfig       `mapstructure:"jwt"`
	Crypto    CryptoConfig    `mapstructure:"crypto"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Driver string `mapstructure:"driver"`
	DSN    string `mapstructure:"dsn"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
	Expire int    `mapstructure:"expire"` // hours
}

type CryptoConfig struct {
	MasterKey string `mapstructure:"master_key"`
}

// SchedulerConfig 定时任务配置
type SchedulerConfig struct {
	// DriftCheckInterval 漂移检测间隔（秒），0 表示禁用
	DriftCheckInterval int `mapstructure:"drift_check_interval"`
	// HealthCheckInterval 健康检测间隔（秒），0 表示禁用
	HealthCheckInterval int `mapstructure:"health_check_interval"`
}

var Global Config

// Load 加载配置，支持环境变量覆盖
func Load() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// 设置默认值
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.dsn", "xray-pilot.db")
	viper.SetDefault("jwt.expire", 24)
	viper.SetDefault("scheduler.drift_check_interval", 300)   // 5 分钟
	viper.SetDefault("scheduler.health_check_interval", 120)  // 2 分钟

	// 支持环境变量覆盖，例如 XRAY_PILOT_SERVER_PORT
	viper.SetEnvPrefix("XRAY_PILOT")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	if err := viper.Unmarshal(&Global); err != nil {
		return fmt.Errorf("解析配置失败: %w", err)
	}

	// MasterKey 三级优先级
	// 1. 环境变量 XRAY_PILOT_MASTER_KEY
	// 2. config.yaml 中 crypto.master_key
	// 3. 首次启动自动生成并打印提示
	if envKey := os.Getenv("XRAY_PILOT_MASTER_KEY"); envKey != "" {
		Global.Crypto.MasterKey = envKey
	}
	if Global.Crypto.MasterKey == "" {
		key, err := generateMasterKey()
		if err != nil {
			return fmt.Errorf("生成 MasterKey 失败: %w", err)
		}
		Global.Crypto.MasterKey = key
		fmt.Printf(`
╔══════════════════════════════════════════════════════════════╗
║  [xray-pilot] 首次启动，已自动生成 MasterKey               ║
║  请将以下 Key 写入 config.yaml 的 crypto.master_key 字段   ║
║  或设置环境变量 XRAY_PILOT_MASTER_KEY                      ║
║                                                              ║
║  %s  ║
╚══════════════════════════════════════════════════════════════╝
`, key)
	}

	return nil
}

func generateMasterKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
