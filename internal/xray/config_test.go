package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
)

// TestGenerateConfigIncludesStatsAndPolicy 验证生成的 xray 配置包含
// stats 模块与 policy 段，是流量统计功能的前置必要条件
func TestGenerateConfigIncludesStatsAndPolicy(t *testing.T) {
	node := &entity.Node{ID: 1, Name: "n1", IP: "1.2.3.4"}
	logCfg := LogConfig{Access: "none", Error: "/var/log/xray/error.log", Level: "warning"}

	configJSON, _, err := GenerateConfig(node, nil, nil, logCfg)
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}

	// 1. 反序列化必须成功
	var raw map[string]any
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// 2. stats 段存在（可为空对象）
	if _, ok := raw["stats"]; !ok {
		t.Fatalf("config missing required field: stats\n%s", configJSON)
	}

	// 3. policy 段存在，levels.0 含两个用户维度开关
	policy, ok := raw["policy"].(map[string]any)
	if !ok {
		t.Fatalf("config missing required field: policy\n%s", configJSON)
	}
	levels, _ := policy["levels"].(map[string]any)
	level0, _ := levels["0"].(map[string]any)
	if level0["statsUserUplink"] != true || level0["statsUserDownlink"] != true {
		t.Errorf("policy.levels.0 must enable both statsUserUplink/Downlink, got %+v", level0)
	}

	// 4. api.services 仍含 StatsService（与现有功能向后兼容）
	if !strings.Contains(configJSON, "StatsService") {
		t.Errorf("config missing api.services=StatsService")
	}
}

// TestInboundTag 锁死 tag 命名——gRPC live-apply 按 tag 定位 inbound，命名漂移即失效。
func TestInboundTag(t *testing.T) {
	cases := []struct {
		protocol   string
		id         uint
		wantTag    string
		manageable bool
	}{
		{types.ProtocolVlessReality, 3, "vless-reality-3", true},
		{types.ProtocolVlessWSTLS, 5, "vless-ws-5", true},
		{types.ProtocolTrojan, 7, "trojan-7", true},
		{types.ProtocolHysteria2, 9, "", false},
		{"unknown", 1, "", false},
	}
	for _, c := range cases {
		tag, ok := InboundTag(c.protocol, c.id)
		if tag != c.wantTag || ok != c.manageable {
			t.Errorf("InboundTag(%s,%d) = (%q,%v), want (%q,%v)", c.protocol, c.id, tag, ok, c.wantTag, c.manageable)
		}
	}
}

// TestGeneratedConfigUsesInboundTag 验证 GenerateConfig 生成的 inbound tag 与 InboundTag() 一致，
// 锁住「配置生成」与「live-apply 定位」共用同一 tag 来源的不变式。
func TestGeneratedConfigUsesInboundTag(t *testing.T) {
	node := &entity.Node{ID: 1, Name: "n1", IP: "1.2.3.4"}
	profile := &entity.InboundProfile{
		ID:       42,
		Protocol: types.ProtocolVlessReality,
		Port:     443,
		Active:   true,
		Settings: `{"sni":"www.microsoft.com","private_key":"dummy-priv"}`,
	}
	key := entity.NodeProfileKey{ProfileID: profile.ID, Profile: profile, Settings: `{"public_key":"pk","short_ids":["ab"]}`}
	logCfg := LogConfig{Level: "warning"}

	configJSON, warnings, err := GenerateConfig(node, []entity.NodeProfileKey{key}, nil, logCfg)
	if err != nil {
		t.Fatalf("generate config: %v (warnings=%v)", err, warnings)
	}
	wantTag, _ := InboundTag(types.ProtocolVlessReality, profile.ID)
	if !strings.Contains(configJSON, `"tag": "`+wantTag+`"`) {
		t.Errorf("生成配置未含预期 inbound tag %q\n%s", wantTag, configJSON)
	}
}
