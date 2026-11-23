package xray

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
	"google.golang.org/protobuf/encoding/protowire"
)

func buildTestGeositeBytes(categories map[string][]geositeDomainRule) []byte {
	var listBytes []byte
	for code, rules := range categories {
		var entryBytes []byte
		entryBytes = protowire.AppendTag(entryBytes, 1, protowire.BytesType)
		entryBytes = protowire.AppendString(entryBytes, code)
		for _, r := range rules {
			var domBytes []byte
			domBytes = protowire.AppendTag(domBytes, 1, protowire.VarintType)
			domBytes = protowire.AppendVarint(domBytes, uint64(r.Type))
			domBytes = protowire.AppendTag(domBytes, 2, protowire.BytesType)
			domBytes = protowire.AppendString(domBytes, r.Value)
			for _, attr := range r.Attrs {
				var attrBytes []byte
				attrBytes = protowire.AppendTag(attrBytes, 1, protowire.BytesType)
				attrBytes = protowire.AppendString(attrBytes, attr)
				domBytes = protowire.AppendTag(domBytes, 3, protowire.BytesType)
				domBytes = protowire.AppendBytes(domBytes, attrBytes)
			}
			entryBytes = protowire.AppendTag(entryBytes, 2, protowire.BytesType)
			entryBytes = protowire.AppendBytes(entryBytes, domBytes)
		}
		listBytes = protowire.AppendTag(listBytes, 1, protowire.BytesType)
		listBytes = protowire.AppendBytes(listBytes, entryBytes)
	}
	return listBytes
}

const semanticsBaseConfig = `{"log":{"loglevel":"none"},"routing":{"rules":[{"type":"field","inboundTag":["api-inbound"],"outboundTag":"api"},{"type":"field","ip":["geoip:private"],"outboundTag":"block"}]},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"}]}`

