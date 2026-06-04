package repository

import (
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

type LogRepository struct{}

func NewLogRepository() *LogRepository {
	return &LogRepository{}
}

func (r *LogRepository) Create(log *entity.SyncLog) error {
	return DB.Create(log).Error
}

// Record 快捷写入一条操作日志（不带 actor）
//
// Deprecated: 新代码请用 RecordWithActor 显式声明 actor。
// 该方法保留供 v0.4.0 之前的 27 处老调用点继续工作；
// v0.5.0 多管理员落地时统一迁移完毕后再删除。
func (r *LogRepository) Record(action, target string, success bool, msg string, durationMs int64) {
	_ = r.Create(&entity.SyncLog{
		Action:     action,
		Target:     target,
		Success:    success,
		Message:    msg,
		DurationMs: durationMs,
	})
}

// RecordWithActor 写入一条带 actor 的操作日志。
// actor 字符串格式约定见 entity.SyncLog godoc。
func (r *LogRepository) RecordWithActor(action, target, actor string, success bool, msg string, durationMs int64) {
	_ = r.Create(&entity.SyncLog{
		Action:     action,
		Target:     target,
		Actor:      actor,
		Success:    success,
		Message:    msg,
		DurationMs: durationMs,
	})
}

func (r *LogRepository) List(page, pageSize int) ([]entity.SyncLog, int64, error) {
	var total int64
	if err := DB.Model(&entity.SyncLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []entity.SyncLog
	offset := (page - 1) * pageSize
	err := DB.Model(&entity.SyncLog{}).Order("id desc").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

func (r *LogRepository) CleanupBefore(cutoff time.Time) (int64, error) {
	result := DB.Where("created_at < ?", cutoff).Delete(&entity.SyncLog{})
	return result.RowsAffected, result.Error
}
