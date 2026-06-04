package entity

import "time"

// SyncLog 操作日志实体
//
// Actor 字段记录操作发起方，约定字符串格式（v0.4.0 引入、v0.5.0 多管理员落地时迁移老调用点）：
//
//	admin:<username>                  - 已登录管理员触发
//	system:scheduler:<task>           - 后台调度器自动触发（health_check / drift_check / traffic_poll / backup_run / install_token_cleanup）
//	system:install-token:<前8位>      - 节点装机脚本通过一次性 token 触发
//	system:feishu-webhook             - 飞书事件回调触发
//	system:agent:<node_id>            - 未来 v0.7.0+ agent 上报触发（占位）
//
// Actor 为空表示老调用点（未迁移）；新代码必须用 LogRepository.RecordWithActor 写入。
type SyncLog struct {
	ID         uint      `gorm:"primaryKey"            json:"id"`
	Action     string    `                             json:"action"`     // 操作类型，如 sync / keygen / toggle
	Target     string    `                             json:"target"`     // 操作目标描述
	Actor      string    `gorm:"size:64;index"         json:"actor"`      // 操作发起方，格式见上方 godoc
	Success    bool      `                             json:"success"`
	Message    string    `                             json:"message"`
	DurationMs int64     `                             json:"duration_ms"`
	CreatedAt  time.Time `                             json:"created_at"`
}
