package entity

import "time"

// SystemSetting 系统运行时配置（KV 存储，管理后台可读写）
type SystemSetting struct {
	Key       string    `gorm:"primaryKey"`
	Value     string    `gorm:"type:text"`
	UpdatedAt time.Time
}
