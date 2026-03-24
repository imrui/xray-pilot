package entity

import "time"

// SyncLog 操作日志实体
type SyncLog struct {
	ID         uint   `gorm:"primaryKey"`
	Action     string // 操作类型，如 sync / keygen / toggle
	Target     string // 操作目标描述
	Success    bool
	Message    string
	DurationMs int64
	CreatedAt  time.Time
}
