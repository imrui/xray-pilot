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

// TestGenerateConfigProxyInbound 验证 http/socks 代理入站与白名单路由规则的生成。
// 白名单通过 routing 规则实现：白名单来源 → direct，其余同 inbound 流量 → block。
func TestGenerateConfigProxyInbound(t *testing.T) {
	node := &entity.Node{ID: 1, Name: "n1", IP: "1.2.3.4"}
	httpProfile := &entity.InboundProfile{
		ID: 11, Name: "HTTP 代理", Protocol: types.ProtocolHTTP, Port: 18080, Active: true,
		Settings: `{"allowed_ips":["203.0.113.10","198.51.100.0/24"],"accounts":[{"user":"app","pass":"secret"}]}`,
	}
	socksProfile := &entity.InboundProfile{
		ID: 12, Name: "SOCKS 代理", Protocol: types.ProtocolSocks, Port: 11080, Active: true,
		Settings: `{"allowed_ips":["203.0.113.10"]}`,
	}
	keys := []entity.NodeProfileKey{
		{NodeID: 1, ProfileID: 11, Profile: httpProfile, Settings: `{}`},
		{NodeID: 1, ProfileID: 12, Profile: socksProfile, Settings: `{}`},
	}

	configJSON, warnings, err := GenerateConfig(node, keys, nil, LogConfig{Level: "warning"})
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}

	var cfg struct {
		Inbounds []struct {
			Protocol string          `json:"protocol"`
			Port     int             `json:"port"`
			Tag      string          `json:"tag"`
			Settings json.RawMessage `json:"settings"`
		} `json:"inbounds"`
		Routing struct {
			Rules []struct {
				InboundTag  []string `json:"inboundTag"`
				Source      []string `json:"source"`
				OutboundTag string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// http 入站：端口 / tag / 认证账号
	var foundHTTP, foundSocks bool
	for _, in := range cfg.Inbounds {
		switch in.Tag {
		case "http-proxy-11":
			foundHTTP = true
			if in.Protocol != "http" || in.Port != 18080 {
				t.Errorf("http inbound 参数错误: %+v", in)
			}
			var hs struct {
				Accounts []struct{ User, Pass string } `json:"accounts"`
			}
			if err := json.Unmarshal(in.Settings, &hs); err != nil || len(hs.Accounts) != 1 || hs.Accounts[0].User != "app" {
				t.Errorf("http inbound 缺少认证账号: %s", in.Settings)
			}
		case "socks-proxy-12":
			foundSocks = true
			if in.Protocol != "socks" || in.Port != 11080 {
				t.Errorf("socks inbound 参数错误: %+v", in)
			}
			// 无 accounts 时 auth 必须为 noauth（仅白名单模式）
			var ss struct {
				Auth string `json:"auth"`
			}
			if err := json.Unmarshal(in.Settings, &ss); err != nil || ss.Auth != "noauth" {
				t.Errorf("socks inbound auth 应为 noauth: %s", in.Settings)
			}
		}
	}
	if !foundHTTP || !foundSocks {
		t.Fatalf("缺少代理入站 http=%v socks=%v\n%s", foundHTTP, foundSocks, configJSON)
	}

	// 路由规则：每个代理入站一条白名单 direct + 一条兜底 block，顺序 direct 在前
	assertProxyRules := func(tag string, wantSource []string) {
		t.Helper()
		directIdx, blockIdx := -1, -1
		for i, r := range cfg.Routing.Rules {
			if len(r.InboundTag) == 1 && r.InboundTag[0] == tag {
				if r.OutboundTag == "direct" && len(r.Source) == len(wantSource) {
					directIdx = i
				}
				if r.OutboundTag == "block" && len(r.Source) == 0 {
					blockIdx = i
				}
			}
		}
		if directIdx == -1 || blockIdx == -1 || directIdx > blockIdx {
			t.Errorf("入站 %s 白名单路由规则缺失或顺序错误 (direct=%d block=%d)\n%s", tag, directIdx, blockIdx, configJSON)
		}
	}
	assertProxyRules("http-proxy-11", []string{"203.0.113.10", "198.51.100.0/24"})
	assertProxyRules("socks-proxy-12", []string{"203.0.113.10"})
}

// TestGenerateConfigProxyGuardrails 安全护栏：白名单与认证均未配置、或白名单条目非法时，
// 拒绝生成该入站（转为 warning），防止公网开放代理或让 xray 整体起不来。
func TestGenerateConfigProxyGuardrails(t *testing.T) {
	node := &entity.Node{ID: 1, Name: "n1", IP: "1.2.3.4"}
	openProfile := &entity.InboundProfile{
		ID: 21, Name: "裸奔代理", Protocol: types.ProtocolHTTP, Port: 18081, Active: true,
		Settings: `{}`,
	}
	badIPProfile := &entity.InboundProfile{
		ID: 22, Name: "坏白名单", Protocol: types.ProtocolSocks, Port: 11081, Active: true,
		Settings: `{"allowed_ips":["not-an-ip"]}`,
	}
	keys := []entity.NodeProfileKey{
		{NodeID: 1, ProfileID: 21, Profile: openProfile, Settings: `{}`},
		{NodeID: 1, ProfileID: 22, Profile: badIPProfile, Settings: `{}`},
	}

	configJSON, warnings, err := GenerateConfig(node, keys, nil, LogConfig{Level: "warning"})
	if err != nil {
		t.Fatalf("generate config: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %v", warnings)
	}
	if strings.Contains(configJSON, "http-proxy-21") || strings.Contains(configJSON, "socks-proxy-22") {
		t.Fatalf("护栏未生效，非法代理入站被生成\n%s", configJSON)
	}
}

// TestProxyInboundNodeKeyOverride 节点密钥可覆盖协议模板的白名单与账号（Effective helper 语义）。
func TestProxyInboundNodeKeyOverride(t *testing.T) {
	profile := &entity.InboundProfile{
		ID: 31, Name: "HTTP 代理", Protocol: types.ProtocolHTTP, Port: 18080, Active: true,
		Settings: `{"allowed_ips":["203.0.113.10"]}`,
	}
	key := &entity.NodeProfileKey{
		NodeID: 1, ProfileID: 31, Profile: profile,
		Settings: `{"allowed_ips":["192.0.2.99"]}`,
	}

	before, after := proxyRoutingRules(profile, key)
	if len(before) != 0 || len(after) != 2 {
		t.Fatalf("expected 0 before + 2 after rules, got %d/%d", len(before), len(after))
	}
	if len(after[0].Source) != 1 || after[0].Source[0] != "192.0.2.99" {
		t.Fatalf("节点覆盖白名单未生效: %+v", after[0].Source)
	}
}

// TestProxyAllowPrivate 校验 allow_private 开关：
// 默认（false）时白名单 direct 规则排在 geoip:private 屏蔽之后（内网目标被拦截）；
// true 时 direct 规则前置于 geoip:private 之前（白名单来源可访问内网目标）。
func TestProxyAllowPrivate(t *testing.T) {
	node := &entity.Node{ID: 1, Name: "n1", IP: "1.2.3.4"}
	profile := &entity.InboundProfile{
		ID: 41, Name: "内网代理", Protocol: types.ProtocolSocks, Port: 11080, Active: true,
		Settings: `{"allowed_ips":["203.0.113.10"],"allow_private":true}`,
	}
	keys := []entity.NodeProfileKey{{NodeID: 1, ProfileID: 41, Profile: profile, Settings: `{}`}}

	configJSON, warnings, err := GenerateConfig(node, keys, nil, LogConfig{Level: "warning"})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("generate config: err=%v warnings=%v", err, warnings)
	}

	var cfg struct {
		Routing struct {
			Rules []struct {
				IP          []string `json:"ip"`
				InboundTag  []string `json:"inboundTag"`
				Source      []string `json:"source"`
				OutboundTag string   `json:"outboundTag"`
			} `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	directIdx, privateIdx, blockIdx := -1, -1, -1
	for i, r := range cfg.Routing.Rules {
		if len(r.IP) == 1 && r.IP[0] == "geoip:private" {
			privateIdx = i
		}
		if len(r.InboundTag) == 1 && r.InboundTag[0] == "socks-proxy-41" {
			if r.OutboundTag == "direct" {
				directIdx = i
			} else if r.OutboundTag == "block" {
				blockIdx = i
			}
		}
	}
	if directIdx == -1 || privateIdx == -1 || blockIdx == -1 {
		t.Fatalf("规则缺失 direct=%d private=%d block=%d\n%s", directIdx, privateIdx, blockIdx, configJSON)
	}
	if directIdx > privateIdx {
		t.Errorf("allow_private=true 时 direct 规则应排在 geoip:private 之前 (direct=%d private=%d)", directIdx, privateIdx)
	}
	if blockIdx < privateIdx {
		t.Errorf("兜底 block 规则应排在 geoip:private 之后 (block=%d private=%d)", blockIdx, privateIdx)
	}

	// 节点密钥显式 false 覆盖模板 true → direct 回到 geoip:private 之后
	allowFalse := `{"allow_private":false}`
	before, after := proxyRoutingRules(profile, &entity.NodeProfileKey{
		NodeID: 1, ProfileID: 41, Profile: profile, Settings: allowFalse,
	})
	if len(before) != 0 || len(after) != 2 {
		t.Errorf("节点级 allow_private=false 覆盖未生效: before=%d after=%d", len(before), len(after))
	}
}
