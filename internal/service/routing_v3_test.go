package service

import (
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

func TestSchemaV3ServiceEncryptionDriftStabilityAndAdminGuardrails(t *testing.T) {
	node, _ := routingFixture(t)
	routingSvc := NewRoutingService()
	syncSvc := NewSyncService()

	draftV3 := `{
		"schema_version": 3,
		"domain_strategy": "IPIfNonMatch",
		"ipv4_force_domains": ["domain:openai.com"],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [
			{
				"tag": "warp-egress",
				"protocol": "wireguard",
				"remark": "WARP",
				"settings": {
					"secretKey": "wg-plain-secret",
					"address": ["172.16.0.2/32"],
					"peers": [{
						"endpoint": "engage.cloudflareclient.com:2408",
						"publicKey": "wg-pub",
						"preSharedKey": "wg-plain-psk"
					}]
				}
			},
			{
				"tag": "hk-socks",
				"protocol": "socks",
				"settings": {
					"servers": [{
						"address": "203.0.113.10",
						"port": 1080,
						"users": [{"user": "ops", "pass": "socks-plain-pass"}]
					}]
				}
			}
		],
		"custom_rules": [
			{"type": "field", "domain": ["geosite:openai"], "outboundTag": "warp-egress"},
			{"type": "field", "domain": ["geosite:netflix"], "outboundTag": "hk-socks"}
		]
	}`

	// 1. 保存模板应自动加密，重复保存同明文保持密文幂等，发布后 DraftChanged 为 false
	tmpl, err := routingSvc.SaveTemplate(0, 0, "v3-full", "schema v3 test", draftV3, "admin:root")
	if err != nil {
		t.Fatalf("SaveTemplate failed: %v", err)
	}
	if strings.Contains(tmpl.Draft, "wg-plain-secret") || strings.Contains(tmpl.Draft, "socks-plain-pass") || !strings.Contains(tmpl.Draft, xray.OutboundSecretPrefix) {
		t.Fatalf("template draft secrets not encrypted: %s", tmpl.Draft)
	}
	firstDraft := tmpl.Draft
	tmpl2, err := routingSvc.SaveTemplate(tmpl.ID, tmpl.Version, "v3-full", "schema v3 test", draftV3, "admin:root")
	if err != nil {
		t.Fatalf("second SaveTemplate failed: %v", err)
	}
	if tmpl2.Draft != firstDraft {
		t.Fatal("re-saving identical plaintext changed encrypted draft")
	}
	rev, err := routingSvc.Publish(tmpl2.ID, tmpl2.Version, "admin:root")
	if err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	overview, err := routingSvc.Overview()
	if err != nil {
		t.Fatal(err)
	}
	for _, summary := range overview.Templates {
		if summary.ID == tmpl2.ID && summary.DraftChanged {
			t.Fatal("newly published v3 template falsely marked DraftChanged")
		}
	}

	// 2. 绑定节点并设置 outbound_overrides（replace 含明文密码 + remap），验证不误触发二次漂移
	b, err := routingSvc.Binding("node", node.ID)
	if err != nil {
		t.Fatal(err)
	}
	b.Mode = "template"
	b.TemplateID = rev.TemplateID
	b.Revision = rev.Revision
	b.Overrides = `{
		"ipv4_force_domains": ["domain:custom.example.com"],
		"outbound_overrides": {
			"hk-socks": {
				"mode": "replace",
				"outbound": {
					"tag": "hk-socks",
					"protocol": "socks",
					"settings": {
						"servers": [{
							"address": "198.51.100.88",
							"port": 2080,
							"users": [{"user": "node-ops", "pass": "node-override-pass"}]
						}]
					}
				}
			},
			"warp-egress": {
				"mode": "remap",
				"target_tag": "direct-v4"
			}
		}
	}`
	var impact1 BindingImpact
	if err := routingSvc.SaveBinding(b, b.Version, "admin:root", true, &impact1); err != nil {
		t.Fatalf("SaveBinding v3 override failed: %v", err)
	}
	if len(impact1.Changed) != 1 || impact1.Changed[0] != node.ID {
		t.Fatalf("expected node changed on initial override, got %+v", impact1)
	}
	savedBinding, _ := routingSvc.Binding("node", node.ID)
	if strings.Contains(savedBinding.Overrides, "node-override-pass") || !strings.Contains(savedBinding.Overrides, xray.OutboundSecretPrefix) {
		t.Fatalf("override secret not encrypted: %s", savedBinding.Overrides)
	}

	// 再次提交相同明文覆盖，不应产生节点失效变更
	var stateBefore entity.NodeConfigState
	repository.DB.First(&stateBefore, "node_id = ?", node.ID)
	b2 := savedBinding
	b2.Overrides = b.Overrides // 明文重新提交
	var impact2 BindingImpact
	if err := routingSvc.SaveBinding(b2, b2.Version, "admin:root", true, &impact2); err != nil {
		t.Fatalf("second SaveBinding failed: %v", err)
	}
	var stateAfter entity.NodeConfigState
	repository.DB.First(&stateAfter, "node_id = ?", node.ID)
	if len(impact2.Changed) != 0 || stateAfter.ChangeRevision != stateBefore.ChangeRevision {
		t.Fatalf("idempotent re-save falsely invalidated node: impact=%+v rev=%d->%d", impact2, stateBefore.ChangeRevision, stateAfter.ChangeRevision)
	}

	// 3. PreviewConfig 扩展脱敏检查（secretKey / preSharedKey / pass 全部为 <masked>）
	// 先切换到保留 warp-egress 的无 remap 绑定以验证 WireGuard 密钥脱敏
	bNoRemap, _ := routingSvc.Binding("node", node.ID)
	bNoRemap.Overrides = `{"outbound_overrides":{"hk-socks":{"mode":"replace","outbound":{"tag":"hk-socks","protocol":"socks","settings":{"servers":[{"address":"198.51.100.88","port":2080,"users":[{"user":"node-ops","pass":"node-override-pass"}]}]}}}}}`
	if err := routingSvc.SaveBinding(bNoRemap, bNoRemap.Version, "admin:root", true); err != nil {
		t.Fatal(err)
	}
	previewCfg, warnings, err := syncSvc.PreviewConfig(node.ID)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("PreviewConfig failed: %v %v", err, warnings)
	}
	for _, secret := range []string{"wg-plain-secret", "wg-plain-psk", "node-override-pass", xray.OutboundSecretPrefix} {
		if strings.Contains(previewCfg, secret) {
			t.Fatalf("PreviewConfig leaked secret %q:\n%s", secret, previewCfg)
		}
	}
	if !strings.Contains(previewCfg, `"secretKey": "<masked>"`) || !strings.Contains(previewCfg, `"preSharedKey": "<masked>"`) {
		t.Fatalf("PreviewConfig did not redact wireguard keys:\n%s", previewCfg)
	}

	// 4. 普通 admin 权限边界：禁止修改 outbound_overrides，但允许保留原脱敏 outbound_overrides 的同时修改域名覆盖
	currentBinding, _ := routingSvc.Binding("node", node.ID)
	adminAttempt := currentBinding
	adminAttempt.Overrides = `{"outbound_overrides":{"warp-egress":{"mode":"remap","target_tag":"direct"}}}`
	if err := routingSvc.SaveBinding(adminAttempt, adminAttempt.Version, "admin:ops", false); err == nil {
		t.Fatal("normal admin was allowed to modify outbound_overrides")
	}

	maskedForAdmin := xray.MaskOverridesJSON(currentBinding.Overrides)
	adminDomainEdit := currentBinding
	adminDomainEdit.Overrides = strings.TrimSuffix(maskedForAdmin, "}") + `,"ipv4_force_domains":["domain:admin-allowed.com"]}`
	if err := routingSvc.SaveBinding(adminDomainEdit, adminDomainEdit.Version, "admin:ops", false); err != nil {
		t.Fatalf("normal admin blocked from updating ipv4_force_domains when preserving masked outbound_overrides: %v", err)
	}
	afterAdminSave, _ := routingSvc.Binding("node", node.ID)
	if strings.Contains(afterAdminSave.Overrides, xray.MaskedPlaceholder) || !strings.Contains(afterAdminSave.Overrides, xray.OutboundSecretPrefix) {
		t.Fatalf("admin save corrupted stored encrypted secret: %s", afterAdminSave.Overrides)
	}

	// 5. 同步并验证手动 RestorePolicy 对 Schema 3 生效
	origPush := pushConfig
	t.Cleanup(func() { pushConfig = origPush })
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		if !strings.Contains(raw, "node-override-pass") || !strings.Contains(raw, "wg-plain-secret") {
			t.Fatalf("sync config did not decrypt outbound secrets:\n%s", raw)
		}
		return &xray.SyncResult{Success: true, ProcessID: "pid-1"}
	}
	if r := syncSvc.SyncNode(node.ID, "admin:root"); !r.Success {
		t.Fatalf("SyncNode failed: %s", r.Error)
	}
	latestBinding, _ := routingSvc.Binding("node", node.ID)
	latestBinding.Overrides = `{"ipv4_force_domains":["domain:broken.com"]}`
	if err := routingSvc.SaveBinding(latestBinding, latestBinding.Version, "admin:root", true); err != nil {
		t.Fatal(err)
	}
	beforeRestore, _ := routingSvc.Binding("node", node.ID)
	if err := syncSvc.RestorePolicy(node.ID, beforeRestore.Version, "admin:root"); err != nil {
		t.Fatalf("RestorePolicy failed on Schema 3 policy: %v", err)
	}
	effRestored, err := routingSvc.Effective(node.ID)
	if err != nil || len(effRestored.Policy.IPv4Domains) != 1 || effRestored.Policy.IPv4Domains[0] != "domain:admin-allowed.com" {
		t.Fatalf("RestorePolicy did not restore healthy Schema 3 policy: %+v %v", effRestored, err)
	}
}
