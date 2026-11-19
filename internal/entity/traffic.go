package entity

import "time"

// DeletedNode 保留节点删除时的身份，供历史流量使用。
type DeletedNode struct {
	NodeID    uint `gorm:"primaryKey;autoIncrement:false"`
	Name      string
	CreatedAt time.Time
	DeletedAt time.Time
}

// TrafficSample 单周期、单用户、单节点的流量增量记录
//
// 由 TrafficPoller 每个调度周期写入一行。值为该周期内（period_end 之前到上次拉取之后）
// 通过 xray StatsService QueryStats(reset=true) 拉到的增量字节数
//
// 索引设计：
//   - (user_id, period_end) 复合索引服务用户维度的趋势查询
//   - (node_id) 单列索引服务节点维度的聚合
type TrafficSample struct {
	ID        uint64    `gorm:"primaryKey"                                                json:"id"`
	UserID    uint      `gorm:"not null;index:idx_traffic_user_period,priority:1"        json:"user_id"`
	NodeID    uint      `gorm:"not null;index;index:idx_traffic_node_period,priority:1" json:"node_id"`
	PeriodEnd time.Time `gorm:"not null;index;index:idx_traffic_user_period,priority:2;index:idx_traffic_node_period,priority:2" json:"period_end"`
	UpBytes   int64     `gorm:"not null;default:0"                                        json:"up_bytes"`
	DownBytes int64     `gorm:"not null;default:0"                                        json:"down_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// TrafficPoll 保留零用量和失败的采集结果，不能从非零流量明细推断采集是否成功。
type TrafficPoll struct {
	ID        uint64    `gorm:"primaryKey"`
	NodeID    uint      `gorm:"not null;index:idx_poll_node_period,priority:1"`
	PeriodEnd time.Time `gorm:"not null;index;index:idx_poll_node_period,priority:2"`
	Success   bool      `gorm:"not null"`
}

// UserTrafficTotal 用户累计流量（跨节点合并）
//
// 每用户一行，每周期 UPSERT 累加。LastUpdatedAt 反映最近一次成功累加的时刻
// 不持久化"按节点拆分的累计"，节点维度的累计由 TrafficSample 聚合得到
type UserTrafficTotal struct {
	UserID        uint      `gorm:"primaryKey"        json:"user_id"`
	UpBytes       int64     `gorm:"not null;default:0" json:"up_bytes"`
	DownBytes     int64     `gorm:"not null;default:0" json:"down_bytes"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}
