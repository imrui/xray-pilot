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
	// 1. SchemaVersion < 3 禁止携带 v3 专属字段
	for _, legacyBad := range []string{
		`{"schema_version":2,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","dns":{"enabled":true,"servers":["1.1.1.1"]},"custom_rules":[]}`,
		`{"schema_version":2,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","balancers":[{"tag":"b1","selector":["direct"]}],"custom_rules":[]}`,
		`{"schema_version":2,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[{"type":"field","port":"443","balancerTag":"b1"}]}`,
	} {
		if _, err := DecodeRoutingPolicy(legacyBad); err == nil {
			t.Fatalf("expected schema_version < 3 rejection for %s", legacyBad)
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

func TestPhase3DNSBalancersObservatoryAndCrossModuleMerge(t *testing.T) {
	fullPolicyRaw := `{
		"schema_version": 3,
		"domain_strategy": "IPIfNonMatch",
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{"tag": "hk-landing", "protocol": "socks", "settings": {"servers": [{"address": "203.0.113.10", "port": 1080}]}},
			{"tag": "sg-landing", "protocol": "socks", "settings": {"servers": [{"address": "203.0.113.20", "port": 1080}]}}
		],
		"balancers": [
			{
				"tag": "unlock-pool",
				"remark": "多落地最低延迟池",
				"selector": ["hk-landing", "sg-landing"],
				"strategy": {"type": "leastPing"},
				"fallbackTag": "direct-v4"
			}
		],
		"dns": {
			"enabled": true,
			"queryStrategy": "UseIPv4",
			"disableFallbackIfMatch": true,
			"hosts": {
				"dns.google": ["8.8.8.8", "8.8.4.4"],
				"domain:internal.example.com": "198.51.100.5"
			},
			"servers": [
				{
					"address": "1.1.1.1",
					"port": 53,
					"domains": ["geosite:openai", "geosite:netflix"],
					"expectIPs": ["!geoip:cn"],
					"skipFallback": true,
					"queryStrategy": "UseIPv4"
				},
				"https+local://1.1.1.1/dns-query",
				"localhost"
			]
		},
		"custom_rules": [
			{
				"type": "field",
				"remark": "AI 走负载均衡池",
				"domain": ["geosite:openai"],
				"balancerTag": "unlock-pool"
			}
		]
	}`

	p, err := DecodeRoutingPolicy(fullPolicyRaw)
	if err != nil {
		t.Fatalf("DecodeRoutingPolicy Phase 3 failed: %v", err)
	}

	baseCfg := `{
		"log": {"loglevel": "warning"},
		"routing": {"domainStrategy": "IPIfNonMatch", "rules": [{"inboundTag": ["api-inbound"], "outboundTag": "api"}]},
		"inbounds": [
			{"listen": "0.0.0.0", "port": 24443, "protocol": "vless", "tag": "vless-reality-1", "settings": {"clients": [], "decryption": "none"}},
			{"listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "tag": "api-inbound", "settings": {"address": "127.0.0.1"}}
		],
		"outbounds": [{"protocol": "freedom", "tag": "direct"}]
	}`

	rendered, err := ApplyRoutingPolicyWithOptions(baseCfg, p, RenderOptions{
		DefaultProbeURL: "https://cp.cloudflare.com/generate_204",
	})
	if err != nil {
		t.Fatalf("ApplyRoutingPolicyWithOptions failed: %v", err)
	}

	// 1. 验证启用 DNS 时内置 direct 自动切换为 UseIPv4（对齐 dns.queryStrategy），且剥离面板 enabled/remark 字段
	if strings.Contains(rendered, `"enabled": true`) && strings.Contains(rendered, `"dns"`) {
		var rawMap map[string]any
		_ = json.Unmarshal([]byte(rendered), &rawMap)
		if dnsMap, ok := rawMap["dns"].(map[string]any); ok && dnsMap["enabled"] != nil {
			t.Fatalf("panel dns.enabled leaked into Xray config:\n%s", rendered)
		}
	}
	if strings.Contains(rendered, "多落地最低延迟池") {
		t.Fatalf("balancer remark leaked into Xray config:\n%s", rendered)
	}
	// 2. 验证 leastPing 自动补齐 observatory，使用系统探针 URL，并拉入候选出站及 fallbackTag (direct-v4)
	for _, want := range []string{
		`"observatory"`,
		`"https://cp.cloudflare.com/generate_204"`,
		`"probeInterval": "1m"`,
		`"balancerTag": "unlock-pool"`,
		`"hk-landing"`,
		`"sg-landing"`,
		`"direct-v4"`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered Phase 3 config missing %s:\n%s", want, rendered)
		}
	}
	var parsedCfg Config
	if err := json.Unmarshal([]byte(rendered), &parsedCfg); err != nil {
		t.Fatal(err)
	}
	directSettings, _ := json.Marshal(parsedCfg.Outbounds[0].Settings)
	if !strings.Contains(string(directSettings), `"UseIPv4"`) {
		t.Fatalf("expected built-in direct outbound domainStrategy=UseIPv4 when DNS enabled, got %s", directSettings)
	}

	// 3. 节点级 dns_override 关闭 DNS 时：回退系统解析（不输出 dns 段，direct 恢复无 domainStrategy）
	mergedDisableDNS, err := MergeRoutingPolicy(fullPolicyRaw, `{"dns_override":{"enabled":false}}`)
	if err != nil {
		t.Fatalf("MergeRoutingPolicy disable dns failed: %v", err)
	}
	renderedNoDNS, err := ApplyRoutingPolicy(baseCfg, mergedDisableDNS)
	if err != nil {
		t.Fatal(err)
	}
	var noDNSCfg Config
	_ = json.Unmarshal([]byte(renderedNoDNS), &noDNSCfg)
	if noDNSCfg.DNS != nil || noDNSCfg.Outbounds[0].Settings != nil {
		t.Fatalf("expected DNS disabled and direct settings nil, got dns=%+v settings=%+v", noDNSCfg.DNS, noDNSCfg.Outbounds[0].Settings)
	}

	// 4. 节点级 outbound_overrides 与负载均衡联动：remap 掉一个出站保留另一个成功；全部 remap 掉导致候选集为空则报错拦截
	partialRemap, err := MergeRoutingPolicy(fullPolicyRaw, `{"outbound_overrides":{"sg-landing":{"mode":"remap","target_tag":"direct"}}}`)
	if err != nil || len(partialRemap.Balancers) != 1 || len(partialRemap.Balancers[0].Selector) != 1 || partialRemap.Balancers[0].Selector[0] != "hk-landing" {
		t.Fatalf("expected partial remap to shrink balancer selector to [hk-landing], got %+v err=%v", partialRemap.Balancers, err)
	}
	if _, err := MergeRoutingPolicy(fullPolicyRaw, `{
		"outbound_overrides":{
			"hk-landing":{"mode":"remap","target_tag":"direct"},
			"sg-landing":{"mode":"remap","target_tag":"direct"}
		}
	}`); err == nil {
		t.Fatal("expected error when all candidates of an active balancer are remapped away")
	}

	// 5. 安全护栏：SSRF probeURL 拦截、本机回环 DNS 端口与托管入站/10085 自环冲突拦截、balancer 与出站重名拦截、规则目标互斥拦截
	invalidPhase3 := []string{
		// observatory SSRF 内网地址拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"s1","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}],"balancers":[{"tag":"b1","selector":["s1"],"strategy":{"type":"leastPing"}}],"observatory":{"subjectSelector":["s1"],"probeURL":"https://127.0.0.1/health","probeInterval":"30s"},"custom_rules":[]}`,
		// observatory probeInterval 过短拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"s1","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}}],"observatory":{"subjectSelector":["s1"],"probeInterval":"2s"},"custom_rules":[]}`,
		// observatory 未覆盖 leastPing 候选出站拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"s1","protocol":"socks","settings":{"servers":[{"address":"1.1.1.1","port":1080}]}},{"tag":"s2","protocol":"socks","settings":{"servers":[{"address":"1.1.1.2","port":1080}]}}],"balancers":[{"tag":"b1","selector":["s1","s2"],"strategy":{"type":"leastPing"}}],"observatory":{"subjectSelector":["s1"]},"custom_rules":[]}`,
		// balancer 与 custom_outbound 同名拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"dup","protocol":"freedom"}],"balancers":[{"tag":"dup","selector":["direct"]}],"custom_rules":[]}`,
		// balancer selector 匹配 block 拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"balancers":[{"tag":"b1","selector":["blo"]}],"custom_rules":[]}`,
		// 规则同时填写 outboundTag 和 balancerTag 拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"balancers":[{"tag":"b1","selector":["direct"]}],"custom_rules":[{"type":"field","port":"443","outboundTag":"direct","balancerTag":"b1"}]}`,
		// DNS 指向 127.0.0.1:10085 拦截
		`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"dns":{"enabled":true,"servers":[{"address":"127.0.0.1","port":10085}]},"custom_rules":[]}`,
	}
	for _, raw := range invalidPhase3 {
		if _, err := DecodeRoutingPolicy(raw); err == nil {
			t.Fatalf("expected invalid Phase 3 policy to be rejected: %s", raw)
		}
	}

	// DNS 指向当前节点托管入站端口（127.0.0.1:24443）在渲染期拦截自环
	loopbackInboundDNS, err := DecodeRoutingPolicy(`{
		"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict",
		"custom_outbounds":[],
		"dns":{"enabled":true,"servers":[{"address":"127.0.0.1","port":24443}]},
		"custom_rules":[]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRoutingPolicy(baseCfg, loopbackInboundDNS); err == nil {
		t.Fatal("expected DNS loopback port conflict with node inbound port 24443 to be rejected")
	}
}
