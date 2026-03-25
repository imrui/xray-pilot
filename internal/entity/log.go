package entity

import "time"

// SyncLog 操作日志实体
type SyncLog struct {
	ID         uint      `gorm:"primaryKey"        json:"id"`
	Action     string    `json:"action"`            // 操作类型，如 sync / keygen / toggle
	Target     string    `json:"target"`            // 操作目标描述
	Success    bool      `json:"success"`
	Message    string    `json:"message"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}
