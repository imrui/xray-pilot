package service

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func TestSubscriptionsIgnoreRetiredUDP443Policies(t *testing.T) {
	n, u := routingFixture(t)
	s := NewRoutingService()
	b, err := s.Binding("node", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.Overrides = `{"ipv4_force_domains":["domain:example.com"]}`
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	beforeHash, err := NewSyncService().DesiredConfigHash(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	sub := NewSubscribeService()
	generate := func() []string {
		t.Helper()
		var result []string
		for _, generate := range []func(string) (string, error){sub.GenerateSubscription, sub.GenerateClash, sub.GenerateSingbox} {
			value, err := generate(u.Token)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, value)
		}
		return result
	}
	baseline := generate()
	uri, err := base64.StdEncoding.DecodeString(baseline[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(uri), "flow=xtls-rprx-vision&") || strings.Contains(string(uri), "-udp443") {
		t.Fatal("URI did not retain standard Vision")
	}
	if !strings.HasPrefix(baseline[1], "proxies:\n") || !strings.Contains(baseline[1], "flow: xtls-rprx-vision") {
		t.Fatal("Clash lost Reality entry")
	}
	var singbox singboxConfig
	if err := json.Unmarshal([]byte(baseline[2]), &singbox); err != nil {
		t.Fatal(err)
	}
	vision := 0
	for _, outbound := range singbox.Outbounds {
		if outbound["flow"] == "xtls-rprx-vision" {
			vision++
		}
	}
	if vision != 1 {
		t.Fatalf("sing-box Reality count: %d", vision)
	}

	// New installations have no field. Existing SQLite databases may retain
	// unmapped columns; even legacy allow/block values must have no effect.
	if repository.DB.Migrator().HasColumn(&entity.RoutingBinding{}, "udp443_policy") {
		t.Fatal("new schema contains retired policy")
	}
	if err := repository.DB.Exec("ALTER TABLE routing_bindings ADD COLUMN udp443_policy TEXT").Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Create(&entity.RoutingBinding{Scope: "user", TargetID: u.ID, Mode: "inherit", Overrides: "{}", Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"global", "node", "user"} {
		for _, policy := range []string{"allow", "block", "inherit"} {
			if err := repository.DB.Exec("UPDATE routing_bindings SET udp443_policy = ? WHERE scope = ?", policy, scope).Error; err != nil {
				t.Fatal(err)
			}
			if got := generate(); !reflect.DeepEqual(got, baseline) {
				t.Fatalf("legacy %s:%s changed subscription", scope, policy)
			}
		}
	}
	afterHash, err := NewSyncService().DesiredConfigHash(n.ID)
	if err != nil || afterHash != beforeHash {
		t.Fatalf("retired policy changed routing: %v", err)
	}
	b, err = s.Binding("node", n.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(b)
	if strings.Contains(string(raw), "udp443") || b.Overrides != `{"ipv4_force_domains":["domain:example.com"]}` {
		t.Fatal("binding contract changed unrelated routing")
	}
	if _, err := s.Binding("user", u.ID); err == nil {
		t.Fatal("retired user routing scope still readable")
	}
	if err := s.SaveBinding(entity.RoutingBinding{Scope: "user", TargetID: u.ID, Mode: "inherit"}, 1, "test", true); err == nil {
		t.Fatal("retired user routing scope still writable")
	}
}
