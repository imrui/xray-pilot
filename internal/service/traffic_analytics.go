package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
)

var ErrTrafficRange = errors.New("请选择有效的起止时间（最多 90 天，不能晚于当前时间）")

func ValidateTrafficRange(r dto.TrafficRange, now time.Time) error {
	if r.Start.IsZero() || r.End.IsZero() || r.Start.Unix() < 0 || !r.Start.Before(r.End) || r.End.Sub(r.Start) > 90*24*time.Hour || r.End.After(now.Add(time.Minute)) {
		return ErrTrafficRange
	}
	if r.Dimension != "all" && r.Dimension != "node" && r.Dimension != "user" {
		return errors.New("统计维度必须是 all、node 或 user")
	}
	return nil
}

func (s *TrafficService) Analytics(ctx context.Context, window dto.TrafficRange, now time.Time) (*dto.TrafficAnalytics, error) {
	if err := ValidateTrafficRange(window, now); err != nil {
		return nil, err
	}
	seconds := int64(86400)
	interval := "day"
	if window.End.Sub(window.Start) <= 24*time.Hour {
		seconds = 3600
		interval = "hour"
	}
	out := &dto.TrafficAnalytics{Start: window.Start, End: window.End, Timezone: "Asia/Shanghai", Interval: interval,
		Nodes: []dto.TrafficOption{}, Users: []dto.TrafficOption{}, Ranking: []dto.TrafficRanking{}, Series: []dto.TrafficSeries{}}
	if cutoff, ok := retentionCutoff(s.settingSvc.GetInt(KeyTrafficSampleRetentionDays), now); ok {
		out.RetentionStart = &cutoff
		out.Truncated = window.Start.Before(cutoff)
	}
	if err := s.trafficRepo.AnalyticsContext(ctx, window, out); err != nil {
		return nil, err
	}
	amounts, polls, err := s.trafficRepo.AnalyticsBuckets(ctx, window, seconds)
	if err != nil {
		return nil, err
	}
	nodes, users := map[uint]string{}, map[uint]string{}
	for _, n := range out.Nodes {
		nodes[n.ID] = n.Name
	}
	for _, u := range out.Users {
		users[u.ID] = u.Name
	}
	activeNodes, activeUsers := map[uint]bool{}, map[uint]bool{}
	totals := map[uint]dto.TrafficAmount{}
	points := map[uint]map[int64]dto.TrafficAmount{}
	for _, a := range amounts {
		if a.UpBytes+a.DownBytes == 0 {
			continue
		}
		activeNodes[a.NodeID] = true
		activeUsers[a.UserID] = true
		out.UpBytes += a.UpBytes
		out.DownBytes += a.DownBytes
		id := uint(0)
		if window.Dimension == "node" {
			id = a.NodeID
		}
		if window.Dimension == "user" {
			id = a.UserID
		}
		t := totals[id]
		t.UpBytes += a.UpBytes
		t.DownBytes += a.DownBytes
		totals[id] = t
		if points[id] == nil {
			points[id] = map[int64]dto.TrafficAmount{}
		}
		p := points[id][a.Bucket]
		p.UpBytes += a.UpBytes
		p.DownBytes += a.DownBytes
		points[id][a.Bucket] = p
	}
	out.ActiveNodes = len(activeNodes)
	out.ActiveUsers = len(activeUsers)
	for id, t := range totals {
		name := "全部用量"
		if window.Dimension == "node" {
			name = nodes[id]
			if name == "" {
				name = fmt.Sprintf("已删除节点 #%d", id)
			}
		}
		if window.Dimension == "user" {
			name = users[id]
			if name == "" {
				name = fmt.Sprintf("已删除用户 #%d", id)
			}
		}
		out.Ranking = append(out.Ranking, dto.TrafficRanking{TrafficOption: dto.TrafficOption{ID: id, Name: name}, TrafficAmount: t})
	}
	sort.Slice(out.Ranking, func(i, j int) bool {
		a, b := out.Ranking[i], out.Ranking[j]
		if a.UpBytes+a.DownBytes == b.UpBytes+b.DownBytes {
			return a.ID < b.ID
		}
		return a.UpBytes+a.DownBytes > b.UpBytes+b.DownBytes
	})
	// 仅当相关采集节点有成功证据且没有失败记录时补零。
	successes := map[int64]map[uint]bool{}
	failures := map[int64]map[uint]bool{}
	for _, p := range polls {
		if p.Success {
			out.SuccessfulPolls += p.Count
			if successes[p.Bucket] == nil {
				successes[p.Bucket] = map[uint]bool{}
			}
			successes[p.Bucket][p.NodeID] = true
		} else {
			out.FailedPolls += p.Count
			if failures[p.Bucket] == nil {
				failures[p.Bucket] = map[uint]bool{}
			}
			failures[p.Bucket][p.NodeID] = true
		}
	}
	allNodes := map[uint]bool{}
	if window.NodeID != 0 {
		allNodes[window.NodeID] = true
	} else {
		collectors, err := s.nodeRepo.FindAll()
		if err != nil {
			return nil, err
		}
		for _, n := range collectors {
			allNodes[n.ID] = true
		}
	}
	first := (window.Start.Unix() + 28800) / seconds
	last := (window.End.Add(-time.Nanosecond).Unix() + 28800) / seconds
	appendSeries := func(id, name string, ids []uint) {
		series := dto.TrafficSeries{ID: id, Name: name, Points: []dto.TrafficPoint{}}
		for bucket := first; bucket <= last; bucket++ {
			var up, down int64
			observed := false
			for _, key := range ids {
				if a, ok := points[key][bucket]; ok {
					observed = true
					up += a.UpBytes
					down += a.DownBytes
				}
			}
			required := allNodes
			if window.Dimension == "node" && !(len(ids) == 1 && ids[0] == 0) {
				required = map[uint]bool{}
				for _, key := range ids {
					required[key] = true
				}
			}
			known := len(required) > 0
			for node := range required {
				if !successes[bucket][node] || failures[bucket][node] {
					known = false
					break
				}
			}
			stamp := repository.BucketTime(bucket, seconds)
			if stamp.Before(window.Start) {
				stamp = window.Start
			}
			point := dto.TrafficPoint{Time: stamp}
			if observed || known {
				u, d := up, down
				point.UpBytes = &u
				point.DownBytes = &d
			}
			series.Points = append(series.Points, point)
		}
		out.Series = append(out.Series, series)
	}
	for i, item := range out.Ranking {
		if i == 5 {
			break
		}
		appendSeries(strconv.FormatUint(uint64(item.ID), 10), item.Name, []uint{item.ID})
	}
	if len(out.Ranking) > 5 {
		ids := []uint{}
		for _, r := range out.Ranking[5:] {
			ids = append(ids, r.ID)
		}
		appendSeries("other", "其他", ids)
	}
	if len(out.Series) == 0 {
		id := uint(0)
		if window.Dimension == "node" {
			id = window.NodeID
		}
		appendSeries("empty", "所选范围", []uint{id})
	}
	return out, nil
}
