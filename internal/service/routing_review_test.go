package service

import (
	"errors"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"strings"
	"testing"
	"time"
)

func TestRecoveryRechecksAfterWaitingForLiveApply(t *testing.T) {
	n, _ := routingFixture(t)
	svc := NewSyncService()
	saved := pushConfig
	defer func() { pushConfig = saved }()
	calls := 0
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult { calls++; return &xray.SyncResult{Success: true} }
	// 模拟热更新执行期间恢复队列已到期、且读取了旧快照。
	unlock := lockNode(n.ID)
	state, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Updates(map[string]any{"recovery_pending": true, "next_retry_at": time.Now().Add(-20 * time.Second)})
	started := make(chan struct{})
	done := make(chan *SyncResult, 1)
	go func() { close(started); done <- svc.SyncNode(n.ID, "system:config_recovery") }()
	<-started
	raw, _, err := svc.buildConfig(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := rememberDesired(state, xray.ConfigHash(raw)); err != nil {
		t.Fatal(err)
	}
	if err := svc.recordApplied(state, raw, "live", nil); err != nil {
		t.Fatal(err)
	}
	release()
	unlock()
	select {
	case r := <-done:
		if !r.Success || calls != 0 {
			t.Fatalf("stale recovery restarted node: %+v calls=%d", r, calls)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deadlocked")
	}
}

func TestLeaseBusyAndExpiredReceipt(t *testing.T) {
	n, _ := routingFixture(t)
	svc := NewSyncService()
	state, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := svc.SyncNode(n.ID, "test")
	if !r.Busy || r.Success {
		t.Fatalf("busy classified as failure: %+v", r)
	}
	repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("lease_until", time.Now().Add(-time.Second))
	if err := svc.recordApplied(state, `{"inbounds":[]}`, "old", nil); !errors.Is(err, ErrConfigLeaseLost) {
		t.Fatalf("expired receipt accepted: %v", err)
	}
	if err := finishNodeStatus(state, "stale", false); !errors.Is(err, ErrConfigLeaseLost) {
		t.Fatalf("expired node update accepted: %v", err)
	}
	current, _ := repository.NewNodeRepository().FindByID(n.ID)
	if current.ConfigHash == "stale" {
		t.Fatal("expired lease wrote hash")
	}
	release()
}

func TestBindingImpactAndActualOverrideValidation(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	b, _ := s.Binding("node", n.ID)
	b.Overrides = `{"custom_rules":[{"type":"field","port":"443","outboundTag":"direct-v6"}]}`
	if err := s.SaveBinding(b, b.Version, "test", true); err == nil || !strings.Contains(err.Error(), "0") {
		t.Fatalf("IPv6-disabled override accepted: %v", err)
	}
	global, _ := s.Binding("global", 0)
	b, _ = s.Binding("node", n.ID)
	b.Mode = "template"
	b.TemplateID = global.TemplateID
	b.Revision = global.Revision
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	var before entity.NodeConfigState
	repository.DB.First(&before, "node_id = ?", n.ID)
	rev := publishRouting(t, "changed", strings.Replace(DefaultRoutingPolicy, `"ipv4_force_domains":[]`, `"ipv4_force_domains":["domain:new.example"]`, 1))
	global.TemplateID = rev.TemplateID
	global.Revision = rev.Revision
	var impact BindingImpact
	if err := s.SaveBinding(global, global.Version, "test", true, &impact); err != nil {
		t.Fatal(err)
	}
	var after entity.NodeConfigState
	repository.DB.First(&after, "node_id = ?", n.ID)
	if after.ChangeRevision != before.ChangeRevision || len(impact.Changed) != 0 {
		t.Fatalf("unaffected node invalidated: %+v", impact)
	}
}

func TestExpiryRemainsPendingUntilConfirmedOrStopped(t *testing.T) {
	n, u := routingFixture(t)
	now := time.Now()
	expiry := now.Add(-time.Hour)
	repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", expiry)
	savedPush, savedStop := pushConfig, stopRevokedNode
	defer func() { pushConfig = savedPush; stopRevokedNode = savedStop }()
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult { return &xray.SyncResult{Error: "offline"} }
	stopRevokedNode = func(p xray.SSHParams) error { return errors.New("offline") }
	repository.EnsureConfigState(repository.DB, n.ID)
	repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("recovery_attempts", 3)
	svc := NewUserService()
	r, err := svc.SweepExpired(now)
	if err != nil || len(r.Swept) != 0 {
		t.Fatalf("offline revoke reported success: %+v %v", r, err)
	}
	var user entity.User
	repository.DB.First(&user, u.ID)
	if user.ExpiredSweptAt != nil {
		t.Fatal("offline expiry swept")
	}
	stopRevokedNode = func(p xray.SSHParams) error { return nil }
	r, err = svc.SweepExpired(now)
	if err != nil || len(r.Swept) != 1 {
		t.Fatalf("confirmed stop not recorded: %+v %v", r, err)
	}
	repository.DB.First(&user, u.ID)
	if user.ExpiredSweptAt == nil {
		t.Fatal("confirmed stop not swept")
	}
}

func TestRegisteredNodeDoesNotImplicitlyGrantBaseline(t *testing.T) {
	n, _ := routingFixture(t)
	now := time.Now()
	repository.DB.Model(n).Update("registered_at", now)
	saved := pushConfig
	defer func() { pushConfig = saved }()
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		if p.AllowInitial {
			t.Fatal("registration silently bypassed baseline")
		}
		return &xray.SyncResult{Error: "DRIFT_CONFLICT"}
	}
	r := NewSyncService().SyncNode(n.ID, "test")
	if r.Success {
		t.Fatal("manual configuration overwritten")
	}
}
func TestLostLeaseCannotCompleteSync(t *testing.T) {
	n, _ := routingFixture(t)
	saved := pushConfig
	defer func() { pushConfig = saved }()
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("lease_until", time.Now().Add(-time.Second))
		return &xray.SyncResult{Success: true}
	}
	r := NewSyncService().SyncNode(n.ID, "test")
	if r.Success {
		t.Fatal("lost lease reported success")
	}
	current, _ := repository.NewNodeRepository().FindByID(n.ID)
	if current.ConfigHash != "" {
		t.Fatal("lost lease overwrote config hash")
	}
}