func semanticsPolicy(t *testing.T, fields string) RoutingPolicy {
	t.Helper()
	p, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict",` + fields + `}`)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDNSEffectivePortAndProtectedEndpoints(t *testing.T) {
	for _, server := range []string{
		`"tcp+local://localhost.:10085"`,
		`"tcp+local://LOCALHOST:10085"`,
		`"tcp+local://resolver.example:10085"`,
		`"tcp+local://[::ffff:127.0.0.1]:10085"`,
		`"tcp+local://0.0.0.0:10085"`,
		`{"address":"tcp+local://localhost.","port":10085}`,
		`{"address":"tcp://1.1.1.1:53","port":5353}`,
	} {
		t.Run(server, func(t *testing.T) {
			_, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","dns":{"enabled":true,"servers":[` + server + `]}}`)
			if err == nil {
				t.Fatal("accepted protected or ambiguous DNS endpoint")
			}
		})
	}
	p := semanticsPolicy(t, `"dns":{"enabled":true,"servers":[{"address":"tcp+local://127.0.0.1","port":5353}]}`)
	rendered, err := ApplyRoutingPolicy(semanticsBaseConfig, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, `"address": "tcp+local://127.0.0.1:5353"`) || strings.Contains(rendered, `"port": 5353`) {
		t.Fatalf("TCP DNS effective port was not moved into URI: %s", rendered)
	}
	if err := checkDNSInboundPortConflict(p.DNS, []Inbound{{Tag: "socks-proxy-1", Port: 53}}); err != nil {
		t.Fatalf("normalized target is 5353, not the ignored URI default 53: %v", err)
	}
	if err := checkDNSInboundPortConflict(p.DNS, []Inbound{{Tag: "socks-proxy-1", Port: 5353}}); err == nil {
		t.Fatal("effective 5353 target bypassed managed inbound guard")
	}
	for _, fields := range []string{
		`"dns":{"enabled":true,"servers":["tcp+local://localhost.:5353"]}`,
		`"dns":{"enabled":true,"servers":["tcp+local://sub.localhost:5353"]}`,
		`"dns":{"enabled":true,"hosts":{"resolver.example":"127.0.0.1"},"servers":["tcp://resolver.example:5353"]}`,
	} {
		p := semanticsPolicy(t, fields)
		if err := checkDNSInboundPortConflict(p.DNS, []Inbound{{Tag: "socks-proxy-1", Port: 5353}}); err == nil {
			t.Fatalf("local target %s bypassed managed inbound guard", fields)
		}
	}
	// 真实节点普遍在 443 端口运行 VLESS-Reality / Trojan：
	// 1) 公网域名 DoH 不得被误伤拦截；
	// 2) 非 +local 模式的 DoH 若被 Xray dns.hosts 映射至本机/私网地址，必须拦截自环；
	// 3) +local 模式走系统 internet.DialSystem，不受 Xray dns.hosts 影响，正常放行（但显式 localhost / 本地 IP 仍拦截）。
	pPublicDoH := semanticsPolicy(t, `"dns":{"enabled":true,"servers":["https://dns.google/dns-query","https+local://dns.alidns.com/dns-query"]}`)
	if err := checkDNSInboundPortConflict(pPublicDoH.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}); err != nil {
		t.Fatalf("public domain DoH falsely blocked on port-443 node: %v", err)
	}
	pHijackedNonLocalDoH := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"domain:google":"127.0.0.1"},"servers":["https://dns.google/dns-query"]}`)
	if err := checkDNSInboundPortConflict(pHijackedNonLocalDoH.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}); err == nil {
		t.Fatal("hosts-hijacked non-local DoH bypassed port-443 inbound guard")
	}
	pLocalModeUnaffectedByXrayHosts := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"domain:google":"127.0.0.1"},"servers":["https+local://dns.google/dns-query"]}`)
	if err := checkDNSInboundPortConflict(pLocalModeUnaffectedByXrayHosts.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}); err != nil {
		t.Fatalf("https+local DoH uses DialSystem and must not be blocked by Xray dns.hosts: %v", err)
	}
	for _, explicitLocal := range []string{`"https+local://localhost/dns-query"`, `"https+local://127.0.0.1/dns-query"`} {
		pLocalSelf := semanticsPolicy(t, `"dns":{"enabled":true,"servers":[`+explicitLocal+`]}`)
		if err := checkDNSInboundPortConflict(pLocalSelf.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}); err == nil {
			t.Fatalf("explicit local DoH %s bypassed port-443 inbound guard", explicitLocal)
		}
	}

	// 4) geosite hosts 规则：区分「已匹配本地地址」「明确不匹配」「无本地 geodata 无法静态确定」三态，且绝不削弱 10085 保护。
	testGeosite, err := ParseGeositeData(buildTestGeositeBytes(map[string][]geositeDomainRule{
		"CATEGORY-ADS-ALL": {
			{Type: geositeDomainRootDomain, Value: "doubleclick.net"},
			{Type: geositeDomainFull, Value: "ads.example.com"},
		},
		"GOOGLE": {
			{Type: geositeDomainFull, Value: "dns.google"},
			{Type: geositeDomainRootDomain, Value: "google.com"},
			{Type: geositeDomainRootDomain, Value: "googleadservices.com", Attrs: []string{"ads"}},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}

	// 正例：广告屏蔽 hosts (geosite:category-ads-all -> 0.0.0.0) + 公网 DoH (https://dns.google/dns-query) + 443 入站
	pAdsAndGoogleDoH := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:category-ads-all":"0.0.0.0"},"servers":["https://dns.google/dns-query"]}`)
	matched, indeterminate := dnsHostsStaticLocalMatch("dns.google", pAdsAndGoogleDoH.DNS.Hosts, testGeosite)
	if matched || indeterminate {
		t.Fatalf("expected (matched=false, indeterminate=false) with loaded geosite, got (%v, %v)", matched, indeterminate)
	}
	if err := checkDNSInboundPortConflictWithGeosite(pAdsAndGoogleDoH.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}, testGeosite); err != nil {
		t.Fatalf("geosite:category-ads-all falsely blocked https://dns.google/dns-query: %v", err)
	}
	// 无本地 geodata 时应明确标记为 indeterminate=true（无法静态确定），而非混同为 matched=true
	matchedNil, indeterminateNil := dnsHostsStaticLocalMatch("dns.google", pAdsAndGoogleDoH.DNS.Hosts, nil)
	if matchedNil || !indeterminateNil {
		t.Fatalf("expected (matched=false, indeterminate=true) without local geodata, got (%v, %v)", matchedNil, indeterminateNil)
	}
	if err := checkDNSInboundPortConflictWithGeosite(pAdsAndGoogleDoH.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}, nil); err != nil {
		t.Fatalf("panel without local geodata falsely blocked https://dns.google/dns-query: %v", err)
	}
	// 反例：geosite:google 确实包含 dns.google 且映射至 127.0.0.1，非 +local DoH 命中 443 入站自环应被拦截
	pMatchedGeositeSelfLoop := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:google":"127.0.0.1"},"servers":["https://dns.google/dns-query"]}`)
	matchedHit, indeterminateHit := dnsHostsStaticLocalMatch("dns.google", pMatchedGeositeSelfLoop.DNS.Hosts, testGeosite)
	if !matchedHit || indeterminateHit {
		t.Fatalf("expected (matched=true, indeterminate=false) for geosite:google on dns.google, got (%v, %v)", matchedHit, indeterminateHit)
	}
	if err := checkDNSInboundPortConflictWithGeosite(pMatchedGeositeSelfLoop.DNS, []Inbound{{Tag: "vless-reality-1", Port: 443}}, testGeosite); err == nil {
		t.Fatal("geosite:google -> 127.0.0.1 failed to block https://dns.google/dns-query on port 443")
	}
	// 属性过滤：geosite:google@ads 不包含无 @ads 属性的 dns.google
	pAttrFiltered := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:google@ads":"127.0.0.1"},"servers":["https://dns.google/dns-query"]}`)
	matchedAttr, indeterminateAttr := dnsHostsStaticLocalMatch("dns.google", pAttrFiltered.DNS.Hosts, testGeosite)
	if matchedAttr || indeterminateAttr {
		t.Fatalf("expected @ads attribute filter not to match dns.google, got (%v, %v)", matchedAttr, indeterminateAttr)
	}
	// 10085 管理端口无条件保护不受影响
	if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","dns":{"enabled":true,"hosts":{"geosite:category-ads-all":"0.0.0.0"},"servers":["https://dns.google:10085/dns-query"]}}`); err == nil {
		t.Fatal("management port 10085 must remain unconditionally blocked")
	}
}

