package repository

import (
	"strings"
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

// List 分页查询；actor 非空时按前缀过滤（如 "admin:" 只看人工操作、"system:scheduler:" 只看调度器）
func (r *LogRepository) List(page, pageSize int, actor string) ([]entity.SyncLog, int64, error) {
	query := DB.Model(&entity.SyncLog{})
	if actor != "" {
		query = query.Where("actor LIKE ? ESCAPE '\\'", escapeLike(actor)+"%")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var logs []entity.SyncLog
	offset := (page - 1) * pageSize
	err := query.Order("id desc").Offset(offset).Limit(pageSize).Find(&logs).Error
	return logs, total, err
}

// escapeLike 转义 LIKE 通配符，让用户输入按字面匹配
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	return strings.ReplaceAll(s, "_", "\\_")
}

func (r *LogRepository) CleanupBefore(cutoff time.Time) (int64, error) {
	result := DB.Where("created_at < ?", cutoff).Delete(&entity.SyncLog{})
	return result.RowsAffected, result.Error
}
