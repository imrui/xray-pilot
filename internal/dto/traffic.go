package dto

import "time"

type TrafficRange struct {
	Start     time.Time
	End       time.Time // exclusive: adjacent queries never double count a sample
	Dimension string
	NodeID    uint
	UserID    uint
}

type TrafficOption struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type TrafficAmount struct {
	UpBytes   int64 `json:"up_bytes"`
	DownBytes int64 `json:"down_bytes"`
}

type TrafficRanking struct {
	TrafficOption
	TrafficAmount
}

type TrafficPoint struct {
	Time time.Time `json:"time"`
	// nil means no evidence of a successful collection, not zero usage.
	UpBytes   *int64 `json:"up_bytes"`
	DownBytes *int64 `json:"down_bytes"`
}

type TrafficSeries struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Points []TrafficPoint `json:"points"`
}

type TrafficAnalytics struct {
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Timezone string    `json:"timezone"`
	Interval string    `json:"interval"`
	TrafficAmount
	ActiveUsers     int              `json:"active_users"`
	ActiveNodes     int              `json:"active_nodes"`
	TotalNodes      int64            `json:"total_nodes"`
	TotalUsers      int64            `json:"total_users"`
	EnabledUsers    int64            `json:"enabled_users"`
	HealthyNodes    int64            `json:"healthy_nodes"`
	SuccessfulPolls int64            `json:"successful_polls"`
	FailedPolls     int64            `json:"failed_polls"`
	LastPollAt      *time.Time       `json:"last_poll_at"`
	RetentionStart  *time.Time       `json:"retention_start"`
	Truncated       bool             `json:"truncated"`
	Nodes           []TrafficOption  `json:"nodes"`
	Users           []TrafficOption  `json:"users"`
	Ranking         []TrafficRanking `json:"ranking"`
	Series          []TrafficSeries  `json:"series"`
}