func TestRequiresIPv6FollowsPhysicalDialPath(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields string
		want   bool
	}{
		{"ipv6-target-over-ipv4-proxy", `"custom_outbounds":[{"tag":"landing","protocol":"socks","settings":{"servers":[{"address":"2001:db8::1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"underlay"}}},{"tag":"underlay","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"landing"}]`, false},
		{"ipv4-target-over-ipv6-proxy", `"custom_outbounds":[{"tag":"landing","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"underlay"}}},{"tag":"underlay","protocol":"socks","settings":{"servers":[{"address":"2001:db8::1","port":1080}]}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"landing"}]`, true},
		{"freedom-target-strategy", `"custom_outbounds":[{"tag":"egress","protocol":"freedom","settings":{"targetStrategy":"ForceIPv6","domainStrategy":"ForceIPv4"}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"egress"}]`, true},
		{"freedom-case-insensitive-strategy", `"custom_outbounds":[{"tag":"egress","protocol":"freedom","settings":{"domainStrategy":"forceipv6"}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"egress"}]`, true},
		{"sockopt-strategy", `"custom_outbounds":[{"tag":"egress","protocol":"socks","settings":{"servers":[{"address":"proxy.example","port":1080}]},"streamSettings":{"sockopt":{"domainStrategy":"ForceIPv6"}}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"egress"}]`, true},
		{"ipv6-dialer-direct-with-ipv4-literal", `"custom_outbounds":[{"tag":"egress","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"direct-v6"}}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"egress"}]`, false},
		{"dns-only-ipv6-results", `"dns":{"enabled":true,"queryStrategy":"UseIPv6","servers":["1.1.1.1"]}`, true},
		{"disabled-dns", `"dns":{"enabled":false,"queryStrategy":"UseIPv6","servers":["2001:db8::53"]}`, false},
		{"ipv6-local-dns-upstream", `"dns":{"enabled":true,"servers":["tcp+local://[2001:db8::53]"]}`, true},
		{"ipv6-routed-dns-upstream", `"dns":{"enabled":true,"servers":["2001:db8::53"]}`, true},
		{"loopback-dns-no-default-route", `"dns":{"enabled":true,"servers":["tcp+local://[::1]"]}`, false},
		{"dns-upstream-through-ipv4-proxy", `"dns":{"enabled":true,"servers":["2001:db8::53"]},"custom_outbounds":[{"tag":"proxy","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]}}],"custom_rules":[{"type":"field","ip":["2001:db8::53"],"port":"53","outboundTag":"proxy"}]`, false},
		{"unused-ipv6-outbound", `"custom_outbounds":[{"tag":"unused","protocol":"freedom","settings":{"targetStrategy":"ForceIPv6"}}]`, false},
		{"balancer-proxy-chain", `"custom_outbounds":[{"tag":"landing","protocol":"socks","settings":{"servers":[{"address":"2001:db8::1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"underlay"}}},{"tag":"underlay","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]}}],"balancers":[{"tag":"pool","selector":["landing"],"fallbackTag":"direct-v4"}],"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}]`, false},
		{"balancer-ipv6-fallback", `"balancers":[{"tag":"pool","selector":["direct-v4"],"fallbackTag":"direct-v6"}],"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}]`, true},
		{"independent-observatory-ipv4-probe", `"balancers":[{"tag":"idle","selector":["direct-v4"]}],"observatory":{"subjectSelector":["direct-v6"],"probeURL":"https://1.1.1.1/"}`, false},
		{"independent-observatory-ipv6-probe", `"balancers":[{"tag":"idle","selector":["direct-v4"]}],"observatory":{"subjectSelector":["direct-v4"],"probeURL":"https://[2606:4700:4700::1111]/"}`, true},
		{"flat-socks-ipv6-endpoint", `"custom_outbounds":[{"tag":"proxy","protocol":"socks","settings":{"address":"2001:db8::1","port":1080}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"proxy"}]`, true},
		{"flat-vless-ipv6-endpoint", `"custom_outbounds":[{"tag":"proxy","protocol":"vless","settings":{"address":"2001:db8::1","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"proxy"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := semanticsPolicy(t, tc.fields)
			if got := p.RequiresIPv6(); got != tc.want {
				t.Fatalf("RequiresIPv6=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestIPv6AnalysisUsesAutomaticObservatoryProbeURL(t *testing.T) {
	p := semanticsPolicy(t, `"balancers":[{"tag":"pool","selector":["direct-v4"],"strategy":{"type":"leastPing"}}],"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}]`)
	if p.RequiresIPv6() {
		t.Fatal("default IPv4 probe unexpectedly requires IPv6")
	}
	if !p.RequiresIPv6WithOptions(RenderOptions{DefaultProbeURL: "https://[2606:4700:4700::1111]/"}) {
		t.Fatal("automatic observatory's configured IPv6 probe was ignored")
	}
	if p.RequiresIPv6WithOptions(RenderOptions{DefaultProbeURL: "https://[::1]/"}) {
		t.Fatal("invalid probe URL should fall back to safe IPv4 default")
	}
}

func TestIPv6AnalysisUsesMergedOutboundAndDNSOverrides(t *testing.T) {
	base := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"landing","protocol":"socks","settings":{"servers":[{"address":"2001:db8::1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"underlay"}}},{"tag":"underlay","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]}}],"custom_rules":[{"type":"field","port":"443","outboundTag":"landing"}]}`
	for _, tc := range []struct {
		override string
		want     bool
	}{
		{`{"outbound_overrides":{"underlay":{"mode":"replace","outbound":{"tag":"underlay","protocol":"socks","settings":{"servers":[{"address":"2001:db8::2","port":1080}]}}}}}`, true},
		{`{"outbound_overrides":{"underlay":{"mode":"remap","target_tag":"direct"}}}`, true},
		{`{"outbound_overrides":{"landing":{"mode":"remap","target_tag":"direct-v4"}}}`, false},
		{`{"dns_override":{"enabled":true,"queryStrategy":"UseIPv6","servers":["1.1.1.1"]}}`, true},
	} {
		p, err := MergeRoutingPolicy(base, tc.override)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.RequiresIPv6(); got != tc.want {
			t.Fatalf("override %s: RequiresIPv6=%v, want %v", tc.override, got, tc.want)
		}
	}
}

func TestRemapKeepsStillMatchingSelectorPrefixes(t *testing.T) {
	base := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"landing","protocol":"freedom"},{"tag":"landing2","protocol":"freedom"}],"balancers":[{"tag":"pool","selector":["landing"],"strategy":{"type":"leastPing"}}],"observatory":{"subjectSelector":["landing"],"probeURL":"https://1.1.1.1/"},"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}]}`
	p, err := MergeRoutingPolicy(base, `{"outbound_overrides":{"landing":{"mode":"remap","target_tag":"direct"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Balancers) != 1 || len(p.Balancers[0].Selector) != 1 || p.Balancers[0].Selector[0] != "landing" || p.Observatory == nil || p.Observatory.SubjectSelector[0] != "landing" {
		t.Fatalf("prefix selectors lost after remap: %+v", p)
	}
	rendered, err := ApplyRoutingPolicy(semanticsBaseConfig, p)
	if err != nil || !strings.Contains(rendered, `"tag": "landing2"`) {
		t.Fatalf("remaining candidate was not rendered: %v\n%s", err, rendered)
	}
}

func TestNegativeGeoIPRenderingIsVersionCompatible(t *testing.T) {
	for _, rule := range []string{"!geoip:cn", "geoip:!cn"} {
		p := semanticsPolicy(t, `"dns":{"enabled":true,"servers":[{"address":"1.1.1.1","expectIPs":["`+rule+`"]}]},"custom_rules":[{"type":"field","ip":["`+rule+`"],"outboundTag":"direct"}]`)
		rendered, err := ApplyRoutingPolicy(semanticsBaseConfig, p)
		if err != nil || strings.Contains(rendered, "!geoip:") || strings.Count(rendered, "geoip:!cn") != 2 {
			t.Fatalf("negative GeoIP was not normalized: %v\n%s", err, rendered)
		}
	}
}

func TestIPv6ParitySharedFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "ipv6_parity_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name   string          `json:"name"`
		Want   bool            `json:"want"`
		Policy json.RawMessage `json:"policy"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			p, err := DecodeRoutingPolicy(string(tc.Policy))
			if err != nil {
				t.Fatalf("DecodeRoutingPolicy failed: %v", err)
			}
			if got := p.RequiresIPv6(); got != tc.Want {
				t.Fatalf("RequiresIPv6()=%v, want %v", got, tc.Want)
			}
		})
	}
}

