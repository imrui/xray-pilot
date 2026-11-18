package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

func TestRestoreHealthyInheritedOutboundRemapExactly(t *testing.T) {
	for _, legacySnapshot := range []bool{false, true} {
		name := "saved-overrides"
		if legacySnapshot {
			name = "legacy-snapshot"
		}
		t.Run(name, func(t *testing.T) {
			n, _ := routingFixture(t)
			s := NewRoutingService()
			raw := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"proxy-a","protocol":"freedom"},{"tag":"proxy-b","protocol":"socks","settings":{"servers":[{"address":"203.0.113.1","port":1080,"users":[{"user":"ops","pass":"base-password"}]}]}}],"balancers":[{"tag":"pool","selector":["proxy-b"],"strategy":{"type":"random"},"fallbackTag":"proxy-a"}],"custom_rules":[{"type":"field","domain":["domain:example.com"],"balancerTag":"pool"}]}`
			rev := publishRouting(t, "restore-remap", raw)
			global, _ := s.Binding("global", 0)
			global.Mode, global.TemplateID, global.Revision = "template", rev.TemplateID, rev.Revision
			if err := s.SaveBinding(global, global.Version, "admin:root", true); err != nil {
				t.Fatal(err)
			}
			b, _ := s.Binding("node", n.ID)
			b.Mode = "inherit"
			b.Overrides = `{"outbound_overrides":{"proxy-a":{"mode":"remap","target_tag":"direct-v4"},"proxy-b":{"mode":"replace","outbound":{"tag":"proxy-b","protocol":"socks","settings":{"servers":[{"address":"203.0.113.2","port":1080,"users":[{"user":"ops","pass":"node-password"}]}]}}}}}`
			if err := s.SaveBinding(b, b.Version, "admin:root", true); err != nil {
				t.Fatal(err)
			}
			origPush := pushConfig
			t.Cleanup(func() { pushConfig = origPush })
			pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
				return &xray.SyncResult{Success: true, ProcessID: "test-pid"}
			}
			if result := NewSyncService().SyncNode(n.ID, "admin:root"); !result.Success {
				t.Fatal(result.Error)
			}
			var state entity.NodeConfigState
			if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(state.LastHealthyOverrides, xray.OutboundSecretPrefix) || strings.Contains(state.LastHealthyOverrides, "node-password") {
				t.Fatal("healthy override snapshot did not preserve encrypted credentials")
			}
			stateJSON, _ := json.Marshal(state)
			if strings.Contains(string(stateJSON), "outbound_overrides") || strings.Contains(string(stateJSON), xray.OutboundSecretPrefix) {
				t.Fatal("private healthy snapshot leaked through config-state API")
			}
			var forgedState entity.NodeConfigState
			if err := json.Unmarshal([]byte(`{"LastHealthyOverrides":"forged","last_healthy_overrides":"forged","LastHealthyPolicy":"forged","last_healthy_policy":"forged"}`), &forgedState); err != nil {
				t.Fatal(err)
			}
			if forgedState.LastHealthyOverrides != "" || forgedState.LastHealthyPolicy != "" {
				t.Fatal("request body populated private healthy snapshot fields")
			}
			if legacySnapshot {
				if err := repository.DB.Model(&state).Update("last_healthy_overrides", "").Error; err != nil {
					t.Fatal(err)
				}
			}
			b, _ = s.Binding("node", n.ID)
			b.Overrides = "{}"
			if err := s.SaveBinding(b, b.Version, "admin:root", true); err != nil {
				t.Fatal(err)
			}
			b, _ = s.Binding("node", n.ID)
			if err := NewSyncService().RestorePolicy(n.ID, b.Version, "admin:root"); err != nil {
				t.Fatal(err)
			}
			restored, err := s.Effective(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			restoredJSON, _ := json.Marshal(restored.Policy)
			if !xray.PoliciesSemanticallyEqual(string(restoredJSON), state.LastHealthyPolicy) || restored.Policy.Balancers[0].FallbackTag != "direct-v4" {
				t.Fatal("restore changed the verified policy")
			}
		})
	}
}

func TestCredentialSafetyAtSaveAndResponseBoundaries(t *testing.T) {
	routingFixture(t)
	raw := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"egress","protocol":"socks","settings":{"servers":[{"address":"203.0.113.1","port":1080,"users":[{"user":"ops","Pass":"case-secret"}]}]}}]}`
	if _, err := NewRoutingService().SaveTemplate(0, 0, "unsafe-credentials", "", raw, "admin:root"); err == nil {
		t.Fatal("noncanonical credentials accepted into storage")
	}
	legacy := `{"outbounds":[{"settings":{"Password":"old-secret","ID":"old-id","pass":"x","secretKey":"escaped\"key"}}]}`
	redacted := redactConfig(legacy)
	for _, secret := range []string{"old-secret", "old-id", `escaped\"key`} {
		if strings.Contains(redacted, secret) {
			t.Fatal("preview leaked historical credential")
		}
	}
	message := `password old-secret, uuid old-id, short x, escaped escaped\"key`
	scrubbed := scrubConfigSecretsFromMessage(message, legacy)
	for _, secret := range []string{"old-secret", "old-id", `escaped\"key`} {
		if strings.Contains(scrubbed, secret) {
			t.Fatalf("error response leaked %s", secret)
		}
	}
	if !strings.Contains(scrubbed, "short <masked>") {
		t.Fatal("short password not scrubbed")
	}
}

func TestHealthySnapshotEncryptsLegacyFlatCredentials(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	rev := publishRouting(t, "legacy-flat", `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"egress","protocol":"trojan","settings":{"address":"203.0.113.1","port":443,"password":"template-password"}}],"custom_rules":[{"type":"field","domain":["domain:example.com"],"outboundTag":"egress"}]}`)
	b, _ := s.Binding("node", n.ID)
	b.Mode, b.TemplateID, b.Revision = "template", rev.TemplateID, rev.Revision
	if err := s.SaveBinding(b, b.Version, "admin:root", true); err != nil {
		t.Fatal(err)
	}
	// 模拟升级前已落库的简写明文，不能通过本次新增的 SaveBinding 加密掩盖问题。
	legacyOverrides := `{"outbound_overrides":{"egress":{"mode":"replace","outbound":{"tag":"egress","protocol":"trojan","settings":{"address":"203.0.113.2","port":443,"password":"legacy-flat-password"}}}}}`
	if err := repository.DB.Model(&entity.RoutingBinding{}).Where("scope = ? AND target_id = ?", "node", n.ID).Update("overrides", legacyOverrides).Error; err != nil {
		t.Fatal(err)
	}
	origPush := pushConfig
	t.Cleanup(func() { pushConfig = origPush })
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		return &xray.SyncResult{Success: true, ProcessID: "test-pid"}
	}
	if result := NewSyncService().SyncNode(n.ID, "admin:root"); !result.Success {
		t.Fatal(result.Error)
	}
	var state entity.NodeConfigState
	if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{state.LastHealthyPolicy, state.LastHealthyOverrides} {
		if strings.Contains(raw, "legacy-flat-password") || !strings.Contains(raw, xray.OutboundSecretPrefix) {
			t.Fatal("healthy snapshot persisted legacy plaintext")
		}
	}
	if err := NewSyncService().RestorePolicy(n.ID, b.Version+1, "admin:root"); err != nil {
		t.Fatal(err)
	}
}
