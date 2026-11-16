package xray

import (
	"encoding/json"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutingPolicyValidationAndMerge(t *testing.T) {
	base := `{"enable_ipv6_outbound":true,"ipv4_force_domains":["domain:a.example"],"ipv6_force_domains":["domain:b.example"],"ipv6_failure_policy":"strict","custom_rules":[]}`
	p, err := MergeRoutingPolicy(base, `{"enable_ipv6_outbound":false,"ipv4_force_domains":[],"ipv6_force_domains":null}`)
	if err != nil || p.EnableIPv6 || len(p.IPv4Domains) != 0 || len(p.IPv6Domains) != 1 {
		t.Fatalf("merge %+v %v", p, err)
	}
	for _, override := range []string{
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v6","port":"443"}],"enable_ipv6_outbound":false}`,
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v4","inboundTag":["api-inbound"],"port":"80"}]}`,
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v4","port":"0"}]}`,
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v4","ip":["999.1.1.1"]}]}`,
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v4","network":"tcp,udp"}]}`,
		`{"custom_rules":[{"type":"field","outboundTag":"direct-v4","balancerTag":"bad","port":"80"}]}`,
		`{"ipv6_failure_policy":"prefer_ipv6"}`, `{"ipv4_force_domains":["regexp:("]}`, `{"other":true}`, "null",
	} {
		if _, err := MergeRoutingPolicy(base, override); err == nil {
			t.Fatalf("accepted %s", override)
		}
	}
	p, err = MergeRoutingPolicy(base, `{"ipv4_force_domains":[" domain:a.example ","","domain:a.example"]}`)
	if err != nil || len(p.IPv4Domains) != 1 {
		t.Fatalf("normalization %+v %v", p, err)
	}
}
func TestRoutingPreservesSystemAndCredentials(t *testing.T) {
	raw := `{"log":{},"routing":{"rules":[{"inboundTag":["api-inbound"],"outboundTag":"api"},{"type":"field","ip":["geoip:private"],"outboundTag":"block"}]},"inbounds":[{"listen":"0.0.0.0","port":443,"protocol":"vless","settings":{"clients":[{"id":"test-id","email":"test-user","flow":"xtls-rprx-vision"}]},"streamSettings":{"security":"reality","realitySettings":{"privateKey":"test-key","shortIds":["abc"]}}}],"outbounds":[{"protocol":"freedom","tag":"direct"}]}`
	p, _ := DecodeRoutingPolicy(entity.DefaultRoutingPolicy)
	out, err := ApplyRoutingPolicy(raw, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"test-id", "test-user", "test-key", "abc", "geoip:private"} {
		if !strings.Contains(out, value) {
			t.Fatalf("lost %s", value)
		}
	}
	var cfg Config
	json.Unmarshal([]byte(out), &cfg)
	if cfg.Routing.Rules[0].OutboundTag != "api" || cfg.Routing.Rules[1].OutboundTag != "block" {
		t.Fatal("system protection order changed")
	}
	twice, err := ApplyRoutingPolicy(raw, p)
	if err != nil || ConfigHash(out) != ConfigHash(twice) {
		t.Fatal("nondeterministic rendering")
	}
}
func TestUserDeltaUsesCurrentSnapshotAndKeepsTransportFlow(t *testing.T) {
	old := `{"inbounds":[{"tag":"ws","protocol":"vless","settings":{"clients":[{"id":"old","email":"renamed"}]},"streamSettings":{"security":"tls"}}],"routing":{"rules":[]}}`
	next := `{"inbounds":[{"tag":"ws","protocol":"vless","settings":{"clients":[{"id":"new","email":"current"}]},"streamSettings":{"security":"tls"}}],"routing":{"rules":[]}}`
	if !SameNonUserConfig(old, next) {
		t.Fatal("pure user delta rejected")
	}
	changes, err := DiffUsers(old, next)
	if err != nil || len(changes) != 2 || !changes[0].Remove || changes[1].UUID != "new" || changes[1].Protocol != types.ProtocolVlessWSTLS {
		t.Fatalf("delta %+v %v", changes, err)
	}
	if SameNonUserConfig(old, strings.Replace(next, `"rules":[]`, `"rules":[{"outboundTag":"block"}]`, 1)) {
		t.Fatal("routing change considered a user delta")
	}
}
func TestActualXrayRoutingFixtures(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to the 26.3.27 binary")
	}
	v, err := exec.Command(binary, "version").CombinedOutput()
	if err != nil || !strings.Contains(string(v), "26.3.27") {
		t.Fatalf("baseline binary: %s %v", v, err)
	}
	n := &entity.Node{ID: 1, Name: "fixture", IP: "127.0.0.1"}
	reality := &entity.InboundProfile{ID: 1, Name: "Reality", Protocol: types.ProtocolVlessReality, Port: 24443, Active: true, Settings: `{"sni":"www.microsoft.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"],"fingerprint":"chrome"}`}
	keys := []entity.NodeProfileKey{{ProfileID: 1, Profile: reality, Settings: "{}"}}
	users := []entity.User{{UUID: "11111111-1111-4111-8111-111111111111", Username: "fixture", Active: true}}
	base, warnings, err := GenerateConfig(n, keys, users, LogConfig{Level: "none", Access: "none"})
	if err != nil || len(warnings) > 0 {
		t.Fatalf("%v %v", err, warnings)
	}
	if strings.Contains(base, "fingerprint") {
		t.Fatal("client fingerprint leaked into server")
	}
	for _, tc := range []struct {
		name string
		ipv6 bool
		ips  []string
	}{{"ipv4-only", false, nil}, {"ipv6-strict", true, nil}, {"custom-target-ips", false, []string{"1.1.1.1", "203.0.113.0/24", "2001:db8::/32"}}} {
		t.Run(tc.name, func(t *testing.T) {
			p := RoutingPolicy{EnableIPv6: tc.ipv6, IPv6FailurePolicy: "strict", IPv4Domains: []string{"domain:example.com"}, IPv6Domains: []string{"domain:example.net"}}
			if len(tc.ips) > 0 {
				p.CustomRules = []RoutingRule{{Type: "field", IP: tc.ips, OutboundTag: "direct-v4"}}
			}
			data, err := ApplyRoutingPolicy(base, p)
			if err != nil {
				t.Fatal(err)
			}
			// Match the extensionless staging files used for remote transactions.
			path := filepath.Join(t.TempDir(), ".pilot-config."+tc.name)
			os.WriteFile(path, []byte(data), 0600)
			cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", path)
			cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("actual Xray preflight: %s %v", out, err)
			}
			t.Logf("Xray 26.3.27 %s passed", tc.name)
		})
	}
}

func TestRoutingPreservesOperationalDirect(t *testing.T) {
	raw := `{"routing":{"rules":[{"type":"field","inboundTag":["proxy-http"],"source":["::1/128"],"outboundTag":"direct"}]},"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"}]}`
	policy, _ := DecodeRoutingPolicy(entity.DefaultRoutingPolicy)
	out, err := ApplyRoutingPolicy(raw, policy)
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	json.Unmarshal([]byte(out), &c)
	if c.Routing.Rules[0].OutboundTag != "direct" {
		t.Fatal("operational rule forced to IPv4")
	}
	for _, o := range c.Outbounds {
		if o.Tag == "direct" {
			data, _ := json.Marshal(o)
			if strings.Contains(string(data), "ForceIPv") {
				t.Fatal("operational outbound changed")
			}
			return
		}
	}
	t.Fatal("operational direct removed")
}
