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

// Node 节点实体（SSH 管理目标，不含协议配置）
// 协议配置由 InboundProfile + NodeProfileKey 管理
type Node struct {
	ID     uint   `gorm:"primaryKey"`
	Name   string `gorm:"not null"`
	Region string
	IP     string // 服务器 IP（SSH 连接使用）
	Domain string // 可选连接域名（订阅 URI 优先使用，为空则用 IP）

	// XrayActive 当前节点 Xray 运行状态（由健康检测更新）
	XrayActive  bool
	XrayVersion string // 远端 Xray 版本，同步时更新

	SSHPort    int    `gorm:"default:22"`
	SSHUser    string `gorm:"default:root"`
	SSHKeyPath string

	Active     bool       `gorm:"default:true"`
	ConfigHash string     // SHA256 of rendered Xray config，用于漂移检测
	SyncStatus SyncStatus `gorm:"default:pending"`

	LastSyncAt    *time.Time
	LastCheckAt   *time.Time
	LastCheckOK   bool
	LastLatencyMs int

	Remark    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConnectAddr 返回客户端订阅 URI 中使用的连接地址
// 优先使用 Domain，回退到 IP
func (n *Node) ConnectAddr() string {
	if n.Domain != "" {
		return n.Domain
	}
	return n.IP
}
