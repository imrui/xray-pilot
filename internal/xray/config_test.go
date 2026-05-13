package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
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
