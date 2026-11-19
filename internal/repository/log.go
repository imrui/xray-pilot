package repository

import (
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

type LogRepository struct{ db *gorm.DB }

type LogListFilter struct {
	Actor         string
	Keyword       string
	ImportantOnly bool
}

func NewLogRepository() *LogRepository {
	return &LogRepository{db: DB}
}

func (r *LogRepository) WithTx(tx *gorm.DB) *LogRepository {
	return &LogRepository{db: tx}
}

func (r *LogRepository) Create(log *entity.SyncLog) error {
	return r.db.Create(log).Error
}

// RecordWithActor 写入一条带 actor 的操作日志。
// actor 字符串格式约定见 entity.SyncLog godoc。
func (r *LogRepository) RecordWithActor(action, target, actor string, success bool, msg string, durationMs int64) error {
	return r.Create(&entity.SyncLog{
		Action:     action,
		Target:     target,
		Actor:      actor,
		Success:    success,
		Message:    msg,
		DurationMs: durationMs,
	})
}

// List 分页查询；actor 非空时按前缀过滤（如 "admin:" 只看人工操作、"system:scheduler:" 只看调度器）
func (r *LogRepository) List(page, pageSize int, actor string, importantOnly ...bool) ([]entity.SyncLog, int64, error) {
	return r.ListFiltered(page, pageSize, LogListFilter{
		Actor:         actor,
		ImportantOnly: len(importantOnly) > 0 && importantOnly[0],
	})
}

func (r *LogRepository) ListFiltered(page, pageSize int, filter LogListFilter) ([]entity.SyncLog, int64, error) {
	query := r.db.Model(&entity.SyncLog{})
	if filter.ImportantOnly {
		query = query.Where("actor LIKE ? OR success = ?", "admin:%", false)
	}
	if actor := strings.TrimSpace(filter.Actor); actor != "" {
		query = query.Where("actor LIKE ? ESCAPE '\\'", escapeLike(actor)+"%")
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		pattern := "%" + escapeLike(strings.ToLower(keyword)) + "%"
		query = query.Where(
			"LOWER(action) LIKE ? ESCAPE '\\' OR LOWER(target) LIKE ? ESCAPE '\\' OR LOWER(actor) LIKE ? ESCAPE '\\' OR LOWER(message) LIKE ? ESCAPE '\\'",
			pattern, pattern, pattern, pattern,
		)
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
	result := r.db.Where("created_at < ?", cutoff).Delete(&entity.SyncLog{})
	return result.RowsAffected, result.Error
}