// 同一组 fixture 分别用 XRAY_TEST_BINARY 指定最低支持内核和当前内核，只运行 -test。
func TestActualXraySchemaV3DNSBalancerFixtures(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to the Xray binary under test")
	}
	t.Run("port443-ads-geosite-and-google-doh", func(t *testing.T) {
		node := &entity.Node{ID: 1, Name: "fixture-443", IP: "127.0.0.1"}
		reality443 := &entity.InboundProfile{
			ID:       1,
			Name:     "Reality-443",
			Protocol: types.ProtocolVlessReality,
			Port:     443,
			Active:   true,
			Settings: `{"sni":"www.microsoft.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"],"fingerprint":"chrome"}`,
		}
		keys := []entity.NodeProfileKey{{ProfileID: 1, Profile: reality443, Settings: "{}"}}
		users := []entity.User{{UUID: "11111111-1111-4111-8111-111111111111", Username: "fixture", Active: true}}
		baseWith443, warnings, genErr := GenerateConfig(node, keys, users, LogConfig{Level: "none", Access: "none"})
		if genErr != nil || len(warnings) > 0 {
			t.Fatalf("GenerateConfig port 443 failed: %v %v", genErr, warnings)
		}
		pAllowed := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:category-ads-all":"0.0.0.0"},"servers":["https://dns.google/dns-query"]}`)
		rendered, err := ApplyRoutingPolicy(baseWith443, pAllowed)
		if err != nil {
			t.Fatalf("expected ads geosite + google DoH on port-443 node to render, got: %v", err)
		}
		path := filepath.Join(t.TempDir(), "port443-doh.json")
		if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", path)
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("actual Xray preflight failed: %v\n%s", err, output)
		}
		// 若提供已核验的目标节点 geosite.dat（摘要匹配），验证 geosite:google -> 127.0.0.1 在 443 入站下确定性拦截
		if realGeosite := ResolveDiagnosticLocalGeositeDB(""); realGeosite != nil {
			pBlocked := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:google":"127.0.0.1"},"servers":["https://dns.google/dns-query"]}`)
			if _, err := ApplyRoutingPolicyWithOptions(baseWith443, pBlocked, RenderOptions{
				NodeGeosite:       realGeosite,
				NodeGeositeSHA256: realGeosite.SHA256(),
			}); err == nil {
				t.Fatal("expected verified node geosite:google -> 127.0.0.1 to block https://dns.google/dns-query on port 443")
			}
		}
	})
	for _, strategy := range []string{"random", "roundRobin", "leastPing"} {
		t.Run(strategy, func(t *testing.T) {
			p := semanticsPolicy(t, `"dns":{"enabled":true,"queryStrategy":"UseIPv4","hosts":{"dns.google":["8.8.8.8"]},"servers":[{"address":"tcp+local://1.1.1.1","port":5353,"expectIPs":["!geoip:cn"]},"https+local://1.1.1.1/dns-query"]},"custom_outbounds":[{"tag":"proxy","protocol":"socks","settings":{"servers":[{"address":"192.0.2.1","port":1080}]}}],"balancers":[{"tag":"pool","selector":["proxy"],"strategy":{"type":"`+strategy+`"},"fallbackTag":"direct-v4"}],"custom_rules":[{"type":"field","ip":["!geoip:cn"],"balancerTag":"pool"}]`)
			rendered, err := ApplyRoutingPolicy(semanticsBaseConfig, p)
			if err != nil {
				t.Fatal(err)
			}
			var cfg Config
			if err := json.Unmarshal([]byte(rendered), &cfg); err != nil || cfg.Observatory == nil {
				t.Fatalf("missing automatically generated observatory: %v", err)
			}
			path := filepath.Join(t.TempDir(), "routing-v3.json")
			if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", path)
			cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("actual Xray preflight: %v\n%s", err, output)
			}
		})
	}
}

