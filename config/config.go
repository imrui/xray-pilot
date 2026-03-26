package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/spf13/viper"
)

type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	JWT          JWTConfig          `mapstructure:"jwt"`
	Crypto       CryptoConfig       `mapstructure:"crypto"`
	Scheduler    SchedulerConfig    `mapstructure:"scheduler"`
	Admins       []AdminUser        `mapstructure:"admins"`
	SSH          SSHConfig          `mapstructure:"ssh"`
	Subscription SubscriptionConfig `mapstructure:"subscription"`
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

// AdminUser 管理员账号（存储在 config.yaml，不入库）
type AdminUser struct {
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`      // 明文密码（启动时自动 bcrypt hash）
	PasswordHash string `mapstructure:"password_hash"` // bcrypt 哈希（优先使用）
}

// SSHConfig SSH 默认参数
type SSHConfig struct {
	DefaultPort    int    `mapstructure:"default_port"`
	DefaultUser    string `mapstructure:"default_user"`
	DefaultKeyPath string `mapstructure:"default_key_path"`
}

// SubscriptionConfig 订阅相关配置
type SubscriptionConfig struct {
	BaseURL      string `mapstructure:"base_url"`       // 订阅链接前缀（空则动态获取）
	RemarkFormat string `mapstructure:"remark_format"`   // 备注格式，如 "{region}-{name}"
}

var Global Config

// Load 加载配置，支持环境变量覆盖
func Load() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")

	// 设置默认值
	viper.SetDefault("server.port", 2026)
	viper.SetDefault("server.mode", "debug")
	viper.SetDefault("database.driver", "sqlite")
	viper.SetDefault("database.dsn", "xray-pilot.db")
	viper.SetDefault("jwt.expire", 24)
	viper.SetDefault("scheduler.drift_check_interval", 300)  // 5 分钟
	viper.SetDefault("scheduler.health_check_interval", 120) // 2 分钟
	viper.SetDefault("ssh.default_port", 22)
	viper.SetDefault("ssh.default_user", "root")
	viper.SetDefault("subscription.remark_format", "{region}-{name}")

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

	// 若未配置管理员，自动添加默认管理员（仅开发用）
	if len(Global.Admins) == 0 {
		fmt.Println("[xray-pilot] 警告：未配置 admins，将使用默认管理员 admin/admin，请在 config.yaml 中修改")
		Global.Admins = []AdminUser{{Username: "admin", Password: "admin"}}
	}

	// 对明文密码进行 bcrypt hash（内存操作，不写回文件）
	if err := HashAdminPasswords(); err != nil {
		return fmt.Errorf("哈希管理员密码失败: %w", err)
	}

	return nil
}

// HashAdminPasswords 将 config.yaml 中明文密码在内存中哈希
func HashAdminPasswords() error {
	for i, admin := range Global.Admins {
		if admin.Password != "" {
			hash, err := bcrypt.GenerateFromPassword([]byte(admin.Password), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("管理员 %s: %w", admin.Username, err)
			}
			Global.Admins[i].PasswordHash = string(hash)
			Global.Admins[i].Password = "" // 清除明文
		}
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
