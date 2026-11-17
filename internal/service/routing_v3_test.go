package service

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestPreflightDraftSandboxConcurrencyAndAlertIsolation(t *testing.T) {
	node, _ := routingFixture(t)
	syncSvc := NewSyncService()
	routingSvc := NewRoutingService()

	// 开启告警观察以验证草稿预检失败绝不写入任何告警状态
	_ = NewSettingService().BatchUpdate(map[string]string{
		KeyFeishuEnabled:     "true",
		KeyFeishuAppID:       "cli_dummy",
		KeyFeishuAppSecret:   "secret_dummy",
		KeyAlertEnabled:      "true",
		KeyAlertFeishuChatID: "oc_dummy",
		KeyAlertKinds:        "sync_failed",
	})

	origRemote := remoteDraftPreflight
	t.Cleanup(func() { remoteDraftPreflight = origRemote })

	sshCalls := 0
	// 1. 静态校验未通过：毫秒级直接返回 valid=false，不触发 SSH
	badStatic := json.RawMessage(`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"api","protocol":"freedom"}],"custom_rules":[]}`)
	res1, err := syncSvc.PreflightDraft(node.ID, PreflightDraftRequest{Policy: badStatic})
	if err != nil || res1.Valid || !strings.Contains(res1.Error, "保留") || sshCalls != 0 {
		t.Fatalf("expected static preflight rejection without SSH, got res=%+v err=%v sshCalls=%d", res1, err, sshCalls)
	}

	// 2. 活跃部署租约期间礼让线上同步：返回 ErrNodeSyncBusy (409)
	_, release, leaseErr := repository.AcquireConfigLease(node.ID)
	if leaseErr != nil {
		t.Fatal(leaseErr)
	}
	validDraft := json.RawMessage(`{
		"schema_version": 3,
		"domain_strategy": "IPIfNonMatch",
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [{
			"tag": "warp-draft",
			"protocol": "wireguard",
			"settings": {
				"secretKey": "draft-wg-secret-xyz",
				"peers": [{"endpoint": "162.159.192.1:2408", "publicKey": "pub"}]
			}
		}],
		"custom_rules": [{"type": "field", "domain": ["geosite:openai"], "outboundTag": "warp-draft"}]
	}`)
	_, busyErr := syncSvc.PreflightDraft(node.ID, PreflightDraftRequest{Policy: validDraft})
	release()
	if !errors.Is(busyErr, ErrNodeSyncBusy) {
		t.Fatalf("expected ErrNodeSyncBusy when lease active, got %v", busyErr)
	}

	// 3. 远端 xray -test 成功：接收解密后真实配置，响应返回脱敏配置与版本号
	remoteDraftPreflight = func(p xray.SSHParams, content string, requiresIPv6 bool) (string, error) {
		sshCalls++
		if !strings.Contains(content, "draft-wg-secret-xyz") || requiresIPv6 {
			t.Fatalf("unexpected remote preflight input: requiresIPv6=%v content=%s", requiresIPv6, content)
		}
		return "26.3.27", nil
	}
	// 先给节点绑一个不兼容旧覆盖以验证 use_node_overrides 优雅降级告警提示
	b, _ := routingSvc.Binding("node", node.ID)
	rev := publishRouting(t, "other-v3", `{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"other-tag","protocol":"freedom"}],"custom_rules":[]}`)
	b.Mode, b.TemplateID, b.Revision = "template", rev.TemplateID, rev.Revision
	b.Overrides = `{"outbound_overrides":{"other-tag":{"mode":"remap","target_tag":"direct"}}}`
	if err := routingSvc.SaveBinding(b, b.Version, "admin:root", true); err != nil {
		t.Fatal(err)
	}
	_ = repository.NewNodeRepository().UpdateSyncStatus(node.ID, entity.SyncStatusSynced, "baseline-hash")

	resOK, err := syncSvc.PreflightDraft(node.ID, PreflightDraftRequest{Policy: validDraft, UseNodeOverrides: true})
	if err != nil || !resOK.Valid || resOK.XrayVersion != "26.3.27" || len(resOK.Warnings) == 0 {
		t.Fatalf("expected valid preflight with incompatible-override warning, got %+v err=%v", resOK, err)
	}
	if strings.Contains(resOK.RedactedConfig, "draft-wg-secret-xyz") || !strings.Contains(resOK.RedactedConfig, `"secretKey": "<masked>"`) {
		t.Fatalf("redacted_config leaked secret:\n%s", resOK.RedactedConfig)
	}

	// 4. 远端 xray -test 失败（如缺少 geosite 条目且错误输出含敏感串）：脱敏错误，且零库表/状态/告警副作用
	remoteDraftPreflight = func(p xray.SSHParams, content string, requiresIPv6 bool) (string, error) {
		sshCalls++
		return "26.3.27", fmt.Errorf("PREFLIGHT_FAILED: entry NOTFOUND missing (key=draft-wg-secret-xyz)")
	}
	var logsBefore, alertsBefore int64
	repository.DB.Model(&entity.SyncLog{}).Count(&logsBefore)
	repository.DB.Model(&entity.NodeAlertState{}).Count(&alertsBefore)

	resFail, err := syncSvc.PreflightDraft(node.ID, PreflightDraftRequest{Policy: validDraft})
	if err != nil || resFail.Valid || strings.Contains(resFail.Error, "draft-wg-secret-xyz") || !strings.Contains(resFail.Error, "<masked>") {
		t.Fatalf("expected scrubbed failure result, got %+v err=%v", resFail, err)
	}
	afterNode, _ := repository.NewNodeRepository().FindByID(node.ID)
	var logsAfter, alertsAfter int64
	repository.DB.Model(&entity.SyncLog{}).Count(&logsAfter)
	repository.DB.Model(&entity.NodeAlertState{}).Count(&alertsAfter)
	if afterNode.SyncStatus != entity.SyncStatusSynced || logsAfter != logsBefore || alertsAfter != alertsBefore {
		t.Fatalf("draft preflight caused side effects! status=%s logs=%d->%d alerts=%d->%d", afterNode.SyncStatus, logsBefore, logsAfter, alertsBefore, alertsAfter)
	}
}
