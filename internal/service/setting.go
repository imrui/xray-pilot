package service

import (
	"strconv"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/repository"
)

// 配置键常量
const (
	KeySchedulerDriftInterval   = "scheduler.drift_check_interval"
	KeySchedulerHealthInterval  = "scheduler.health_check_interval"
	KeySchedulerTrafficInterval = "scheduler.traffic_poll_interval"
	KeyBackupDir                = "backup.dir"
	KeyBackupIntervalHours      = "backup.interval_hours"
	KeyBackupRetentionDays      = "backup.retention_days"
	KeySSHDefaultPort           = "ssh.default_port"
	KeySSHDefaultUser           = "ssh.default_user"
	KeySSHDefaultKeyPath        = "ssh.default_key_path"
	KeySSHKnownHostsPath        = "ssh.known_hosts_path"
	KeySubscriptionBaseURL      = "subscription.base_url"
	KeySubscriptionRemarkFormat = "subscription.remark_format"
	KeyFeishuEnabled            = "feishu.enabled"
	KeyFeishuAppID              = "feishu.app_id"
	KeyFeishuAppSecret          = "feishu.app_secret"
	KeyFeishuVerificationToken  = "feishu.verification_token"
	KeyFeishuEncryptKey         = "feishu.encrypt_key"
	KeyFeishuBaseURL            = "feishu.base_url"
	KeyFeishuBotName            = "feishu.bot_name"
	KeyXrayLogAccess            = "xray.log_access"
	KeyXrayLogError             = "xray.log_error"
	KeyXrayLogLevel             = "xray.log_level"
)

// 硬编码默认值（三级优先级最后兜底）
var settingDefaults = map[string]string{
	KeySchedulerDriftInterval:   "300",
	KeySchedulerHealthInterval:  "120",
	KeySchedulerTrafficInterval: "300",
	KeyBackupDir:                "data/backup",
	KeyBackupIntervalHours:      "24",
	KeyBackupRetentionDays:      "30",
	KeySSHDefaultPort:           "22",
	KeySSHDefaultUser:           "root",
	KeySSHDefaultKeyPath:        "",
	KeySSHKnownHostsPath:        "/var/lib/xray-pilot/known_hosts",
	KeySubscriptionBaseURL:      "",
	KeySubscriptionRemarkFormat: "{node_name} ({username}) [{protocol} - {transport}]",
	KeyFeishuEnabled:            "false",
	KeyFeishuAppID:              "",
	KeyFeishuAppSecret:          "",
	KeyFeishuVerificationToken:  "",
	KeyFeishuEncryptKey:         "",
	KeyFeishuBaseURL:            "",
	KeyFeishuBotName:            "xray-pilot",
	KeyXrayLogAccess:            "none",
	KeyXrayLogError:             "/var/log/xray/error.log",
	KeyXrayLogLevel:             "warning",
}

// SettingService 系统运行时配置服务
type SettingService struct {
	repo *repository.SettingRepository
}

func NewSettingService() *SettingService {
	return &SettingService{repo: repository.NewSettingRepository()}
}

// Get 读取配置值：DB → 硬编码默认
func (s *SettingService) Get(key string) string {
	if v, ok := s.repo.Get(key); ok {
		return v
	}
	return settingDefaults[key]
}

// GetInt 读取整型配置值
func (s *SettingService) GetInt(key string) int {
	v, _ := strconv.Atoi(s.Get(key))
	return v
}

// GetAll 返回所有配置（DB 值覆盖默认值）
func (s *SettingService) GetAll() map[string]string {
	result := make(map[string]string, len(settingDefaults))
	for k, v := range settingDefaults {
		result[k] = v
	}
	if db, err := s.repo.GetAll(); err == nil {
		for k, v := range db {
			result[k] = v
		}
	}
	return result
}

// BatchUpdate 批量更新配置（仅允许已知 key）
func (s *SettingService) BatchUpdate(kv map[string]string) error {
	filtered := make(map[string]string, len(kv))
	for k, v := range kv {
		if _, known := settingDefaults[k]; known {
			filtered[k] = v
		}
	}
	return s.repo.BatchSet(filtered)
}

// SeedFromConfig 将 config.yaml 中的运行时配置写入 DB（仅首次启动，已有则跳过）
func (s *SettingService) SeedFromConfig() {
	cfg := config.Global
	seeds := map[string]string{
		KeySchedulerDriftInterval:  strconv.Itoa(cfg.Scheduler.DriftCheckInterval),
		KeySchedulerHealthInterval: strconv.Itoa(cfg.Scheduler.HealthCheckInterval),
		KeySSHDefaultPort:          strconv.Itoa(cfg.SSH.DefaultPort),
		KeySSHDefaultUser:          cfg.SSH.DefaultUser,
		KeySSHDefaultKeyPath:       cfg.SSH.DefaultKeyPath,
		KeySSHKnownHostsPath:       cfg.SSH.KnownHostsPath,
		KeySubscriptionBaseURL:     cfg.Subscription.BaseURL,
	}
	// remark_format 仅在 config.yaml 配置了非旧默认值时才种入
	if rf := cfg.Subscription.RemarkFormat; rf != "" && rf != "{region}-{name}" {
		seeds[KeySubscriptionRemarkFormat] = rf
	}
	for k, v := range seeds {
		_ = s.repo.SetIfAbsent(k, v)
	}
}
