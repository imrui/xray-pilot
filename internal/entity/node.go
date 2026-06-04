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
	Owner  string // 所有者，标识节点来源
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

	// ConnectionMode 标识节点控制平面类型，为未来 agent 化预留扩展。
	//   ssh    - 当前默认，panel 通过 SSH push 配置（v0.4.0 全部填这个）
	//   grpc   - v0.4.5 预留，xray gRPC 实时控制面
	//   agent  - v0.7.0+ 预留，独立 xpilot-node 程序反向连接
	ConnectionMode string `gorm:"size:16;default:'ssh'"`

	// RegisteredAt 节点通过一键安装脚本注册回 panel 的时间；
	// 手工新增的节点该字段为空，用于区分接入路径。
	RegisteredAt *time.Time

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
