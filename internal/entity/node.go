package entity

import "time"

// SyncStatus 节点配置同步状态枚举
type SyncStatus string

const (
	SyncStatusSynced  SyncStatus = "synced"
	SyncStatusFailed  SyncStatus = "failed"
	SyncStatusDrifted SyncStatus = "drifted"
	SyncStatusPending SyncStatus = "pending"
)

// Node 节点实体
type Node struct {
	ID         uint   `gorm:"primaryKey"`
	Name       string `gorm:"not null"`
	Region     string
	IP         string
	Port       int    `gorm:"default:443"`
	PrivateKey string // AES-GCM 加密存储
	PublicKey  string
	ShortID    string
	SNI        string
	SSHPort    int    `gorm:"default:22"`
	SSHUser    string `gorm:"default:root"`
	SSHKeyPath string
	Active     bool       `gorm:"default:true"`
	ConfigHash string     // SHA256 of rendered Xray config，用于漂移检测
	SyncStatus SyncStatus `gorm:"default:pending"`
	LastSyncAt  *time.Time
	LastCheckAt *time.Time
	LastCheckOK bool
	LastLatencyMs int
	Remark     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
