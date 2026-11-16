package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
)

// A node can keep multiple protocols while replacing its one routing selection.
// Exercise the same binding -> renderer -> preview chain used by the UI.
func TestRoutingSelectionReplacesPolicyAndPreservesProtocols(t *testing.T) {
	node, _ := routingFixture(t)
	// This fixture also runs on Windows, where the Linux log directory is absent.
	if err := NewSettingService().BatchUpdate(map[string]string{KeyXrayLogError: "none"}); err != nil {
		t.Fatal(err)
	}
	routing := NewRoutingService()
	syncer := NewSyncService()
	proxy := entity.InboundProfile{Name: "Local HTTP", Protocol: types.ProtocolHTTP, Port: 28888, Active: true, Settings: `{"allowed_ips":["127.0.0.1/32"]}`}
	if err := repository.DB.Create(&proxy).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Create(&entity.NodeProfileKey{NodeID: node.ID, ProfileID: proxy.ID, Port: 28889, Settings: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	decode := func(raw string) map[string]any {
		t.Helper()
		var cfg map[string]any
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	configs := map[string]string{}
	build := func(name string) (string, map[string]any) {
		t.Helper()
		raw, warnings, err := syncer.buildConfig(node)
		if err != nil || len(warnings) != 0 {
			t.Fatalf("%s render: %v %v", name, err, warnings)
		}
		preview, warnings, err := syncer.PreviewConfig(node.ID)
		if err != nil || len(warnings) != 0 {
			t.Fatalf("%s preview: %v %v", name, err, warnings)
		}
		cfg, shown := decode(raw), decode(preview)
		if !reflect.DeepEqual(cfg["routing"], shown["routing"]) || !reflect.DeepEqual(cfg["outbounds"], shown["outbounds"]) {
			t.Fatalf("%s preview differs from the config that would be synchronized", name)
		}
		configs[name] = raw
		return raw, cfg
	}
	initial, baseline := build("default")
	effective, err := routing.Effective(node.ID)
	global, _ := routing.Binding("global", 0)
	if err != nil || effective.Mode != "inherit" || effective.TemplateID != global.TemplateID || effective.Revision != global.Revision || effective.Policy.EnableIPv6 {
		t.Fatalf("fresh node did not inherit the conservative default: %+v %v", effective, err)
	}
	if len(baseline["inbounds"].([]any)) != 3 || !strings.Contains(initial, `"port": 28889`) || strings.Contains(initial, "direct-v6") || !strings.Contains(initial, "geoip:private") {
		t.Fatal("default lost a protocol, its port override or base routing protections")
	}
	a := publishRouting(t, "Route A", `{"schema_version":2,"ipv4_force_domains":["domain:a.example"],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[]}`)
	b := publishRouting(t, "Route B", `{"schema_version":2,"ipv4_force_domains":["domain:b.example"],"ipv6_force_domains":["domain:v6.example"],"ipv6_failure_policy":"strict","custom_rules":[{"type":"field","ip":["203.0.113.0/24"],"outboundTag":"block"}]}`)
	bind := func(template, revision uint, mode string) {
		t.Helper()
		value, err := routing.Binding("node", node.ID)
		if err != nil {
			t.Fatal(err)
		}
		value.Mode, value.TemplateID, value.Revision = mode, template, revision
		if err := routing.SaveBinding(value, value.Version, "test", true); err != nil {
			t.Fatal(err)
		}
	}
	bind(a.TemplateID, a.Revision, "template")
	rawA, cfgA := build("route-a")
	bind(b.TemplateID, b.Revision, "template")
	rawB, cfgB := build("route-b")
	if !strings.Contains(rawA, "domain:a.example") || strings.Contains(rawB, "domain:a.example") || !strings.Contains(rawB, "domain:b.example") || !strings.Contains(rawB, "203.0.113.0/24") || !strings.Contains(rawB, "direct-v6") {
		t.Fatal("template selection stacked old rules or omitted a required IPv6 outlet")
	}
	for name, cfg := range map[string]map[string]any{"a": cfgA, "b": cfgB} {
		if !reflect.DeepEqual(cfg["inbounds"], baseline["inbounds"]) {
			t.Fatalf("route %s changed protocol bindings, credentials, ports or users", name)
		}
		baseRules := baseline["routing"].(map[string]any)["rules"].([]any)
		rules := cfg["routing"].(map[string]any)["rules"].([]any)
		if !reflect.DeepEqual(baseRules, rules[:len(baseRules)]) {
			t.Fatalf("route %s changed protected API/private-IP/proxy allowlist rules", name)
		}
	}
	var count int64
	if err := repository.DB.Model(&entity.RoutingBinding{}).Where("scope = ? AND target_id = ?", "node", node.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("node has more than one routing binding: %d %v", count, err)
	}
	bind(0, 0, "inherit")
	_, restored := build("inherit-again")
	if !reflect.DeepEqual(restored, baseline) {
		t.Fatal("returning to inheritance retained old template rules")
	}
	if binary := os.Getenv("XRAY_TEST_BINARY"); binary != "" {
		for name, raw := range configs {
			t.Run(name, func(t *testing.T) {
				file := filepath.Join(t.TempDir(), "config.json")
				if err := os.WriteFile(file, []byte(raw), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", file)
				cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("Xray preflight: %v %s", err, out)
				}
			})
		}
	}
}
