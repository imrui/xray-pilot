package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
)

func TestAnalyticsMixedTimestampFormatsAndMidnightBoundary(t *testing.T) {
	setupTrafficTestDB(t)
	start := time.Date(2026, 11, 12, 16, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	for _, stamp := range []string{
		"2026-11-13 00:00:00 +0800 CST",
		"2026-11-12T16:00:00.1Z",
		"2026-11-13T23:59:59.999999999+08:00",
		"2026-11-13 15:59:59.999999999 +0000 UTC",
		"2026-11-13T16:00:00Z",           // exclusive end
		"2026-11-12T15:59:59.999999999Z", // before start
	} {
		if err := DB.Exec("INSERT INTO traffic_samples (node_id, user_id, period_end, up_bytes, down_bytes) VALUES (1, 1, ?, 10, 0)", stamp).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, _, err := NewTrafficRepository().AnalyticsBuckets(context.Background(), dto.TrafficRange{Start: start, End: end}, 86400)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UpBytes != 40 || !BucketTime(rows[0].Bucket, 86400).Equal(start) {
		t.Fatalf("midnight buckets: %+v", rows)
	}
	// Verify the normalization expression still uses the migration's range index.
	var plan []struct{ Detail string }
	if err := DB.Raw("EXPLAIN QUERY PLAN SELECT SUM(up_bytes) FROM traffic_samples WHERE "+sqliteTrafficInstant+" >= ? AND "+sqliteTrafficInstant+" < ?", start.UnixNano(), end.UnixNano()).Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	found := false
	for _, step := range plan {
		if strings.Contains(step.Detail, "idx_traffic_samples_instant") {
			found = true
		}
	}
	if !found {
		t.Fatalf("range query did not use time index: %+v", plan)
	}
}
