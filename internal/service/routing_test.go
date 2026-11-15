package service

import (
	"encoding/json"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/types"
	"strings"
	"testing"
)

func routingFixture(t *testing.T) (*entity.Node, *entity.User) {
	t.Helper()
	setupInstallTestEnv(t)
	n := &entity.Node{Name: "routing-test", IP: "192.0.2.1", Active: true, SyncStatus: entity.SyncStatusSynced}
	if err := repository.NewNodeRepository().Create(n); err != nil {
		t.Fatal(err)
	}
	p := entity.InboundProfile{Name: "Reality", Protocol: types.ProtocolVlessReality, Port: 443, Active: true, Settings: `{"sni":"www.microsoft.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"]}`}
	if err := repository.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	k := entity.NodeProfileKey{NodeID: n.ID, ProfileID: p.ID, Settings: "{}"}
	if err := repository.DB.Create(&k).Error; err != nil {
		t.Fatal(err)
	}
	g := entity.Group{Name: "g", Active: true, Nodes: []entity.Node{*n}}
	if err := repository.DB.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	u := &entity.User{Username: "old-user", UUID: "11111111-1111-4111-8111-111111111111", Token: "test-token", Active: true, Groups: []entity.Group{g}}
	if err := repository.DB.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	return n, u
}
func publishRouting(t *testing.T, name, policy string) *entity.RoutingTemplateRevision {
	t.Helper()
	s := NewRoutingService()
	draft, err := s.SaveTemplate(0, 0, name, "", policy, "test")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := s.Publish(draft.ID, draft.Version, "test")
	if err != nil {
		t.Fatal(err)
	}
	return rev
}
func TestRoutingInheritanceConflictAndOverride(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	legacy := entity.Node{Name: "legacy"}
	repository.DB.Create(&legacy)
	old, err := s.Effective(legacy.ID)
	if err != nil || old.Policy != nil || old.Mode != "legacy" {
		t.Fatalf("legacy %+v %v", old, err)
	}
	fresh, err := s.Effective(n.ID)
	if err != nil || fresh.Policy == nil || fresh.Policy.EnableIPv6 {
		t.Fatalf("default %+v %v", fresh, err)
	}
	rev := publishRouting(t, "dual", `{"enable_ipv6_outbound":true,"ipv4_force_domains":["domain:example.com"],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[]}`)
	b, _ := s.Binding("node", n.ID)
	b.Mode = "template"
	b.TemplateID = rev.TemplateID
	b.Revision = rev.Revision
	b.Overrides = `{"enable_ipv6_outbound":false,"ipv4_force_domains":[],"ipv6_force_domains":null}`
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	effective, err := s.Effective(n.ID)
	if err != nil || effective.Policy.EnableIPv6 || len(effective.Policy.IPv4Domains) != 0 || effective.Sources["enable_ipv6_outbound"] != "节点覆盖" {
		t.Fatalf("override %+v %v", effective, err)
	}
	if err := s.SaveBinding(b, b.Version, "test", true); err != ErrRoutingConflict {
		t.Fatalf("stale revision accepted %v", err)
	}
	b, _ = s.Binding("node", n.ID)
	b.Mode = "inherit"
	b.TemplateID = 0
	b.Revision = 0
	b.Overrides = "{}"
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	var groups []entity.Group
	repository.DB.Find(&groups)
	gb := entity.RoutingBinding{Scope: "group", TargetID: groups[0].ID, Mode: "template", TemplateID: rev.TemplateID, Revision: 1}
	if err := s.SaveBinding(gb, 0, "test", true); err != nil {
		t.Fatal(err)
	}
	second := entity.Group{Name: "conflicting", Active: true, Nodes: []entity.Node{*n}}
	repository.DB.Create(&second)
	defaultBinding, _ := s.Binding("global", 0)
	gb.TargetID = second.ID
	gb.TemplateID = defaultBinding.TemplateID
	var impact BindingImpact
	if err := s.SaveBinding(gb, 0, "test", true, &impact); err != nil || !strings.Contains(impact.Conflicts[n.ID], "冲突") {
		t.Fatalf("conflict not isolated: %+v %v", impact, err)
	}
	// Group membership edits can also create conflicts; render must still refuse.
	repository.DB.Create(&entity.RoutingBinding{Scope: "group", TargetID: second.ID, Mode: "template", TemplateID: defaultBinding.TemplateID, Revision: 1, Overrides: "{}"})
	if _, err := s.Effective(n.ID); err == nil {
		t.Fatal("resolver hid group conflict")
	}
}
func TestRoutingValidationPermissionsAndImmutableRevisions(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	rev := publishRouting(t, "immutable", DefaultRoutingPolicy)
	if err := s.DeleteTemplate(rev.TemplateID); err == nil {
		t.Fatal("deleted published history")
	}
	var template entity.RoutingTemplate
	repository.DB.First(&template, rev.TemplateID)
	updated := strings.Replace(DefaultRoutingPolicy, `"ipv4_force_domains":[]`, `"ipv4_force_domains":["domain:example.com"]`, 1)
	if _, err := s.SaveTemplate(template.ID, template.Version, template.Name, "", updated, "test"); err != nil {
		t.Fatal(err)
	}
	var original entity.RoutingTemplateRevision
	repository.DB.First(&original, rev.ID)
	if original.Payload != rev.Payload {
		t.Fatal("published revision changed")
	}
	for _, raw := range []string{`{"typo":true}`, `{"custom_rules":[{"type":"field","outboundTag":"api","port":"80"}]}`, `{"enable_ipv6_outbound":"yes"}`, "null"} {
		b, _ := s.Binding("node", n.ID)
		b.Overrides = raw
		if err := s.SaveBinding(b, b.Version, "test", true); err == nil {
			t.Fatalf("invalid override accepted %s", raw)
		}
	}
	b, _ := s.Binding("node", n.ID)
	b.Overrides = `{"custom_rules":[{"type":"field","outboundTag":"block","port":"443"}]}`
	if err := s.SaveBinding(b, b.Version, "admin", false); err == nil {
		t.Fatal("admin modified advanced rules")
	}
	global, _ := s.Binding("global", 0)
	if err := s.SaveBinding(global, global.Version, "admin", false); err == nil {
		t.Fatal("admin modified global")
	}
}
func TestDeploymentIdempotencyAndDeletedNodeCleanup(t *testing.T) {
	n, _ := routingFixture(t)
	a, err := EnqueueDeployments([]uint{n.ID, n.ID}, "same-request", "test")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EnqueueDeployments([]uint{n.ID}, "same-request", "test")
	if err != nil || len(a) != 1 || len(b) != 1 || a[0].ID != b[0].ID {
		t.Fatalf("idempotency %+v %+v %v", a, b, err)
	}
	if err := repository.NewNodeRepository().Delete(n.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	repository.DB.Model(&entity.RoutingBinding{}).Where("scope = ? AND target_id = ?", "node", n.ID).Count(&count)
	if count != 0 {
		t.Fatal("orphan binding")
	}
	var job entity.ConfigDeployment
	repository.DB.First(&job, a[0].ID)
	if job.Status != "superseded" {
		t.Fatal("deleted node still queued")
	}
}
func TestRollbackRegeneratesCurrentAuthorization(t *testing.T) {
	n, u := routingFixture(t)
	svc := NewSyncService()
	originalPush := pushConfig
	t.Cleanup(func() { pushConfig = originalPush })
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		return &xray.SyncResult{Success: true, ProcessID: "test-process"}
	}
	if r := svc.SyncNode(n.ID, "test"); !r.Success {
		t.Fatal(r.Error)
	}
	b, _ := NewRoutingService().Binding("node", n.ID)
	b.Overrides = `{"ipv4_force_domains":["domain:changed.example"]}`
	if err := NewRoutingService().SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	calls := 0
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		calls++
		if calls == 1 {
			// A permission revocation occurs while the candidate is being restarted.
			if err := repository.NewUserRepository().Delete(u.ID); err != nil {
				t.Fatal(err)
			}
			return &xray.SyncResult{Error: "AUTHORIZATION_STOPPED"}
		}
		if strings.Contains(raw, u.UUID) || strings.Contains(raw, u.Username) {
			t.Fatal("rollback resurrected revoked identity")
		}
		if strings.Contains(raw, "changed.example") {
			t.Fatal("failed routing policy retained")
		}
		return &xray.SyncResult{Success: true, ProcessID: "recovery-process"}
	}
	r := svc.SyncNode(n.ID, "test")
	if r.Success || !strings.Contains(r.Error, "ROLLED_BACK") || calls != 2 {
		t.Fatalf("unexpected result %+v calls=%d", r, calls)
	}
	var state entity.NodeConfigState
	repository.DB.First(&state, "node_id = ?", n.ID)
	saved, _ := NewRoutingService().Binding("node", n.ID)
	if saved.Overrides != b.Overrides || saved.Mode != b.Mode {
		t.Fatal("automatic recovery rewrote desired binding")
	}
	desired, _, _ := svc.buildConfig(n)
	if state.DesiredHash != xray.ConfigHash(desired) {
		t.Fatal("automatic recovery rewrote desired hash")
	}
	if state.RuntimeHash == "" || !state.RecoveryPending {
		t.Fatal("successful recovery lost receipt or target retry")
	}
}
func TestPendingRevisionSurvivesConcurrentEdit(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewSyncService()
	saved := pushConfig
	t.Cleanup(func() { pushConfig = saved })
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		b, _ := NewRoutingService().Binding("node", n.ID)
		b.Overrides = `{"ipv4_force_domains":["domain:new.example"]}`
		if err := NewRoutingService().SaveBinding(b, b.Version, "other-admin", true); err != nil {
			t.Fatal(err)
		}
		return &xray.SyncResult{Success: true}
	}
	if r := s.SyncNode(n.ID, "test"); r.Success || !strings.Contains(r.Error, "期望配置已更新") {
		t.Fatalf("stale task succeeded: %+v", r)
	}
	var state entity.NodeConfigState
	repository.DB.First(&state, "node_id = ?", n.ID)
	current, _ := repository.NewNodeRepository().FindByID(n.ID)
	if !state.RecoveryPending || current.SyncStatus != entity.SyncStatusDrifted {
		t.Fatal("concurrent change was marked synced")
	}
	var policy xray.RoutingPolicy
	if err := json.Unmarshal([]byte(state.LastHealthyPolicy), &policy); err != nil {
		t.Fatal(err)
	}
	if len(policy.IPv4Domains) != 0 {
		t.Fatal("stored newer policy as previously healthy")
	}
}
func TestSubscriptionPolicyInheritanceAndLimits(t *testing.T) {
	n, u := routingFixture(t)
	s := NewRoutingService()
	b, _ := s.Binding("node", n.ID)
	b.UDP443Policy = "allow"
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	if s.UDP443Policy(n.ID, u.ID) != "allow" {
		t.Fatal("node policy not inherited")
	}
	user := entity.RoutingBinding{Scope: "user", TargetID: u.ID, Mode: "inherit", UDP443Policy: "block"}
	if err := s.SaveBinding(user, 0, "test", false); err != nil {
		t.Fatal(err)
	}
	if s.UDP443Policy(n.ID, u.ID) != "block" {
		t.Fatal("user override ignored")
	}
	sub := NewSubscribeService()
	if raw, err := sub.GenerateClash(u.Token); err != nil || !strings.Contains(raw, "xtls-rprx-vision") {
		t.Fatalf("block missing from Clash: %s %v", raw, err)
	}
	if raw, err := sub.GenerateSingbox(u.Token); err != nil || !strings.Contains(raw, "xtls-rprx-vision") {
		t.Fatalf("block missing from singbox: %s %v", raw, err)
	}
	user, _ = s.Binding("user", u.ID)
	user.UDP443Policy = "allow"
	if err := s.SaveBinding(user, user.Version, "test", false); err != nil {
		t.Fatal(err)
	}
	if _, err := sub.GenerateClash(u.Token); err == nil {
		t.Fatal("unverified allow accepted")
	}
	if _, err := sub.GenerateSingbox(u.Token); err == nil {
		t.Fatal("unverified allow accepted")
	}
}
