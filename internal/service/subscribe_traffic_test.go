package service

import (
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// TestSubscribePageDataIncludesTraffic 验证有累计流量的用户在订阅信息页拿到
// 预格式化好的人类可读字段
func TestSubscribePageDataIncludesTraffic(t *testing.T) {
	setupServiceTestDB(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "hilo",
		UUID:      "00000000-0000-0000-0000-000000000001",
		Token:     "traffic-token-1",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	// 直接写一条 UserTrafficTotal（模拟 TrafficPoller 已经跑过）
	updated := time.Now().Add(-3 * time.Minute)
	total := entity.UserTrafficTotal{
		UserID:        user.ID,
		UpBytes:       730_000,   // ~712 KiB
		DownBytes:     7_540_000, // ~7.19 MiB
		LastUpdatedAt: updated,
	}
	if err := repository.DB.Create(&total).Error; err != nil {
		t.Fatalf("create total: %v", err)
	}

	svc := NewSubscribeService()
	data, err := svc.GetSubscribePageData(user.Token)
	if err != nil {
		t.Fatalf("page data: %v", err)
	}

	if data.TrafficUpBytes != 730_000 || data.TrafficDownBytes != 7_540_000 {
		t.Errorf("unexpected raw bytes: up=%d down=%d", data.TrafficUpBytes, data.TrafficDownBytes)
	}
	if data.TrafficTotalBytes != 730_000+7_540_000 {
		t.Errorf("total mismatch: %d", data.TrafficTotalBytes)
	}
	if data.TrafficUpHuman == "" || data.TrafficDownHuman == "" || data.TrafficTotalHuman == "" {
		t.Errorf("human strings missing: %+v", data)
	}
	if data.TrafficLastUpdatedAt == nil || data.TrafficLastUpdatedAt.IsZero() {
		t.Errorf("last updated missing")
	}
}

// TestSubscribePageDataNoTraffic 验证无流量记录用户字段为零值
// 模板侧据此显示"尚无流量记录"
func TestSubscribePageDataNoTraffic(t *testing.T) {
	setupServiceTestDB(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "newbie",
		UUID:      "00000000-0000-0000-0000-000000000002",
		Token:     "traffic-token-2",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	svc := NewSubscribeService()
	data, err := svc.GetSubscribePageData(user.Token)
	if err != nil {
		t.Fatalf("page data: %v", err)
	}

	if data.TrafficUpBytes != 0 || data.TrafficDownBytes != 0 || data.TrafficTotalBytes != 0 {
		t.Errorf("expected zero bytes, got up=%d down=%d total=%d",
			data.TrafficUpBytes, data.TrafficDownBytes, data.TrafficTotalBytes)
	}
	if data.TrafficLastUpdatedAt != nil {
		t.Errorf("expected nil last updated, got %v", data.TrafficLastUpdatedAt)
	}
}

// TestFormatBytes 验证字节格式化的关键 case
// 与 frontend/src/lib/utils.ts 行为对齐
func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{-1, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KiB"},
		{1536, "1.50 KiB"},
		{10_240, "10.0 KiB"},
		{730_000, "712.9 KiB"},
		{7_540_000, "7.19 MiB"},
		{1_073_741_824, "1.00 GiB"},
	}
	for _, c := range cases {
		got := formatBytes(c.in)
		if got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
