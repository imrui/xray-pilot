package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func analyticsFixture(t *testing.T) (time.Time, *TrafficService) {
	t.Helper()
	setupInstallTestEnv(t)
	for i := 1; i <= 7; i++ {
		if err := repository.DB.Create(&entity.Node{ID: uint(i), Name: fmt.Sprintf("node-%d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i <= 2; i++ {
		if err := repository.DB.Create(&entity.User{ID: uint(i), Username: fmt.Sprintf("user-%d", i), UUID: fmt.Sprintf("uuid-%d", i), Token: fmt.Sprintf("token-%d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC), NewTrafficService()
}

func TestTrafficAnalyticsBoundariesDimensionsAndFilters(t *testing.T) {
	start, svc := analyticsFixture(t)
	end := start.Add(48 * time.Hour)
	now := end.Add(time.Hour)
	zone := time.FixedZone("Taipei", 8*3600)
	samples := []entity.TrafficSample{
		{NodeID: 1, UserID: 1, PeriodEnd: start.Add(-time.Second).In(zone), UpBytes: 999},
		{NodeID: 1, UserID: 1, PeriodEnd: start.In(zone), UpBytes: 100},
		{NodeID: 1, UserID: 1, PeriodEnd: start.Add(24*time.Hour - time.Second), DownBytes: 20},
		{NodeID: 2, UserID: 1, PeriodEnd: start.Add(24 * time.Hour).In(zone), UpBytes: 50},
		{NodeID: 2, UserID: 2, PeriodEnd: start.Add(25 * time.Hour), UpBytes: 70},
		{NodeID: 1, UserID: 1, PeriodEnd: end, UpBytes: 999},
	}
	if err := repository.NewTrafficRepository().WriteCycle(samples, nil); err != nil {
		t.Fatal(err)
	}
	r := dto.TrafficRange{Start: start, End: end, Dimension: "node"}
	result, err := svc.Analytics(context.Background(), r, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpBytes != 220 || result.DownBytes != 20 || result.ActiveNodes != 2 || result.ActiveUsers != 2 {
		t.Fatalf("wrong summary: %+v", result)
	}
	if result.Interval != "day" || len(result.Series) != 2 || len(result.Series[0].Points) != 2 {
		t.Fatalf("wrong series: %+v", result.Series)
	}
	if result.Series[0].Points[0].Time != start {
		t.Fatalf("wrong local midnight: %v", result.Series[0].Points[0].Time)
	}
	var sum int64
	for _, s := range result.Series {
		for _, p := range s.Points {
			if p.UpBytes != nil {
				sum += *p.UpBytes + *p.DownBytes
			}
		}
	}
	if sum != 240 {
		t.Fatalf("series total = %d", sum)
	}
	r.Dimension = "user"
	result, err = svc.Analytics(context.Background(), r, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Ranking[0].Name != "user-1" || result.Ranking[0].UpBytes != 150 || result.Ranking[0].DownBytes != 20 {
		t.Fatalf("cross-node user aggregation: %+v", result.Ranking)
	}
	r.NodeID = 2
	r.UserID = 1
	result, err = svc.Analytics(context.Background(), r, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.UpBytes != 50 || result.ActiveUsers != 1 || result.ActiveNodes != 1 {
		t.Fatalf("intersection filter: %+v", result)
	}
	r.Dimension = "all"
	result, err = svc.Analytics(context.Background(), r, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Series) != 1 || result.Series[0].Name != "全部用量" {
		t.Fatalf("overview: %+v", result.Series)
	}
}

func TestTrafficAnalyticsZeroUnknownAndFailure(t *testing.T) {
	start, svc := analyticsFixture(t)
	err := repository.NewTrafficRepository().WriteCycle([]entity.TrafficSample{{NodeID: 1, UserID: 1, PeriodEnd: start.Add(time.Minute), DownBytes: 10}}, nil,
		entity.TrafficPoll{NodeID: 1, PeriodEnd: start.Add(time.Hour + time.Minute), Success: true},
		entity.TrafficPoll{NodeID: 1, PeriodEnd: start.Add(2*time.Hour + time.Minute), Success: false},
		entity.TrafficPoll{NodeID: 1, PeriodEnd: start.Add(2*time.Hour + 2*time.Minute), Success: true})
	if err != nil {
		t.Fatal(err)
	}
	r := dto.TrafficRange{Start: start, End: start.Add(3 * time.Hour), NodeID: 1, Dimension: "node"}
	out, err := svc.Analytics(context.Background(), r, r.End)
	if err != nil {
		t.Fatal(err)
	}
	p := out.Series[0].Points
	if len(p) != 3 || p[0].DownBytes == nil || *p[0].DownBytes != 10 || p[1].DownBytes == nil || *p[1].DownBytes != 0 || p[2].DownBytes != nil {
		t.Fatalf("zero and missing conflated: %+v", p)
	}
	if out.SuccessfulPolls != 2 || out.FailedPolls != 1 || out.LastPollAt == nil {
		t.Fatalf("poll status: %+v", out)
	}
	r.NodeID = 7
	out, err = svc.Analytics(context.Background(), r, r.End)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Ranking) != 0 || out.Series[0].Points[0].UpBytes != nil {
		t.Fatalf("empty node fabricated data: %+v", out)
	}
}

func TestTrafficAnalyticsTopFiveAndRetention(t *testing.T) {
	start, svc := analyticsFixture(t)
	samples := []entity.TrafficSample{}
	for i := 1; i <= 7; i++ {
		samples = append(samples, entity.TrafficSample{NodeID: uint(i), UserID: 1, PeriodEnd: start, UpBytes: int64(i * 10)})
	}
	if err := repository.NewTrafficRepository().WriteCycle(samples, nil); err != nil {
		t.Fatal(err)
	}
	r := dto.TrafficRange{Start: start, End: start.Add(time.Hour), Dimension: "node"}
	out, err := svc.Analytics(context.Background(), r, start.Add(100*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated || out.RetentionStart == nil || len(out.Series) != 6 || len(out.Ranking) != 7 {
		t.Fatalf("top/retention: %+v", out)
	}
	other := out.Series[5]
	if other.ID != "other" || *other.Points[0].UpBytes != 30 {
		t.Fatalf("other sum: %+v", other)
	}
	var sum int64
	for _, s := range out.Series {
		sum += *s.Points[0].UpBytes
	}
	if sum != out.UpBytes {
		t.Fatalf("series %d != summary %d", sum, out.UpBytes)
	}
}

func TestTrafficRangeValidation(t *testing.T) {
	now := time.Now()
	valid := dto.TrafficRange{Start: now.Add(-time.Hour), End: now, Dimension: "node"}
	if err := ValidateTrafficRange(valid, now); err != nil {
		t.Fatal(err)
	}
	for _, r := range []dto.TrafficRange{
		{Start: now, End: now, Dimension: "node"},
		{Start: now.Add(-91 * 24 * time.Hour), End: now, Dimension: "node"},
		{Start: now, End: now.Add(time.Hour), Dimension: "node"},
		{Start: valid.Start, End: now, Dimension: "sql"},
	} {
		if ValidateTrafficRange(r, now) == nil {
			t.Fatalf("accepted invalid range: %+v", r)
		}
	}
}
