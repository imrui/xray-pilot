package xray

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObservatoryProbeURLCannotResolveIntoPrivateNetwork(t *testing.T) {
	rawFixture, err := os.ReadFile(filepath.Join("testdata", "observatory_url_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Valid   []string `json:"valid"`
		Invalid []string `json:"invalid"`
	}
	if err := json.Unmarshal(rawFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, raw := range fixture.Valid {
		if err := ValidateObservatoryProbeURL(raw); err != nil {
			t.Errorf("public literal %q: %v", raw, err)
		}
	}
	for _, raw := range fixture.Invalid {
		if raw == "" {
			continue
		}
		if err := ValidateObservatoryProbeURL(raw); err == nil {
			t.Errorf("accepted unsafe probe URL %q", raw)
		}
	}
}

func TestObservatoryDefaultRejectsUnsafeSystemProbeSetting(t *testing.T) {
	p, err := DecodeRoutingPolicy(`{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"egress","protocol":"freedom"}],"balancers":[{"tag":"pool","selector":["egress"],"strategy":{"type":"leastPing"}}],"custom_rules":[{"type":"field","port":"443","balancerTag":"pool"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "https://localhost/", "https://www.gstatic.com/generate_204"} {
		rendered, err := ApplyRoutingPolicyWithOptions(`{"log":{},"routing":{"rules":[]},"inbounds":[],"outbounds":[]}`, p, RenderOptions{DefaultProbeURL: setting})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(rendered, DefaultObservatoryProbeURL) || strings.Contains(rendered, "localhost") || strings.Contains(rendered, "gstatic") {
			t.Errorf("unsafe system setting leaked into observatory: %s", rendered)
		}
	}
}
