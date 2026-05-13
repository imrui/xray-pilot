package xray

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/imrui/xray-pilot/internal/xray/statspb"
)

// UserDelta 单个用户在一次轮询周期内的增量字节数
// 通过 QueryStats(reset=true) 拉取，xray 计数器同时被清零，下一周期重新累积
type UserDelta struct {
	Email     string
	UpBytes   int64
	DownBytes int64
}

// PollUserTrafficDelta 调用远端 xray StatsService 拉取并清零所有 user>>> 维度计数
// 返回值即本周期增量，可直接累加到面板侧的累计字段
//
// xray 命名约定（见 https://xtls.github.io/en/config/stats.html）：
//
//	user>>>{email}>>>traffic>>>uplink
//	user>>>{email}>>>traffic>>>downlink
//
// {email} 即 client 配置里的 email 字段。Reset=true 语义为"先取后清零"
func PollUserTrafficDelta(ctx context.Context, conn *grpc.ClientConn) ([]UserDelta, error) {
	cli := statspb.NewStatsServiceClient(conn)

	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := cli.QueryStats(callCtx, &statspb.QueryStatsRequest{
		Pattern: "user>>>",
		Reset_:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("QueryStats: %w", err)
	}
	return parseUserStats(resp.GetStat()), nil
}

// parseUserStats 将 xray 返回的扁平 stat 列表聚合为按 email 维度的增量
// 抽离为独立函数便于单元测试
func parseUserStats(stats []*statspb.Stat) []UserDelta {
	agg := make(map[string]*UserDelta, len(stats)/2+1)
	for _, s := range stats {
		if s == nil {
			continue
		}
		// name 形如 "user>>>{email}>>>traffic>>>{uplink|downlink}"
		parts := strings.Split(s.GetName(), ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			continue
		}
		email := parts[1]
		if email == "" {
			continue
		}
		d, ok := agg[email]
		if !ok {
			d = &UserDelta{Email: email}
			agg[email] = d
		}
		switch parts[3] {
		case "uplink":
			d.UpBytes = s.GetValue()
		case "downlink":
			d.DownBytes = s.GetValue()
		}
	}
	out := make([]UserDelta, 0, len(agg))
	for _, d := range agg {
		out = append(out, *d)
	}
	return out
}
