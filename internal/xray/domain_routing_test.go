package xray

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
)

func TestDomainRoutingDefaultAndExplicitFamilies(t *testing.T) {
	base := `{"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"direct"}],"routing":{"domainStrategy":"IPIfNonMatch","rules":[{"inboundTag":["api-inbound"],"outboundTag":"api"},{"type":"field","ip":["geoip:private"],"outboundTag":"block"}]}}`
	for _, tc := range []struct {
		name   string
		v4, v6 []string
		custom []RoutingRule
	}{
		{name: "default"},
		{name: "only-v4", v4: []string{"domain:v4.example"}},
		{name: "only-v6", v6: []string{"domain:v6.example"}},
		{name: "overlap-v4-first", v4: []string{"domain:example.com"}, v6: []string{"full:www.example.com"}},
		{name: "custom-v6", custom: []RoutingRule{{Type: "field", Domain: []string{"domain:custom.example"}, OutboundTag: "direct-v6"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := RoutingPolicy{SchemaVersion: 2, IPv4Domains: tc.v4, IPv6Domains: tc.v6, CustomRules: tc.custom, IPv6FailurePolicy: "strict"}
			raw, err := ApplyRoutingPolicy(base, p)
			if err != nil {
				t.Fatal(err)
			}
			var cfg Config
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.Outbounds[0].Tag != "direct" || cfg.Outbounds[0].Protocol != "freedom" || cfg.Outbounds[0].Settings != nil {
				t.Fatal("unmatched traffic no longer uses unchanged freedom direct")
			}
			if cfg.Routing.DomainStrategy != "IPIfNonMatch" || len(cfg.Routing.Rules) < 2 || cfg.Routing.Rules[0].OutboundTag != "api" || !reflect.DeepEqual(cfg.Routing.Rules[1].IP, []string{"geoip:private"}) {
				t.Fatal("system protections changed")
			}
			for _, r := range cfg.Routing.Rules {
				if r.Network == "tcp,udp" && len(r.Domain) == 0 && len(r.IP) == 0 {
					t.Fatal("new policy added a catch-all and intercepted normal direct traffic")
				}
			}
			want := append([]RoutingRule{}, tc.custom...)
			if len(tc.v4) > 0 {
				want = append(want, RoutingRule{Type: "field", Domain: tc.v4, OutboundTag: "direct-v4"})
			}
			if len(tc.v6) > 0 {
				want = append(want, RoutingRule{Type: "field", Domain: tc.v6, OutboundTag: "direct-v6"})
			}
			if !reflect.DeepEqual(cfg.Routing.Rules[2:], want) {
				t.Fatalf("wrong family mapping/order: %+v", cfg.Routing.Rules)
			}
			found6 := false
			for _, o := range cfg.Outbounds {
				data, _ := json.Marshal(o)
				if o.Tag == "direct-v6" {
					found6 = true
					if !strings.Contains(string(data), "ForceIPv6") {
						t.Fatal("IPv6 is not strict")
					}
				}
				if o.Tag == "direct-v4" && !strings.Contains(string(data), "ForceIPv4") {
					t.Fatal("IPv4 is not strict")
				}
			}
			if found6 != p.RequiresIPv6() {
				t.Fatal("IPv6 preflight trigger does not match actual IPv6 routes")
			}
		})
	}
}

func TestDomainRoutingVersionCompatibility(t *testing.T) {
	withoutSwitch, err := DecodeRoutingPolicy(`{"ipv6_failure_policy":"strict"}`)
	if err != nil || withoutSwitch.SchemaVersion != 1 {
		t.Fatal("unversioned historical payload without switch lost its legacy behavior")
	}
	legacy := `{"enable_ipv6_outbound":false,"ipv4_force_domains":[],"ipv6_force_domains":["domain:old.example"],"ipv6_failure_policy":"strict","custom_rules":[]}`
	old, err := DecodeRoutingPolicy(legacy)
	if err != nil || old.SchemaVersion != 1 {
		t.Fatalf("legacy detection: %+v %v", old, err)
	}
	base := `{"inbounds":[],"outbounds":[],"routing":{"rules":[]}}`
	before, err := ApplyRoutingPolicy(base, old)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(old)
	restored, err := DecodeRoutingPolicy(string(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := ApplyRoutingPolicy(base, restored)
	if ConfigHash(before) != ConfigHash(after) || strings.Contains(after, "direct-v6") || !strings.Contains(after, `"network": "tcp,udp"`) {
		t.Fatal("historical healthy policy changed during round-trip")
	}
	modern, err := DecodeRoutingPolicy(entity.DefaultRoutingPolicy)
	if err != nil || modern.SchemaVersion != 2 {
		t.Fatalf("new default: %+v %v", modern, err)
	}
	if _, err := MergeRoutingPolicy(entity.DefaultRoutingPolicy, `{"enable_ipv6_outbound":false}`); err == nil {
		t.Fatal("legacy switch silently changed a new policy")
	}
	if _, err := DecodeRoutingPolicy(`{"schema_version":2,"enable_ipv6_outbound":false,"ipv6_failure_policy":"strict"}`); err == nil {
		t.Fatal("ambiguous new policy accepted")
	}
	if _, err := MergeRoutingPolicy(entity.DefaultRoutingPolicy, `{"custom_rules":[{"type":"field","domain":["domain:v6.example"],"outboundTag":"direct-v6"}]}`); err != nil {
		t.Fatal(err)
	}
}
