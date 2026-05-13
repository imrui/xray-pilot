package xray

import (
	"testing"

	"github.com/imrui/xray-pilot/internal/xray/statspb"
)

// TestParseUserStats 验证 xray QueryStats 扁平响应按 email 聚合的正确性
func TestParseUserStats(t *testing.T) {
	in := []*statspb.Stat{
		{Name: "user>>>alice>>>traffic>>>uplink", Value: 100},
		{Name: "user>>>alice>>>traffic>>>downlink", Value: 200},
		{Name: "user>>>bob>>>traffic>>>uplink", Value: 50},
		// 噪音条目应被忽略
		{Name: "inbound>>>vless-reality>>>traffic>>>uplink", Value: 9999},
		{Name: "user>>>>>>traffic>>>uplink", Value: 1}, // 空 email
		{Name: "malformed", Value: 1},
		nil, // nil 入口
	}

	out := parseUserStats(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 users, got %d: %+v", len(out), out)
	}

	got := map[string]UserDelta{}
	for _, d := range out {
		got[d.Email] = d
	}

	if a, ok := got["alice"]; !ok {
		t.Errorf("missing alice")
	} else if a.UpBytes != 100 || a.DownBytes != 200 {
		t.Errorf("alice up=%d down=%d, want 100/200", a.UpBytes, a.DownBytes)
	}

	if b, ok := got["bob"]; !ok {
		t.Errorf("missing bob")
	} else if b.UpBytes != 50 || b.DownBytes != 0 {
		t.Errorf("bob up=%d down=%d, want 50/0", b.UpBytes, b.DownBytes)
	}
}

// TestParseUserStatsEmpty 验证空响应安全返回
func TestParseUserStatsEmpty(t *testing.T) {
	if out := parseUserStats(nil); len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
	if out := parseUserStats([]*statspb.Stat{}); len(out) != 0 {
		t.Errorf("expected empty result, got %v", out)
	}
}