func TestPanelLocalGeositeNeverHardBlocksMismatchedNodeDataset(t *testing.T) {
	panelBytes := buildTestGeositeBytes(map[string][]geositeDomainRule{
		"ADS": {{Type: geositeDomainFull, Value: "dns.google"}},
	})
	nodeBytes := buildTestGeositeBytes(map[string][]geositeDomainRule{
		"ADS": {{Type: geositeDomainFull, Value: "ads.example.com"}},
	})
	panelDB, err := ParseGeositeData(panelBytes)
	if err != nil {
		t.Fatal(err)
	}
	nodeDB, err := ParseGeositeData(nodeBytes)
	if err != nil {
		t.Fatal(err)
	}

	panelDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(panelDir, "geosite.dat"), panelBytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XRAY_LOCATION_ASSET", panelDir)

	base := `{"log":{"loglevel":"none"},"inbounds":[{"tag":"test-inbound","listen":"127.0.0.1","port":443,"protocol":"socks","settings":{"auth":"noauth","udp":false}}],"outbounds":[{"tag":"direct","protocol":"freedom"}],"routing":{"rules":[{"type":"field","inboundTag":["api-inbound"],"outboundTag":"api"},{"type":"field","ip":["geoip:private"],"outboundTag":"block"}]}}`
	p := semanticsPolicy(t, `"dns":{"enabled":true,"hosts":{"geosite:ads":"0.0.0.0"},"servers":["https://dns.google/dns-query"]}`)

	// 1. 未绑定节点已核验 geodata 时，绝不隐式读取面板本机 XRAY_LOCATION_ASSET 硬拒绝节点渲染
	if _, err := ApplyRoutingPolicy(base, p); err != nil {
		t.Fatalf("unverified panel local geosite falsely rejected node render: %v", err)
	}
	// 2. 传入未附带摘要或摘要不匹配的 geodata 时，同样不得冒充目标节点事实触发硬拒绝
	if _, err := ApplyRoutingPolicyWithOptions(base, p, RenderOptions{NodeGeosite: panelDB}); err != nil {
		t.Fatalf("missing SHA256 verification falsely rejected node render: %v", err)
	}
	if _, err := ApplyRoutingPolicyWithOptions(base, p, RenderOptions{NodeGeosite: panelDB, NodeGeositeSHA256: nodeDB.SHA256()}); err != nil {
		t.Fatalf("mismatched SHA256 falsely rejected node render: %v", err)
	}
	// 3. 诊断函数可输出非阻断 warning，且明确说明未持有目标节点已核验 geodata
	diag := DiagnoseRenderedDNSSelfLoop(base, p, RenderOptions{})
	if len(diag) != 1 || !strings.Contains(diag[0], "未持有目标节点已核验") || !strings.Contains(diag[0], "参考提示") {
		t.Fatalf("expected non-blocking diagnostic warning with local reference hint, got: %v", diag)
	}
	// 4. 当调用方提供目标节点真实 geodata 且摘要匹配时：面板 A 数据确定性拒绝，节点 B 数据确定性放行
	if _, err := ApplyRoutingPolicyWithOptions(base, p, RenderOptions{NodeGeosite: panelDB, NodeGeositeSHA256: panelDB.SHA256()}); err == nil {
		t.Fatal("verified matching dataset A should deterministically reject self-loop")
	}
	if _, err := ApplyRoutingPolicyWithOptions(base, p, RenderOptions{NodeGeosite: nodeDB, NodeGeositeSHA256: nodeDB.SHA256()}); err != nil {
		t.Fatalf("verified matching dataset B should deterministically allow render, got: %v", err)
	}
}

