package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/imrui/xray-pilot/internal/entity"
)

type TrafficRepository struct{}

func NewTrafficRepository() *TrafficRepository {
	return &TrafficRepository{}
}

// WriteCycle 单事务写入一次轮询的增量数据
//   - samples：本周期所有 (user, node) 的明细行
//   - totals：本周期所有用户累计（UPSERT，列加和更新 LastUpdatedAt）
//
// 任一失败整体回滚，避免明细写入但累计未更新导致后续数据漂移
func (r *TrafficRepository) WriteCycle(samples []entity.TrafficSample, totals []entity.UserTrafficTotal) error {
	if len(samples) == 0 && len(totals) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if len(samples) > 0 {
			if err := tx.CreateInBatches(&samples, 200).Error; err != nil {
				return err
			}
		}
		for _, t := range totals {
			t := t
			// UPSERT：存在则累加并更新时间戳，不存在则插入
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"up_bytes":        gorm.Expr("user_traffic_totals.up_bytes + ?", t.UpBytes),
					"down_bytes":      gorm.Expr("user_traffic_totals.down_bytes + ?", t.DownBytes),
					"last_updated_at": t.LastUpdatedAt,
				}),
			}).Create(&t).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ListTotalsByUserIDs 批量查询用户累计流量，便于 Users 列表 join
// 返回 map：user_id → total
func (r *TrafficRepository) ListTotalsByUserIDs(ids []uint) (map[uint]entity.UserTrafficTotal, error) {
	out := make(map[uint]entity.UserTrafficTotal, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []entity.UserTrafficTotal
	if err := DB.Where("user_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.UserID] = r
	}
	return out, nil
}

// TrafficSummary 仪表盘汇总指标
type TrafficSummary struct {
	TotalUp       int64 // 历史累计上行
	TotalDown     int64 // 历史累计下行
	ActiveUsers7d int64 // 过去 7 天有过流量的用户数
	LastUpdatedAt *time.Time
}

// Summary 计算仪表盘汇总指标
// 注意 ActiveUsers7d 走明细表去重，明细表过大时建议加上 retention
//
// 实现细节：SUM 与 MAX 分两次查询，避免 SQLite 把 MAX(time) 返回成 string 导致
// GORM Scan 失败；MAX 查询直接拿 time.Time 列，结构稳定可移植到 Postgres
func (r *TrafficRepository) Summary() (*TrafficSummary, error) {
	var sums struct {
		Up   int64
		Down int64
	}
	if err := DB.Model(&entity.UserTrafficTotal{}).
		Select("COALESCE(SUM(up_bytes),0) as up, COALESCE(SUM(down_bytes),0) as down").
		Scan(&sums).Error; err != nil {
		return nil, err
	}

	var latest entity.UserTrafficTotal
	var lastUpdated *time.Time
	if err := DB.Order("last_updated_at desc").Limit(1).Take(&latest).Error; err == nil && !latest.LastUpdatedAt.IsZero() {
		t := latest.LastUpdatedAt
		lastUpdated = &t
	}

	var active int64
	since := time.Now().AddDate(0, 0, -7)
	if err := DB.Model(&entity.TrafficSample{}).
		Where("period_end >= ?", since).
		Distinct("user_id").
		Count(&active).Error; err != nil {
		return nil, err
	}

	return &TrafficSummary{
		TotalUp:       sums.Up,
		TotalDown:     sums.Down,
		ActiveUsers7d: active,
		LastUpdatedAt: lastUpdated,
	}, nil
}

// PurgeSamplesBefore 删除指定时间之前的明细，做 retention
// 累计字段不受影响。返回删除条数
func (r *TrafficRepository) PurgeSamplesBefore(cutoff time.Time) (int64, error) {
	res := DB.Where("period_end < ?", cutoff).Delete(&entity.TrafficSample{})
	return res.RowsAffected, res.Error
}
