package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/config"
)

func TestSchemaV3LosslessExtraAndMetadataStripping(t *testing.T) {
	rawPolicy := `{
		"schema_version": 3,
		"domain_strategy": "IPOnDemand",
		"ipv4_force_domains": ["domain:openai.com"],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{
				"tag": "hk-landing",
				"protocol": "socks",
				"remark": "香港落地出口",
				"sendThrough": "203.0.113.5",
				"settings": {
					"servers": [{"address": "203.0.113.10", "port": 1080}],
					"uot": true
				},
				"streamSettings": {
					"network": "tcp",
					"sockopt": {"dialerProxy": "direct-v4", "tcpKeepAliveInterval": 30}
				},
				"mux": {"enabled": true, "concurrency": 8},
				"proxySettings": {"tag": "legacy-compat"}
			},
			{
				"tag": "unused-outbound",
				"protocol": "freedom",
				"remark": "未被引用出站应自动剔除"
			}
		],
		"custom_rules": [
			{
				"type": "field",
				"remark": "AI 分流",
				"disabled": false,
				"domain": ["geosite:openai", "geosite:google@cn", "keyword:claude"],
				"protocol": ["tls", "quic"],
				"outboundTag": "hk-landing",
				"ruleTag": "ai-rule",
				"domainMatcher": "hybrid"
			},
			{
				"type": "field",
				"remark": "暂停的规则不应输出",
				"disabled": true,
				"ip": ["!geoip:cn", "geoip:telegram"],
				"outboundTag": "unused-outbound"
			}
		]
	}`

	p, err := DecodeRoutingPolicy(rawPolicy)
	if err != nil {
		t.Fatalf("DecodeRoutingPolicy failed: %v", err)
	}
	if p.SchemaVersion != 3 || p.DomainStrategy != "IPOnDemand" {
		t.Fatalf("unexpected decoded header: %+v", p)
	}

	// 二次序列化策略 JSON 应保留 remark、disabled 与未知扩展字段
	roundTripBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal policy failed: %v", err)
	}
	roundTrip := string(roundTripBytes)
	for _, want := range []string{
		`"香港落地出口"`,
		`"proxySettings"`,
		`"tcpKeepAliveInterval"`,
		`"uot":true`,
		`"ruleTag":"ai-rule"`,
		`"domainMatcher":"hybrid"`,
		`"disabled":true`,
	} {
		if !strings.Contains(roundTrip, want) {
			t.Fatalf("policy round-trip lost %s:\n%s", want, roundTrip)
		}
	}

	baseCfg := `{"log":{"loglevel":"warning"},"routing":{"domainStrategy":"IPIfNonMatch","rules":[{"inboundTag":["api-inbound"],"outboundTag":"api"},{"type":"field","ip":["geoip:private"],"outboundTag":"block"}]},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"}]}`
	rendered, err := ApplyRoutingPolicy(baseCfg, p)
	if err != nil {
		t.Fatalf("ApplyRoutingPolicy failed: %v", err)
	}

	// 渲染至节点最终配置时：剥离 remark/disabled、跳过禁用规则、剔除未引用出站、保留高级扩展字段与系统置顶规则
	for _, forbidden := range []string{"remark", "香港落地出口", "AI 分流", "暂停的规则", "disabled", "unused-outbound"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("rendered config leaked metadata or unused outbound %q:\n%s", forbidden, rendered)
		}
	}
	for _, expected := range []string{
		`"domainStrategy": "IPOnDemand"`,
		`"sendThrough": "203.0.113.5"`,
		`"proxySettings"`,
		`"tcpKeepAliveInterval": 30`,
		`"concurrency": 8`,
		`"ruleTag": "ai-rule"`,
		`"domainMatcher": "hybrid"`,
		`"direct-v4"`,
		`"hk-landing"`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered config missing %s:\n%s", expected, rendered)
		}
	}

	var cfg Config
	if err := json.Unmarshal([]byte(rendered), &cfg); err != nil {
		t.Fatalf("unmarshal rendered config: %v", err)
	}
	if len(cfg.Outbounds) != 4 || cfg.Outbounds[0].Tag != "direct" || cfg.Outbounds[1].Tag != "block" || cfg.Outbounds[2].Tag != "direct-v4" || cfg.Outbounds[3].Tag != "hk-landing" {
		t.Fatalf("unexpected outbound order: %+v", cfg.Outbounds)
	}
	if len(cfg.Routing.Rules) != 4 || cfg.Routing.Rules[0].OutboundTag != "api" || cfg.Routing.Rules[2].OutboundTag != "hk-landing" || cfg.Routing.Rules[3].OutboundTag != "direct-v4" {
		t.Fatalf("unexpected rule waterfall order: %+v", cfg.Routing.Rules)
	}
}

