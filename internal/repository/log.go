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

// Record 快捷写入一条操作日志
func (r *LogRepository) Record(action, target string, success bool, msg string, durationMs int64) {
	_ = r.Create(&entity.SyncLog{
		Action:     action,
		Target:     target,
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
