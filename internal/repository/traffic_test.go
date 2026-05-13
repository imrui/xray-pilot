package repository

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/entity"
)

func setupTrafficTestDB(t *testing.T) {
	t.Helper()
	if DB != nil {
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(t.TempDir(), "traffic-test.db")
	config.Global.Crypto.MasterKey = strings.Repeat("11", 32)
	if err := Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		if DB == nil {
			return
		}
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

// TestWriteCycleAccumulates 验证多次调用 WriteCycle 时累计字段正确累加
// 这是周期增量数据落地的核心语义保证
func TestWriteCycleAccumulates(t *testing.T) {
	setupTrafficTestDB(t)
	r := NewTrafficRepository()

	t0 := time.Now()
	t1 := t0.Add(5 * time.Minute)

	// 第 1 周期：alice 上 100 下 200，bob 上 50
	cycle1Samples := []entity.TrafficSample{
		{UserID: 1, NodeID: 10, PeriodEnd: t0, UpBytes: 100, DownBytes: 200},
		{UserID: 2, NodeID: 10, PeriodEnd: t0, UpBytes: 50, DownBytes: 0},
	}
	cycle1Totals := []entity.UserTrafficTotal{
		{UserID: 1, UpBytes: 100, DownBytes: 200, LastUpdatedAt: t0},
		{UserID: 2, UpBytes: 50, DownBytes: 0, LastUpdatedAt: t0},
	}
	if err := r.WriteCycle(cycle1Samples, cycle1Totals); err != nil {
		t.Fatalf("cycle1: %v", err)
	}

	// 第 2 周期：alice 又新增上 30 下 40
	cycle2Samples := []entity.TrafficSample{
		{UserID: 1, NodeID: 10, PeriodEnd: t1, UpBytes: 30, DownBytes: 40},
	}
	cycle2Totals := []entity.UserTrafficTotal{
		{UserID: 1, UpBytes: 30, DownBytes: 40, LastUpdatedAt: t1},
	}
	if err := r.WriteCycle(cycle2Samples, cycle2Totals); err != nil {
		t.Fatalf("cycle2: %v", err)
	}

	totals, err := r.ListTotalsByUserIDs([]uint{1, 2})
	if err != nil {
		t.Fatalf("list totals: %v", err)
	}
	if alice := totals[1]; alice.UpBytes != 130 || alice.DownBytes != 240 {
		t.Errorf("alice total up=%d down=%d, want 130/240", alice.UpBytes, alice.DownBytes)
	}
	if bob := totals[2]; bob.UpBytes != 50 || bob.DownBytes != 0 {
		t.Errorf("bob total up=%d down=%d, want 50/0", bob.UpBytes, bob.DownBytes)
	}

	// 明细应有 3 条
	var count int64
	DB.Model(&entity.TrafficSample{}).Count(&count)
	if count != 3 {
		t.Errorf("expected 3 sample rows, got %d", count)
	}
}

// TestSummary 验证仪表盘汇总指标
func TestSummary(t *testing.T) {
	setupTrafficTestDB(t)
	r := NewTrafficRepository()

	now := time.Now()
	samples := []entity.TrafficSample{
		{UserID: 1, NodeID: 10, PeriodEnd: now, UpBytes: 100, DownBytes: 200},
		{UserID: 2, NodeID: 10, PeriodEnd: now, UpBytes: 50, DownBytes: 80},
	}
	totals := []entity.UserTrafficTotal{
		{UserID: 1, UpBytes: 100, DownBytes: 200, LastUpdatedAt: now},
		{UserID: 2, UpBytes: 50, DownBytes: 80, LastUpdatedAt: now},
	}
	if err := r.WriteCycle(samples, totals); err != nil {
		t.Fatalf("write: %v", err)
	}

	summary, err := r.Summary()
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalUp != 150 || summary.TotalDown != 280 {
		t.Errorf("total up=%d down=%d, want 150/280", summary.TotalUp, summary.TotalDown)
	}
	if summary.ActiveUsers7d != 2 {
		t.Errorf("active 7d = %d, want 2", summary.ActiveUsers7d)
	}
}

// TestPurgeSamplesBefore 验证 retention 清理仅影响明细、不动累计
func TestPurgeSamplesBefore(t *testing.T) {
	setupTrafficTestDB(t)
	r := NewTrafficRepository()

	old := time.Now().Add(-30 * 24 * time.Hour)
	fresh := time.Now()

	samples := []entity.TrafficSample{
		{UserID: 1, NodeID: 1, PeriodEnd: old, UpBytes: 1, DownBytes: 2},
		{UserID: 1, NodeID: 1, PeriodEnd: fresh, UpBytes: 3, DownBytes: 4},
	}
	totals := []entity.UserTrafficTotal{
		{UserID: 1, UpBytes: 4, DownBytes: 6, LastUpdatedAt: fresh},
	}
	if err := r.WriteCycle(samples, totals); err != nil {
		t.Fatalf("write: %v", err)
	}

	deleted, err := r.PurgeSamplesBefore(time.Now().Add(-7 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted=%d, want 1", deleted)
	}

	// 累计字段未被影响
	totalsMap, _ := r.ListTotalsByUserIDs([]uint{1})
	if totalsMap[1].UpBytes != 4 || totalsMap[1].DownBytes != 6 {
		t.Errorf("total mutated: %+v", totalsMap[1])
	}
}