func TestSchemaV3GuardrailsAndProtocols(t *testing.T) {
	// 1. Phase 3 预留字段拦截
	for _, reservedCase := range []string{
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"dns":{"enabled":true},"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"balancers":[{"tag":"b1","selector":["direct"]}],"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"observatory":{"subjectSelector":["direct"]},"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[{"type":"field","port":"443","balancerTag":"b1"}]}`,
	} {
		_, err := DecodeRoutingPolicy(reservedCase)
		if err == nil || !strings.Contains(err.Error(), "DNS 与负载均衡将在后续阶段启用") {
			t.Fatalf("expected Phase 3 guard error, got %v for %s", err, reservedCase)
		}
	}

	// 2. 保留 Tag、托管入站前缀与非法协议拦截
	for _, badTag := range []string{
		"direct", "block", "direct-v4", "direct-v6", "api", "api-inbound", "dns-out",
		"vless-reality-1", "vless-ws-2", "trojan-9", "http-proxy-3", "socks-proxy-4",
		"-bad-start", "bad space",
	} {
		raw := `{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"` + badTag + `","protocol":"freedom"}],"custom_rules":[]}`
		if _, err := DecodeRoutingPolicy(raw); err == nil {
			t.Fatalf("accepted forbidden or malformed tag %q", badTag)
		}
	}

	// 3. 5 类结构化协议合法配置通过，缺失必填项拦截
	validAllFive := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "out-socks", "protocol": "socks", "settings": {"servers": [{"address": "198.51.100.1", "port": 1080}]}},
			{"tag": "out-http", "protocol": "http", "settings": {"servers": [{"address": "198.51.100.2", "port": 8080}]}},
			{"tag": "out-freedom", "protocol": "freedom", "sendThrough": "198.51.100.3", "settings": {"domainStrategy": "UseIPv4"}},
			{"tag": "out-wg", "protocol": "wireguard", "settings": {"secretKey": "wg-priv", "peers": [{"endpoint": "engage.cloudflareclient.com:2408", "publicKey": "wg-pub"}]}},
			{"tag": "out-vless", "protocol": "vless", "settings": {"vnext": [{"address": "example.com", "port": 443, "users": [{"id": "11111111-1111-4111-8111-111111111111"}]}]}}
		],
		"custom_rules": [
			{"type": "field", "domain": ["geosite:openai"], "outboundTag": "out-wg"},
			{"type": "field", "disabled": true, "port": "443", "outboundTag": "out-vless"}
		]
	}`
	if _, err := DecodeRoutingPolicy(validAllFive); err != nil {
		t.Fatalf("valid 5-protocol policy rejected: %v", err)
	}

	invalidCases := []string{
		// dns 协议禁止作为自定义出站
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"my-dns","protocol":"dns"}],"custom_rules":[]}`,
		// socks 端口越界
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"s1","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":70000}]}}],"custom_rules":[]}`,
		// wireguard 缺少端口或 secretKey
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"wg1","protocol":"wireguard","settings":{"secretKey":"","peers":[{"endpoint":"1.1.1.1:2408","publicKey":"pk"}]}}],"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"wg1","protocol":"wireguard","settings":{"secretKey":"sk","peers":[{"endpoint":"1.1.1.1","publicKey":"pk"}]}}],"custom_rules":[]}`,
		// freedom sendThrough 非 IP
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"f1","protocol":"freedom","sendThrough":"not-ip"}],"custom_rules":[]}`,
		// vless 缺少用户 id
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"v1","protocol":"vless","settings":{"vnext":[{"address":"a.com","port":443,"users":[{"id":""}]}]}}],"custom_rules":[]}`,
		// dialerProxy 自环与互环
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"a","protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"a"}}}],"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"a","protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"b"}}},{"tag":"b","protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"a"}}}],"custom_rules":[]}`,
		// dialerProxy 指向 block / 不存在出站
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"a","protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"block"}}}],"custom_rules":[]}`,
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"a","protocol":"freedom","streamSettings":{"sockopt":{"dialerProxy":"ghost"}}}],"custom_rules":[]}`,
		// 禁用规则引用不存在出站仍需拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[{"type":"field","disabled":true,"port":"443","outboundTag":"ghost"}]}`,
		// sourceIP 别名绕过拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[{"type":"field","sourceIP":["1.1.1.1"],"port":"443","outboundTag":"direct"}]}`,
		// geoip:private 重复声明拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[{"type":"field","ip":["geoip:private"],"outboundTag":"direct"}]}`,
	}
	for _, raw := range invalidCases {
		if _, err := DecodeRoutingPolicy(raw); err == nil {
			t.Fatalf("invalid case unexpectedly accepted: %s", raw)
		}
	}
}