func TestGeositeAttributeAndCorruptionSemanticsMatchXray(t *testing.T) {
	// 1. 拒绝空属性 (@ / @@) 与非官方属性取反 (@!attr)
	for _, invalid := range []string{
		"geosite:google@",
		"geosite:google@@cn",
		"geosite:@cn",
		"geosite:google@!cn",
	} {
		if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","custom_rules":[{"type":"field","domain":["` + invalid + `"],"outboundTag":"direct"}]}`); err == nil {
			t.Errorf("expected invalid geosite syntax %q to be rejected", invalid)
		}
		if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","dns":{"enabled":true,"hosts":{"` + invalid + `":"127.0.0.1"},"servers":["1.1.1.1"]}}`); err == nil {
			t.Errorf("expected invalid dns.hosts geosite syntax %q to be rejected", invalid)
		}
	}

	// 2. Attribute protobuf 重复 key 采用末次值生效；原始属性键保留大小写（仅过滤器转小写）
	var multiKeyAttr []byte
	multiKeyAttr = protowire.AppendTag(multiKeyAttr, 1, protowire.BytesType)
	multiKeyAttr = protowire.AppendString(multiKeyAttr, "unused")
	multiKeyAttr = protowire.AppendTag(multiKeyAttr, 1, protowire.BytesType)
	multiKeyAttr = protowire.AppendString(multiKeyAttr, "cn")

	var upperKeyAttr []byte
	upperKeyAttr = protowire.AppendTag(upperKeyAttr, 1, protowire.BytesType)
	upperKeyAttr = protowire.AppendString(upperKeyAttr, "CN")

	makeDomain := func(val string, rawAttr []byte) []byte {
		var d []byte
		d = protowire.AppendTag(d, 1, protowire.VarintType)
		d = protowire.AppendVarint(d, uint64(geositeDomainFull))
		d = protowire.AppendTag(d, 2, protowire.BytesType)
		d = protowire.AppendString(d, val)
		d = protowire.AppendTag(d, 3, protowire.BytesType)
		d = protowire.AppendBytes(d, rawAttr)
		return d
	}
	var entry []byte
	entry = protowire.AppendTag(entry, 1, protowire.BytesType)
	entry = protowire.AppendString(entry, "TEST")
	entry = protowire.AppendTag(entry, 2, protowire.BytesType)
	entry = protowire.AppendBytes(entry, makeDomain("last-wins.example", multiKeyAttr))
	entry = protowire.AppendTag(entry, 2, protowire.BytesType)
	entry = protowire.AppendBytes(entry, makeDomain("upper-attr.example", upperKeyAttr))
	var list []byte
	list = protowire.AppendTag(list, 1, protowire.BytesType)
	list = protowire.AppendBytes(list, entry)

	db, err := ParseGeositeData(list)
	if err != nil {
		t.Fatal(err)
	}
	if hit, known := db.Match("test@cn", "last-wins.example"); !hit || !known {
		t.Fatalf("expected protobuf last-key-wins to match @cn, got hit=%v known=%v", hit, known)
	}
	if hit, known := db.Match("test@cn", "upper-attr.example"); hit || !known {
		t.Fatalf("expected raw uppercase 'CN' attr not to match normalized 'cn' filter, got hit=%v known=%v", hit, known)
	}

	// 3. 尾部畸形字节或非法正则必须返回 known=false，不得冒充明确不匹配
	badRegexBytes := buildTestGeositeBytes(map[string][]geositeDomainRule{
		"BADRE": {{Type: geositeDomainRegex, Value: "([unclosed"}},
	})
	badReDB, err := ParseGeositeData(badRegexBytes)
	if err != nil {
		t.Fatal(err)
	}
	if hit, known := badReDB.Match("badre", "example.com"); hit || known {
		t.Fatalf("corrupt regex category must return (false, false), got (%v, %v)", hit, known)
	}

	var trailingGarbageAttr []byte
	trailingGarbageAttr = protowire.AppendTag(trailingGarbageAttr, 1, protowire.BytesType)
	trailingGarbageAttr = protowire.AppendString(trailingGarbageAttr, "cn")
	trailingGarbageAttr = append(trailingGarbageAttr, 0xff) // truncated invalid varint tag
	var badAttrEntry []byte
	badAttrEntry = protowire.AppendTag(badAttrEntry, 1, protowire.BytesType)
	badAttrEntry = protowire.AppendString(badAttrEntry, "BADATTR")
	badAttrEntry = protowire.AppendTag(badAttrEntry, 2, protowire.BytesType)
	badAttrEntry = protowire.AppendBytes(badAttrEntry, makeDomain("example.com", trailingGarbageAttr))
	var badAttrList []byte
	badAttrList = protowire.AppendTag(badAttrList, 1, protowire.BytesType)
	badAttrList = protowire.AppendBytes(badAttrList, badAttrEntry)
	badAttrDB, err := ParseGeositeData(badAttrList)
	if err != nil {
		t.Fatal(err)
	}
	if hit, known := badAttrDB.Match("badattr@cn", "example.com"); hit || known {
		t.Fatalf("corrupt attribute bytes must return (false, false), got (%v, %v)", hit, known)
	}

	// 4. 非法 UTF-8 字符串（country_code / domain.value / attribute.key）必须拒绝或标记为 unknown，不得返回确定匹配
	var nonUTF8Attr []byte
	nonUTF8Attr = protowire.AppendTag(nonUTF8Attr, 1, protowire.BytesType)
	nonUTF8Attr = protowire.AppendBytes(nonUTF8Attr, []byte{0xff})
	var nonUTF8AttrEntry []byte
	nonUTF8AttrEntry = protowire.AppendTag(nonUTF8AttrEntry, 1, protowire.BytesType)
	nonUTF8AttrEntry = protowire.AppendString(nonUTF8AttrEntry, "BADUTF8")
	nonUTF8AttrEntry = protowire.AppendTag(nonUTF8AttrEntry, 2, protowire.BytesType)
	nonUTF8AttrEntry = protowire.AppendBytes(nonUTF8AttrEntry, makeDomain("dns.google", nonUTF8Attr))
	var nonUTF8AttrList []byte
	nonUTF8AttrList = protowire.AppendTag(nonUTF8AttrList, 1, protowire.BytesType)
	nonUTF8AttrList = protowire.AppendBytes(nonUTF8AttrList, nonUTF8AttrEntry)
	nonUTF8AttrDB, err := ParseGeositeData(nonUTF8AttrList)
	if err != nil {
		t.Fatal(err)
	}
	if hit, known := nonUTF8AttrDB.Match("badutf8", "dns.google"); hit || known {
		t.Fatalf("invalid UTF-8 attribute.key must return (false, false), got (%v, %v)", hit, known)
	}

	var nonUTF8ValDom []byte
	nonUTF8ValDom = protowire.AppendTag(nonUTF8ValDom, 1, protowire.VarintType)
	nonUTF8ValDom = protowire.AppendVarint(nonUTF8ValDom, uint64(geositeDomainFull))
	nonUTF8ValDom = protowire.AppendTag(nonUTF8ValDom, 2, protowire.BytesType)
	nonUTF8ValDom = protowire.AppendBytes(nonUTF8ValDom, []byte{0xff, 'a'})
	var nonUTF8ValEntry []byte
	nonUTF8ValEntry = protowire.AppendTag(nonUTF8ValEntry, 1, protowire.BytesType)
	nonUTF8ValEntry = protowire.AppendString(nonUTF8ValEntry, "BADVAL")
	nonUTF8ValEntry = protowire.AppendTag(nonUTF8ValEntry, 2, protowire.BytesType)
	nonUTF8ValEntry = protowire.AppendBytes(nonUTF8ValEntry, nonUTF8ValDom)
	var nonUTF8ValList []byte
	nonUTF8ValList = protowire.AppendTag(nonUTF8ValList, 1, protowire.BytesType)
	nonUTF8ValList = protowire.AppendBytes(nonUTF8ValList, nonUTF8ValEntry)
	nonUTF8ValDB, err := ParseGeositeData(nonUTF8ValList)
	if err != nil {
		t.Fatal(err)
	}
	if hit, known := nonUTF8ValDB.Match("badval", "dns.google"); hit || known {
		t.Fatalf("invalid UTF-8 domain.value must return (false, false), got (%v, %v)", hit, known)
	}

	var nonUTF8CodeEntry []byte
	nonUTF8CodeEntry = protowire.AppendTag(nonUTF8CodeEntry, 1, protowire.BytesType)
	nonUTF8CodeEntry = protowire.AppendBytes(nonUTF8CodeEntry, []byte{0xff})
	var nonUTF8CodeList []byte
	nonUTF8CodeList = protowire.AppendTag(nonUTF8CodeList, 1, protowire.BytesType)
	nonUTF8CodeList = protowire.AppendBytes(nonUTF8CodeList, nonUTF8CodeEntry)
	if _, err := ParseGeositeData(nonUTF8CodeList); err == nil {
		t.Fatal("invalid UTF-8 country_code must fail ParseGeositeData")
	}
}

func TestMappedIPv4CIDRValidationAndCeiling(t *testing.T) {
	for _, validCIDR := range []string{"::ffff:0:0/0", "::ffff:0:0/1", "::ffff:192.0.2.0/24", "::ffff:192.0.2.1/32"} {
		if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","custom_rules":[{"type":"field","ip":["` + validCIDR + `"],"outboundTag":"direct"}]}`); err != nil {
			t.Fatalf("expected valid mapped CIDR %s to pass, got %v", validCIDR, err)
		}
	}
	for _, invalidCIDR := range []string{"::ffff:0:0/33", "::ffff:192.0.2.1/128", "192.0.2.1/33", "2001:db8::/129"} {
		if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","custom_rules":[{"type":"field","ip":["` + invalidCIDR + `"],"outboundTag":"direct"}]}`); err == nil {
			t.Errorf("expected out-of-range CIDR %s to be rejected in routing rules", invalidCIDR)
		}
		if _, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","dns":{"enabled":true,"servers":[{"address":"1.1.1.1","expectIPs":["` + invalidCIDR + `"]}]}}`); err == nil {
			t.Errorf("expected out-of-range CIDR %s to be rejected in dns.expectIPs", invalidCIDR)
		}
	}
}
