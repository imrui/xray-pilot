package xray

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRoutingExtensionsSurvivePolicyOverrideAndRuntime(t *testing.T) {
	const raw = `{
		"schema_version":3,"ipv6_failure_policy":"strict",
		"custom_outbounds":[{"tag":"egress","protocol":"freedom"}],
		"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}],
		"dns":{"enabled":true,"clientIp":"8.8.8.8","tag":"dns-query","servers":[{"address":"1.1.1.1","clientIp":"8.8.4.4","timeoutMs":3000}]},
		"balancers":[{"tag":"pool","remark":"panel remark","selector":["egress"],"strategy":{"settings":{},"futureStrategyOption":{"threshold":9007199254740993}},"futureBalancerOption":[1,2]}],
		"observatory":{"subjectSelector":["egress"],"probeURL":"https://1.1.1.1/","futureProbeOption":{"enabled":true}}
	}`
	p, err := DecodeRoutingPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	q, err := DecodeRoutingPolicy(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, q) {
		t.Fatal("policy round trip changed extension fields")
	}
	merged, err := MergeRoutingPolicy(string(encoded), `{"dns_override":{"enabled":true,"clientIp":"9.9.9.9","servers":["1.1.1.1"]}}`)
	if err != nil || string(merged.DNS.Extra["clientIp"]) != `"9.9.9.9"` {
		t.Fatalf("DNS override lost clientIp: %+v %v", merged.DNS, err)
	}
	rendered, err := ApplyRoutingPolicy(`{"log":{},"routing":{"rules":[]},"inbounds":[],"outbounds":[]}`, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"clientIp", "timeoutMs", "futureBalancerOption", "futureStrategyOption", "9007199254740993", "futureProbeOption"} {
		if !strings.Contains(rendered, field) {
			t.Errorf("runtime lost %s: %s", field, rendered)
		}
	}
	var cfg struct {
		DNS     map[string]json.RawMessage `json:"dns"`
		Routing struct {
			Balancers []map[string]json.RawMessage `json:"balancers"`
		} `json:"routing"`
	}
	if err := json.Unmarshal([]byte(rendered), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.DNS["enabled"] != nil || cfg.Routing.Balancers[0]["remark"] != nil {
		t.Fatal("panel metadata leaked through runtime extension encoding")
	}
}

func TestRoutingExtensionFieldsCannotShadowProtectedFields(t *testing.T) {
	for _, payload := range []string{
		`"dns":{"enabled":true,"Enabled":false,"servers":["1.1.1.1"]}`,
		`"dns":{"enabled":true,"enabled":false,"servers":["1.1.1.1"]}`,
		`"dns":{"enabled":true,"servers":["1.1.1.1"],"tag":"api-inbound"}`,
		`"dns":{"enabled":true,"servers":[{"address":"1.1.1.1","tag":"api-inbound"}]}`,
		`"dns":{"enabled":true,"servers":["1.1.1.1"],"inbounds":[]}`,
		`"observatory":{"subjectSelector":["direct"],"ProbeURL":"https://127.0.0.1/"}`,
		`"observatory":{"subjectSelector":["direct"],"probeURL":"https://1.1.1.1/","probeURL":"https://127.0.0.1/"}`,
		`"balancers":[{"tag":"pool","Tag":"api","selector":["direct"]}]`,
		`"balancers":[{"tag":"pool","selector":["direct"],"strategy":{"type":"random","Type":"leastPing"}}]`,
		`"balancers":[{"tag":"pool","selector":["direct"],"strategy":{"settings":[]}}]`,
	} {
		raw := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[],` + payload + `}`
		if _, err := DecodeRoutingPolicy(raw); err == nil {
			t.Errorf("accepted protected or ambiguous extension: %s", payload)
		}
	}
}

func TestRoutingExtensionsCannotOverrideTypedValuesWhenMarshaled(t *testing.T) {
	dns := DNSConfig{Enabled: false, Extra: map[string]json.RawMessage{
		"Enabled": json.RawMessage(`true`), "queryStrategy": json.RawMessage(`"UseIPv6"`), "clientIp": json.RawMessage(`"1.1.1.1"`),
	}}
	raw, err := json.Marshal(dns)
	if err != nil {
		t.Fatal(err)
	}
	var restored DNSConfig
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Enabled || restored.QueryStrategy != "" || string(restored.Extra["clientIp"]) != `"1.1.1.1"` {
		t.Fatalf("extension shadowed known fields: %s", raw)
	}
}

func TestProgrammaticRoutingExtensionsCannotBypassValidation(t *testing.T) {
	for _, key := range []string{"probeURL", "probeurl", "ProbeURL"} {
		p := RoutingPolicy{SchemaVersion: 3, IPv6FailurePolicy: "strict", Observatory: &Observatory{
			SubjectSelector: []string{"direct"}, ProbeURL: DefaultObservatoryProbeURL,
			Extra: map[string]json.RawMessage{key: json.RawMessage(`"https://127.0.0.1/"`)},
		}}
		if _, err := ApplyRoutingPolicy(`{"log":{},"routing":{"rules":[]},"inbounds":[],"outbounds":[]}`, p); err == nil {
			t.Errorf("programmatic %s overrode the validated probe URL", key)
		}
	}
	p := RoutingPolicy{SchemaVersion: 3, IPv6FailurePolicy: "strict", DNS: &DNSConfig{
		Enabled: false, Extra: map[string]json.RawMessage{"ENABLED": json.RawMessage(`true`)},
	}}
	if err := p.Validate(); err == nil {
		t.Fatal("programmatic DNS extension overrode enabled")
	}
}