func TestRefinedRequiresIPv6(t *testing.T) {
	// 纯 IPv4 机器通过外部 WARP 域名/IPv4 隧道解锁流媒体：RequiresIPv6 应为 false
	warpOverIPv4 := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "warp", "protocol": "wireguard", "settings": {"secretKey": "sk", "address": ["172.16.0.2/32", "2606:4700:110::2/128"], "peers": [{"endpoint": "162.159.192.1:2408", "publicKey": "pk"}]}}
		],
		"custom_rules": [
			{"type": "field", "domain": ["geosite:netflix"], "outboundTag": "warp"},
			{"type": "field", "disabled": true, "domain": ["domain:disabled-v6.com"], "outboundTag": "direct-v6"}
		]
	}`
	p, err := DecodeRoutingPolicy(warpOverIPv4)
	if err != nil {
		t.Fatal(err)
	}
	if p.RequiresIPv6() {
		t.Fatal("WARP over IPv4 endpoint or disabled direct-v6 rule should not require host IPv6 route")
	}

	// 自定义出站远端地址为 IPv6 字面量且被启用规则引用：RequiresIPv6 应为 true
	v6Socks := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "v6-landing", "protocol": "socks", "settings": {"servers": [{"address": "2001:db8::10", "port": 1080}]}}
		],
		"custom_rules": [
			{"type": "field", "domain": ["geosite:openai"], "outboundTag": "v6-landing"}
		]
	}`
	pV6, err := DecodeRoutingPolicy(v6Socks)
	if err != nil || !pV6.RequiresIPv6() {
		t.Fatalf("expected RequiresIPv6=true for IPv6 remote server, got err=%v", err)
	}

	// 自定义 freedom ForceIPv6 被启用规则引用：RequiresIPv6 应为 true
	forceV6 := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "my-v6", "protocol": "freedom", "settings": {"domainStrategy": "ForceIPv6"}}
		],
		"custom_rules": [
			{"type": "field", "port": "443", "outboundTag": "my-v6"}
		]
	}`
	pForceV6, err := DecodeRoutingPolicy(forceV6)
	if err != nil || !pForceV6.RequiresIPv6() {
		t.Fatalf("expected RequiresIPv6=true for ForceIPv6 custom freedom, got err=%v", err)
	}
}

func TestMergeRoutingPolicyOutboundOverrides(t *testing.T) {
	base := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "hk-landing", "protocol": "socks", "settings": {"servers": [{"address": "203.0.113.10", "port": 1080}]}, "streamSettings": {"sockopt": {"dialerProxy": "warp-egress"}}},
			{"tag": "warp-egress", "protocol": "wireguard", "settings": {"secretKey": "sk", "peers": [{"endpoint": "162.159.192.1:2408", "publicKey": "pk"}]}}
		],
		"custom_rules": [
			{"type": "field", "domain": ["geosite:openai"], "outboundTag": "hk-landing"},
			{"type": "field", "domain": ["geosite:netflix"], "outboundTag": "warp-egress"}
		]
	}`

	// replace (跨协议替换为 freedom + sendThrough) + remap (warp-egress -> direct-v4)
	overrides := `{
		"outbound_overrides": {
			"hk-landing": {
				"mode": "replace",
				"outbound": {
					"tag": "hk-landing",
					"protocol": "freedom",
					"sendThrough": "198.51.100.99",
					"streamSettings": {"sockopt": {"dialerProxy": "warp-egress"}}
				}
			},
			"warp-egress": {
				"mode": "remap",
				"target_tag": "direct-v4"
			}
		}
	}`
	merged, err := MergeRoutingPolicy(base, overrides)
	if err != nil {
		t.Fatalf("MergeRoutingPolicy failed: %v", err)
	}
	if len(merged.CustomOutbounds) != 1 || merged.CustomOutbounds[0].Tag != "hk-landing" || merged.CustomOutbounds[0].Protocol != "freedom" || merged.CustomOutbounds[0].SendThrough != "198.51.100.99" {
		t.Fatalf("unexpected merged outbounds: %+v", merged.CustomOutbounds)
	}
	if merged.CustomRules[1].OutboundTag != "direct-v4" {
		t.Fatalf("expected rule outboundTag remapped to direct-v4, got %s", merged.CustomRules[1].OutboundTag)
	}
	dp, err := extractDialerProxy(merged.CustomOutbounds[0].StreamSettings)
	if err != nil || dp != "direct-v4" {
		t.Fatalf("expected dialerProxy remapped to direct-v4, got %q (%v)", dp, err)
	}

	// 异常覆盖场景：残留不存在的 tag、outbound.tag 不匹配、多跳链式 remap
	for _, badOv := range []string{
		`{"outbound_overrides":{"stale-tag":{"mode":"remap","target_tag":"direct"}}}`,
		`{"outbound_overrides":{"hk-landing":{"mode":"replace","outbound":{"tag":"wrong-tag","protocol":"freedom"}}}}`,
		`{"outbound_overrides":{"hk-landing":{"mode":"remap","target_tag":"warp-egress"},"warp-egress":{"mode":"remap","target_tag":"direct"}}}`,
		`{"dns_override":{"enabled":true}}`,
	} {
		if _, err := MergeRoutingPolicy(base, badOv); err == nil {
			t.Fatalf("expected error for invalid override: %s", badOv)
		}
	}
}

