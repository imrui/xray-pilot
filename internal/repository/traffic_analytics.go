package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

type TrafficBucket struct {
	Bucket    int64
	NodeID    uint
	UserID    uint
	UpBytes   int64
	DownBytes int64
}

type PollBucket struct {
	Bucket  int64
	NodeID  uint
	Success bool
	Count   int64
}

// modernc 保存 Go 时区格式，其他环境使用 RFC3339。
// 秒与小数分开解析，避免 SQLite 毫秒舍入把午夜前样本计入下一天。
func sqliteInstant(column string) string {
	const zone = "CASE WHEN instr(substr(%[1]s, 20), ' ') > 0 THEN substr(%[1]s, 20 + instr(substr(%[1]s, 20), ' '), 3) || ':' || substr(%[1]s, 23 + instr(substr(%[1]s, 20), ' '), 2) WHEN substr(%[1]s, -1) = 'Z' THEN 'Z' WHEN substr(%[1]s, -6, 1) IN ('+', '-') THEN substr(%[1]s, -6) ELSE '' END"
	const instant = "(unixepoch(substr(%[1]s, 1, 19) || (" + zone + ")) * 1000000000 + CASE WHEN substr(%[1]s, 20, 1) = '.' THEN CAST(ROUND(CAST('0' || substr(%[1]s, 20) AS REAL) * 1000000000) AS INTEGER) ELSE 0 END)"
	return fmt.Sprintf(instant, column)
}

var sqliteTrafficInstant = sqliteInstant("period_end")

// 统一采用北京时区 UTC+8，在 SQL 中聚合，避免长时间范围载入全部明细。
func trafficBucketSQL(db *gorm.DB, seconds int64) string {
	if db.Dialector.Name() == "postgres" {
		return fmt.Sprintf("CAST(FLOOR((EXTRACT(EPOCH FROM period_end) + 28800) / %d) AS BIGINT)", seconds)
	}
	return fmt.Sprintf("CAST((%s / 1000000000 + 28800) / %d AS INTEGER)", sqliteTrafficInstant, seconds)
}

func trafficScope(db *gorm.DB, r dto.TrafficRange) *gorm.DB {
	if db.Dialector.Name() == "sqlite" {
		db = db.Where(sqliteTrafficInstant+" >= ? AND "+sqliteTrafficInstant+" < ?", r.Start.UnixNano(), r.End.UnixNano())
	} else {
		db = db.Where("period_end >= ? AND period_end < ?", r.Start.UTC(), r.End.UTC())
	}
	if r.NodeID != 0 {
		db = db.Where("node_id = ?", r.NodeID)
	}
	return db
}

func (r *TrafficRepository) AnalyticsBuckets(ctx context.Context, window dto.TrafficRange, seconds int64) ([]TrafficBucket, []PollBucket, error) {
	db := DB.WithContext(ctx)
	expr := trafficBucketSQL(db, seconds)
	q := trafficScope(db.Model(&entity.TrafficSample{}), window)
	if window.UserID != 0 {
		q = q.Where("user_id = ?", window.UserID)
	}
	var amounts []TrafficBucket
	if err := q.Select(expr + " AS bucket, node_id, user_id, SUM(up_bytes) AS up_bytes, SUM(down_bytes) AS down_bytes").
		Group("bucket, node_id, user_id").Scan(&amounts).Error; err != nil {
		return nil, nil, err
	}
	var polls []PollBucket
	if err := trafficScope(db.Model(&entity.TrafficPoll{}), window).
		Select(expr + " AS bucket, node_id, success, COUNT(*) AS count").Group("bucket, node_id, success").Scan(&polls).Error; err != nil {
		return nil, nil, err
	}
	return amounts, polls, nil
}

func (r *TrafficRepository) AnalyticsContext(ctx context.Context, window dto.TrafficRange, out *dto.TrafficAnalytics) error {
	db := DB.WithContext(ctx)
	if err := db.Model(&entity.Node{}).Select("id, name").Order("name, id").Scan(&out.Nodes).Error; err != nil {
		return err
	}
	if err := db.Model(&entity.User{}).Select("id, username AS name").Order("username, id").Scan(&out.Users).Error; err != nil {
		return err
	}
	out.TotalNodes = int64(len(out.Nodes))
	if err := db.Model(&entity.Node{}).Where("active = ?", true).Count(&out.EnabledNodes).Error; err != nil {
		return err
	}
	out.TotalUsers = int64(len(out.Users))
	if err := db.Model(&entity.User{}).Where("active = ?", true).Count(&out.EnabledUsers).Error; err != nil {
		return err
	}
	if err := db.Model(&entity.Node{}).Where("active = ? AND last_check_ok = ?", true, true).Count(&out.HealthyNodes).Error; err != nil {
		return err
	}
	var latest []entity.TrafficPoll
	q := db.Where("success = ?", true)
	if window.NodeID != 0 {
		q = q.Where("node_id = ?", window.NodeID)
	}
	order := "period_end DESC"
	if db.Dialector.Name() == "sqlite" {
		order = sqliteTrafficInstant + " DESC"
	}
	if err := q.Order(order).Limit(1).Find(&latest).Error; err != nil {
		return err
	}
	if len(latest) > 0 {
		stamp := latest[0].PeriodEnd
		out.LastPollAt = &stamp
	}
	return nil
}

func BucketTime(bucket, seconds int64) time.Time { return time.Unix(bucket*seconds-28800, 0).UTC() }
