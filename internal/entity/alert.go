package entity

import "time"

// 告警事件类型
const (
	AlertKindNodeUnreachable  = "node_unreachable"  // SSH 端口 TCP 探测失败
	AlertKindXrayDown         = "xray_down"         // SSH 可达，但 TCP 类代理端口不通
	AlertKindSyncFailed       = "sync_failed"       // 配置同步失败
	AlertKindPanelUnreachable = "panel_unreachable" // 面板到全部节点不可达（NodeID = 0）
)

// NodeAlertState 节点 × 告警类型的状态机快照（v0.5.1）。
//
// 持久化而非放内存：面板重启后正在告警的节点不会重复告警，恢复消息也不会丢。
// Alerting 是当前判定；NotifiedAlerting 是最近一次「成功送达」的状态，
// 两者不等即待发送——送达失败下一轮自然重试，无需额外队列。
type NodeAlertState struct {
	ID               uint   `gorm:"primaryKey"`
	NodeID           uint   `gorm:"uniqueIndex:idx_alert_node_kind"` // 0 = 面板级
	Kind             string `gorm:"size:32;uniqueIndex:idx_alert_node_kind"`
	FailCount        int
	OKCount          int
	Alerting         bool
	NotifiedAlerting bool
	Since            *time.Time // 进入告警态的时间（恢复时计算持续时长）
	LastNotifiedAt   *time.Time // 重复提醒用
	LastError        string
	UpdatedAt        time.Time
}