func TestOutboundSecretEncryptionIdempotencyAndMasking(t *testing.T) {
	oldKey := config.Global.Crypto.MasterKey
	config.Global.Crypto.MasterKey = strings.Repeat("22", 32)
	t.Cleanup(func() { config.Global.Crypto.MasterKey = oldKey })

	raw := `{
		"schema_version": 3,
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "s1", "protocol": "socks", "settings": {"servers": [{"address": "1.1.1.1", "port": 1080, "users": [{"user": "u1", "pass": "socks-secret"}]}]}},
			{"tag": "v1", "protocol": "vless", "settings": {"vnext": [{"address": "2.2.2.2", "port": 443, "users": [{"id": "11111111-1111-4111-8111-111111111111"}]}]}, "streamSettings": {"realitySettings": {"privateKey": "reality-priv"}}},
			{"tag": "t1", "protocol": "trojan", "settings": {"servers": [{"address": "3.3.3.3", "port": 443, "password": "trojan-password"}]}},
			{"tag": "w1", "protocol": "wireguard", "settings": {"secretKey": "wg-secret", "peers": [{"endpoint": "4.4.4.4:2408", "publicKey": "wg-pub", "preSharedKey": "wg-psk"}]}}
		],
		"custom_rules": [
			{"type": "field", "port": "80", "outboundTag": "s1"},
			{"type": "field", "port": "443", "outboundTag": "v1"},
			{"type": "field", "port": "8443", "outboundTag": "t1"},
			{"type": "field", "port": "2408", "outboundTag": "w1"}
		]
	}`

	plainPolicy, err := DecodeRoutingPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	enc1, err := EncryptPolicySecrets(plainPolicy, nil)
	if err != nil {
		t.Fatal(err)
	}
	enc1JSON, _ := json.Marshal(enc1)
	for _, secret := range []string{"socks-secret", "11111111-1111-4111-8111-111111111111", "reality-priv", "trojan-password", "wg-secret", "wg-psk"} {
		if strings.Contains(string(enc1JSON), secret) {
			t.Fatalf("plaintext secret %q leaked into encrypted JSON: %s", secret, enc1JSON)
		}
	}
	if strings.Count(string(enc1JSON), OutboundSecretPrefix) != 6 {
		t.Fatalf("expected 6 enc:v1: fields, got:\n%s", enc1JSON)
	}

	// 重复提交相同明文（传入 prev = &enc1）应 100% 复用原密文，避免随机 Nonce 变化
	enc2, err := EncryptPolicySecrets(plainPolicy, &enc1)
	if err != nil {
		t.Fatal(err)
	}
	enc2JSON, _ := json.Marshal(enc2)
	if string(enc1JSON) != string(enc2JSON) {
		t.Fatalf("re-saving unchanged plaintext produced different ciphertext!\nenc1=%s\nenc2=%s", enc1JSON, enc2JSON)
	}

	// 回传原密文同样保持字节不变
	enc3, err := EncryptPolicySecrets(enc1, &enc1)
	if err != nil {
		t.Fatal(err)
	}
	enc3JSON, _ := json.Marshal(enc3)
	if string(enc1JSON) != string(enc3JSON) {
		t.Fatal("re-submitting existing ciphertext mutated policy")
	}

	// ApplyRoutingPolicy 必须解密所有 enc:v1: 字段后写入最终配置
	baseCfg := `{"log":{},"routing":{"rules":[{"inboundTag":["api-inbound"],"outboundTag":"api"}]},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"}]}`
	rendered, err := ApplyRoutingPolicy(baseCfg, enc1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered, OutboundSecretPrefix) {
		t.Fatalf("rendered config still contains encrypted token:\n%s", rendered)
	}
	for _, secret := range []string{"socks-secret", "11111111-1111-4111-8111-111111111111", "reality-priv", "trojan-password", "wg-secret", "wg-psk"} {
		if !strings.Contains(rendered, secret) {
			t.Fatalf("rendered config missing decrypted secret %q", secret)
		}
	}

	// 普通 admin 脱敏后不应包含任何 enc:v1: 或明文凭据，且提交 <masked> 应被拦截
	maskedJSON := MaskPolicyJSON(string(enc1JSON))
	if strings.Contains(maskedJSON, OutboundSecretPrefix) || !strings.Contains(maskedJSON, MaskedPlaceholder) {
		t.Fatalf("MaskPolicyJSON failed:\n%s", maskedJSON)
	}
	if _, err := DecodeRoutingPolicy(maskedJSON); err == nil {
		t.Fatal("DecodeRoutingPolicy must reject <masked> placeholder as secret")
	}
}
