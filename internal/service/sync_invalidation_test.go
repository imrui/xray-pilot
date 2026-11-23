package service

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

func TestNodeUpdateMetadataDoesNotMarkDrifted(t *testing.T) {
	nodes, _ := groupSyncFixture(t)
	before := captureGroupSync(t, nodes, true)

	nodeSvc := NewNodeService()
	newName := "los1-renamed"
	newRegion := "US-West"
	newOwner := "ops"
	newRemark := "just metadata"
	sshPort := 2222
	if _, err := nodeSvc.Update(nodes[0].ID, &dto.UpdateNodeRequest{
		Name:    &newName,
		Region:  &newRegion,
		Owner:   &newOwner,
		Remark:  &newRemark,
		SSHPort: &sshPort,
	}, "admin:test"); err != nil {
		t.Fatalf("update node metadata: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before)

	// 修改会影响 Xray 配置的 LogLevel 应仅标记当前节点漂移
	debugLevel := "debug"
	if _, err := nodeSvc.Update(nodes[0].ID, &dto.UpdateNodeRequest{
		LogLevel: &debugLevel,
	}, "admin:test"); err != nil {
		t.Fatalf("update node log level: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[0].ID)
}

func TestUserLifecycleInvalidatesOnlyAffectedNodes(t *testing.T) {
	nodes, _ := groupSyncFixture(t)
	groupSvc := NewGroupService()
	userSvc := NewUserService()

	// 显式关闭 live-apply，验证回退全量同步路径不会误标无关节点
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatalf("disable live-apply: %v", err)
	}
	if userSvc.liveSync.Enabled() {
		t.Fatal("expected live-apply disabled")
	}

	before := captureGroupSync(t, nodes, true)

	// 1. 新建分组仅包含 nodes[1]（los2），不应标记任何节点漂移
	createdGroup, err := groupSvc.Create(&dto.CreateGroupRequest{
		Name:    "los2-only",
		NodeIDs: []uint{nodes[1].ID},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before)

	// 2. 新建用户仅加入该分组，只应标记 nodes[1] 漂移，nodes[0] 与 nodes[2] 保持 synced
	createdUser, err := userSvc.Create(&dto.CreateUserRequest{
		Username: "los2-relay-user",
		GroupIDs: []uint{createdGroup.ID},
	}, "https://panel.example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)

	// 3. 禁用用户只应影响 nodes[1]
	before = captureGroupSync(t, nodes, true)
	if err := userSvc.ToggleActive(createdUser.ID); err != nil {
		t.Fatalf("toggle user inactive: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)

	// 4. 重新启用用户只应影响 nodes[1]
	before = captureGroupSync(t, nodes, true)
	if err := userSvc.ToggleActive(createdUser.ID); err != nil {
		t.Fatalf("toggle user active: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)

	// 5. 重置 UUID 只应影响 nodes[1]
	before = captureGroupSync(t, nodes, true)
	if _, err := userSvc.ResetUUID(createdUser.ID, "https://panel.example.com"); err != nil {
		t.Fatalf("reset uuid: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)

	// 6. 仅修改备注不应标记任何节点漂移
	before = captureGroupSync(t, nodes, true)
	remark := "only remark changed"
	if _, err := userSvc.Update(createdUser.ID, &dto.UpdateUserRequest{
		Remark: &remark,
	}, "https://panel.example.com"); err != nil {
		t.Fatalf("update user remark: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before)

	// 7. 删除用户只应影响 nodes[1]
	before = captureGroupSync(t, nodes, true)
	if err := userSvc.Delete(createdUser.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)

	// 确认其他节点状态始终为 synced
	for _, idx := range []int{0, 2} {
		fresh, err := NewNodeService().GetByID(nodes[idx].ID)
		if err != nil {
			t.Fatalf("get node %d: %v", nodes[idx].ID, err)
		}
		if fresh.SyncStatus != string(entity.SyncStatusSynced) {
			t.Fatalf("unaffected node %d sync status = %q, want synced", nodes[idx].ID, fresh.SyncStatus)
		}
	}
}

// 在读取快照后、写入前模拟另一个已提交的修改。使用当前连接也能在单连接 SQLite
// 上确定性地触发旧 Save 恢复列/关联的问题，不依赖 goroutine 调度或 sleep。
func afterReadOnce(t *testing.T, table string, inject func(*gorm.DB) error) {
	t.Helper()
	fired := false
	callback := repository.DB.Callback().Query()
	if err := callback.After("gorm:after_query").Register("test:interleaved_write", func(q *gorm.DB) {
		if fired || q.Statement.Table != table {
			return
		}
		fired = true
		q.AddError(inject(q.Session(&gorm.Session{NewDB: true})))
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = callback.Remove("test:interleaved_write")
		if !fired {
			t.Error("expected interleaved write")
		}
	})
}

func TestNodeRemarkPreservesConcurrentDrift(t *testing.T) {
	n, _ := routingFixture(t)
	nodes := []entity.Node{*n}
	before := captureGroupSync(t, nodes, true)[n.ID]
	afterReadOnce(t, "nodes", func(tx *gorm.DB) error {
		return repository.InvalidateConfigs(tx, []uint{n.ID})
	})
	remark := "changed while queued"
	got, err := NewNodeService().Update(n.ID, &dto.UpdateNodeRequest{Remark: &remark}, "test")
	if err != nil {
		t.Fatal(err)
	}
	s := captureGroupSync(t, nodes, false)[n.ID]
	if got.Remark != remark || got.SyncStatus != string(entity.SyncStatusDrifted) || s.status != entity.SyncStatusDrifted || !s.state.RecoveryPending || s.state.ChangeRevision != before.state.ChangeRevision+1 {
		t.Fatalf("metadata overwrote pending state: %+v", s.state)
	}
}

func TestUserRemarkPreservesNewGroupAndCredentials(t *testing.T) {
	n, u := routingFixture(t)
	g := entity.Group{Name: "moved", Active: true}
	if err := repository.DB.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	const newUUID = "11111111-2222-4333-8444-555555555555"
	afterReadOnce(t, "users", func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE user_groups SET group_id = ? WHERE user_id = ?", g.ID, u.ID).Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE users SET uuid = ?, active = ? WHERE id = ?", newUUID, false, u.ID).Error; err != nil {
			return err
		}
		return repository.InvalidateConfigs(tx, []uint{n.ID})
	})
	remark := "metadata only"
	got, err := NewUserService().Update(u.ID, &dto.UpdateUserRequest{Remark: &remark}, "")
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := repository.NewUserRepository().FindByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.GroupIDs, []uint{g.ID}) || fresh.UUID != newUUID || fresh.Active || fresh.Remark != remark {
		t.Fatalf("remark restored stale authorization: %+v", got)
	}
	raw, _, err := NewSyncService().buildConfig(n)
	if err != nil || strings.Contains(raw, u.UUID) || strings.Contains(raw, newUUID) {
		t.Fatalf("old membership restored: %v", err)
	}
}

func TestEffectiveUserMembershipInvalidation(t *testing.T) {
	nodes, first := groupSyncFixture(t)
	groups, users := NewGroupService(), NewUserService()
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	second, err := groups.Create(&dto.CreateGroupRequest{Name: "second", NodeIDs: []uint{nodes[1].ID, nodes[2].ID}})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := groups.Create(&dto.CreateGroupRequest{Name: "disabled", NodeIDs: []uint{nodes[2].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = groups.ToggleActive(disabled.ID); err != nil {
		t.Fatal(err)
	}
	before := captureGroupSync(t, nodes, true)
	u, err := users.Create(&dto.CreateUserRequest{Username: "move", GroupIDs: []uint{first, disabled.ID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[0].ID, nodes[1].ID)
	before = captureGroupSync(t, nodes, true)
	ids, _ := json.Marshal([]uint{second.ID, disabled.ID})
	if _, err = users.Update(u.ID, &dto.UpdateUserRequest{GroupIDs: ids}, ""); err != nil {
		t.Fatal(err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[0].ID, nodes[2].ID)
	for _, live := range []bool{false, true} {
		if err = repository.NewSettingRepository().Set(KeyXrayLiveApply, fmt.Sprint(live)); err != nil {
			t.Fatal(err)
		}
		before = captureGroupSync(t, nodes, true)
		name := fmt.Sprintf("rename-%t", live)
		if _, err = users.Update(u.ID, &dto.UpdateUserRequest{Username: &name}, ""); err != nil {
			t.Fatal(err)
		}
		assertGroupSyncImpact(t, nodes, before, nodes[1].ID, nodes[2].ID)
	}
}

func TestIneffectiveUserEditsAndBornExpired(t *testing.T) {
	nodes, group := groupSyncFixture(t)
	svc := NewUserService()
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(3 * time.Hour)
	u, err := svc.Create(&dto.CreateUserRequest{Username: "future", GroupIDs: []uint{group}, ExpiresAt: &future}, "")
	if err != nil {
		t.Fatal(err)
	}
	before := captureGroupSync(t, nodes, true)
	renew, _ := json.Marshal(future.Add(time.Hour))
	if _, err = svc.Update(u.ID, &dto.UpdateUserRequest{ExpiresAt: renew}, ""); err != nil {
		t.Fatal(err)
	}
	assertGroupSyncImpact(t, nodes, before)
	if err = svc.ToggleActive(u.ID); err != nil {
		t.Fatal(err)
	}
	before = captureGroupSync(t, nodes, true)
	if _, err = svc.ResetUUID(u.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err = svc.Delete(u.ID); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	_, err = svc.Create(&dto.CreateUserRequest{Username: "born-expired", GroupIDs: []uint{group}, ExpiresAt: &past}, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.SweepExpired(time.Now())
	if err != nil || len(res.Swept) != 0 {
		t.Fatalf("born-expired scheduled: %+v %v", res, err)
	}
	assertGroupSyncImpact(t, nodes, before)
}

func TestExpiryRetainsOldNodeAcrossMembershipChange(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(fmt.Sprintf("live=%t", live), func(t *testing.T) {
			n, u := routingFixture(t)
			if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, fmt.Sprint(live)); err != nil {
				t.Fatal(err)
			}
			nodes := []entity.Node{*n}
			start := captureGroupSync(t, nodes, true)[n.ID].state.ChangeRevision
			// 使用与本地时区不同的偏移，覆盖 SQLite 日期比较及配置渲染。
			past := time.Now().Add(-time.Hour).In(time.FixedZone("UTC+14", 14*3600))
			if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", past.Format(time.RFC3339Nano)).Error; err != nil {
				t.Fatal(err)
			}
			_, release, err := repository.AcquireConfigLease(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			save := pushConfig
			t.Cleanup(func() { pushConfig = save })
			calls := 0
			pushConfig = func(_ xray.SSHParams, raw string) *xray.SyncResult {
				calls++
				if strings.Contains(raw, u.UUID) {
					t.Error("expired credential rendered")
				}
				return &xray.SyncResult{Success: true, ProcessID: "test"}
			}
			// 一组在第一次扫描前移除，另一组在扫描后移除，均需保留旧节点。
			removeGroup := func() {
				t.Helper()
				g, err := repository.NewGroupRepository().FindByID(u.Groups[0].ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = repository.NewGroupRepository().ReplaceNodes(g, nil); err != nil {
					t.Fatal(err)
				}
			}
			if !live {
				removeGroup()
			}
			for i := 0; i < 2; i++ {
				r, err := NewUserService().SweepExpired(time.Now())
				if err != nil || len(r.Swept) != 0 {
					t.Fatalf("unconfirmed revocation reported: %+v %v", r, err)
				}
				fresh, err := repository.NewUserRepository().FindByID(u.ID)
				if err != nil || fresh.ExpiredSweptAt != nil {
					t.Fatalf("premature stamp: %v", err)
				}
				state := captureGroupSync(t, nodes, false)[n.ID].state
				if state.ChangeRevision != start+1 || !state.RecoveryPending {
					t.Fatalf("retry changed revision: %+v", state)
				}
				if i == 0 && live {
					removeGroup()
					start++
				} // 分组移除本身另有一项配置变更。
			}
			if calls != 0 {
				t.Fatal("busy node reached remote")
			}
			release()
			// 用完整同步回执确认，live 模式的运行时查询已由持有租约的两次扫描覆盖。
			if err = repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
				t.Fatal(err)
			}
			r, err := NewUserService().SweepExpired(time.Now())
			if err != nil || !slices.Equal(r.Swept, []string{u.Username}) || calls != 1 {
				t.Fatalf("persisted target was not confirmed: %+v calls=%d err=%v", r, calls, err)
			}
			r, err = NewUserService().SweepExpired(time.Now())
			if err != nil || len(r.Swept) > 0 || calls != 1 {
				t.Fatal("completed target ran twice")
			}
		})
	}
}

func TestNodeConnectionEditsInvalidateOnlyTheirReceipt(t *testing.T) {
	n, _ := routingFixture(t)
	before := captureGroupSync(t, []entity.Node{*n}, true)[n.ID].state.ChangeRevision
	now := time.Now()
	q := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID)
	if err := q.Updates(map[string]any{"verified_at": now, "process_id": "old-process", "runtime_authorization_hash": "old-auth"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewNodeService()
	ip := "192.0.2.44"
	_, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(n.ID, &dto.UpdateNodeRequest{IP: &ip}, "test")
	release()
	if !errors.Is(err, repository.ErrConfigLeased) {
		t.Fatalf("changed host under lease: %v", err)
	}
	if _, err = svc.Update(n.ID, &dto.UpdateNodeRequest{IP: &ip}, "test"); err != nil {
		t.Fatal(err)
	}
	state := captureGroupSync(t, []entity.Node{*n}, false)[n.ID].state
	if state.ChangeRevision != before+1 || !state.RecoveryPending || state.VerifiedAt != nil || state.RuntimeHash != "" || state.RuntimeAuthorizationHash != "" || state.ProcessID != "" || state.DiskHash != "disk" {
		t.Fatalf("old host receipt survived: %+v", state)
	}
	if err = q.Updates(map[string]any{"last_error": "SSH 连接失败", "recovery_attempts": 3, "next_retry_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	port := 2223
	if _, err = svc.Update(n.ID, &dto.UpdateNodeRequest{SSHPort: &port}, "test"); err != nil {
		t.Fatal(err)
	}
	state = captureGroupSync(t, []entity.Node{*n}, false)[n.ID].state
	if state.ChangeRevision != before+1 || state.RecoveryAttempts != 0 || state.NextRetryAt == nil {
		t.Fatalf("SSH recovery not rearmed: %+v", state)
	}
}

func TestRealityLegacyKeyHiddenFromPreviewAndDiagnostics(t *testing.T) {
	const raw = `{"streamSettings":{"realitySettings":{"PUBLICKEY":"reality-secret"}},"peers":[{"publicKey":"wireguard-public"}]}`
	masked := redactConfig(raw)
	if strings.Contains(masked, "reality-secret") || !strings.Contains(masked, "wireguard-public") {
		t.Fatal("incorrect preview redaction")
	}
	if got := scrubConfigSecretsFromMessage("rejected reality-secret", raw); strings.Contains(got, "reality-secret") {
		t.Fatal("kernel message exposed REALITY key")
	}
}

func TestExpiryBeforeAndAfterDepartureReceipts(t *testing.T) {
	for _, mode := range []string{"user transfer", "group transfer", "confirmed transfer"} {
		t.Run(mode, func(t *testing.T) {
			n, u := routingFixture(t)
			now := time.Now()
			future := now.Add(time.Hour)
			if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
				t.Fatal(err)
			}
			if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", future).Error; err != nil {
				t.Fatal(err)
			}
			save := pushConfig
			t.Cleanup(func() { pushConfig = save })
			calls := 0
			pushConfig = func(_ xray.SSHParams, raw string) *xray.SyncResult {
				calls++
				if strings.Contains(raw, u.UUID) {
					t.Error("old user still present")
				}
				return &xray.SyncResult{Success: true, ProcessID: "1"}
			}
			if mode == "group transfer" {
				if err := NewGroupService().ToggleActive(u.Groups[0].ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := NewUserService().Update(u.ID, &dto.UpdateUserRequest{GroupIDs: json.RawMessage("[]")}, ""); err != nil {
					t.Fatal(err)
				}
			}
			r, err := NewUserService().SweepExpired(now)
			if err != nil || len(r.Swept) > 0 || calls != 0 {
				t.Fatalf("premature expiration: %+v %v", r, err)
			}
			if mode == "confirmed transfer" {
				if r := NewSyncService().SyncNode(n.ID, "admin:test"); !r.Success {
					t.Fatal(r.Error)
				}
			}
			if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			_, release, err := repository.AcquireConfigLease(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			r, err = NewUserService().SweepExpired(now)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "confirmed transfer" {
				if !slices.Equal(r.Swept, []string{u.Username}) || calls != 1 {
					t.Fatalf("previous receipt ignored: %+v calls=%d", r, calls)
				}
				return
			}
			if len(r.Swept) > 0 || calls != 0 {
				t.Fatalf("old grant forgotten: %+v", r)
			}
			release()
			r, err = NewUserService().SweepExpired(now)
			if err != nil || !slices.Equal(r.Swept, []string{u.Username}) || calls != 1 {
				t.Fatalf("old grant not confirmed: %+v calls=%d err=%v", r, calls, err)
			}
		})
	}
}

func TestRenewalRejectsPriorRevocationReceipt(t *testing.T) {
	n, u := routingFixture(t)
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	past := now.Add(-time.Hour)
	if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	targets, _, err := repository.PrepareExpirySweep(now)
	if err != nil || len(targets) != 1 {
		t.Fatalf("expiry setup: %+v %v", targets, err)
	}
	old := targets[0].Ticket
	if err = repository.NewNodeRepository().Delete(n.ID); err == nil {
		t.Fatal("removed only connection before revocation")
	}
	future, _ := json.Marshal(now.Add(time.Hour))
	if _, err = NewUserService().Update(u.ID, &dto.UpdateUserRequest{ExpiresAt: future}, ""); err != nil {
		t.Fatal(err)
	}
	if targets, _, err = repository.PrepareExpirySweep(now); err != nil || len(targets) != 0 {
		t.Fatalf("renewal retained obsolete target: %v %v", targets, err)
	}
	until, _ := json.Marshal(past)
	if _, err = NewUserService().Update(u.ID, &dto.UpdateUserRequest{ExpiresAt: until}, ""); err != nil {
		t.Fatal(err)
	}
	if targets, _, err = repository.PrepareExpirySweep(now); err != nil || len(targets) != 1 || targets[0].Ticket == old {
		t.Fatalf("missing new expiry generation: %v %v", targets, err)
	}
	names, err := repository.ConfirmExpiryTargets([]string{old}, now)
	if err != nil || len(names) > 0 {
		t.Fatalf("old receipt marked renewed user: %v %v", names, err)
	}
	names, err = repository.ConfirmExpiryTargets([]string{targets[0].Ticket}, now)
	if err != nil || !slices.Equal(names, []string{u.Username}) {
		t.Fatalf("current receipt not accepted: %v %v", names, err)
	}
}

func TestSharedNodeConfirmsMultipleExpiriesOnce(t *testing.T) {
	n, u := routingFixture(t)
	now := time.Now()
	past := now.Add(-time.Hour)
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	v := entity.User{Username: "second-expired", UUID: "22222222-2222-4222-8222-222222222222", Token: "second-expired", Groups: u.Groups, Active: true, ExpiresAt: &past}
	if err := repository.DB.Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	start := captureGroupSync(t, []entity.Node{*n}, true)[n.ID].state.ChangeRevision
	save := pushConfig
	t.Cleanup(func() { pushConfig = save })
	calls := 0
	pushConfig = func(_ xray.SSHParams, raw string) *xray.SyncResult {
		calls++
		if strings.Contains(raw, u.UUID) || strings.Contains(raw, v.UUID) {
			t.Error("expired user rendered")
		}
		return &xray.SyncResult{Success: true, ProcessID: "1"}
	}
	result, err := NewUserService().SweepExpired(now)
	state := captureGroupSync(t, []entity.Node{*n}, false)[n.ID].state
	if err != nil || !slices.Equal(result.Swept, []string{u.Username, v.Username}) || calls != 1 || state.ChangeRevision != start+1 || state.RecoveryPending {
		t.Fatalf("shared target: %+v calls=%d state=%+v err=%v", result, calls, state, err)
	}
}

func TestGroupRegrantCancelsOldExpiryStop(t *testing.T) {
	n, u := routingFixture(t)
	s := NewUserService()
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	old, _, err := repository.PrepareExpirySweep(now)
	if err != nil || len(old) != 1 {
		t.Fatalf("missing old expiry: %v %v", old, err)
	}
	groups := NewGroupService()
	if err = groups.ToggleActive(u.Groups[0].ID); err != nil {
		t.Fatal(err)
	}
	renew, _ := json.Marshal(now.Add(time.Hour))
	if _, err = s.Update(u.ID, &dto.UpdateUserRequest{ExpiresAt: renew}, ""); err != nil {
		t.Fatal(err)
	}
	remaining, _, err := repository.PrepareExpirySweep(now)
	if err != nil || len(remaining) != 1 {
		t.Fatalf("renewal discarded old node too early: %v %v", remaining, err)
	}
	if err = groups.ToggleActive(u.Groups[0].ID); err != nil {
		t.Fatal(err)
	}
	remaining, names, err := repository.PrepareExpirySweep(now)
	if err != nil || len(remaining) != 0 || len(names) != 0 {
		t.Fatalf("regranted node %d still scheduled: %v %v", n.ID, remaining, err)
	}
}

func TestUserChangeSkipsNodesWithoutSubscriptionInbounds(t *testing.T) {
	nodes, group := groupSyncFixture(t)
	before := captureGroupSync(t, nodes, true)
	if err := repository.DB.Where("node_id = ?", nodes[1].ID).Delete(&entity.NodeProfileKey{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewUserService().Create(&dto.CreateUserRequest{Username: "subset", GroupIDs: []uint{group}}, ""); err != nil {
		t.Fatal(err)
	}
	for i, n := range nodes {
		var state entity.NodeConfigState
		if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
			t.Fatal(err)
		}
		want := before[n.ID].state.ChangeRevision
		if i == 0 {
			want++
		}
		if state.ChangeRevision != want || state.RecoveryPending != (i == 0) {
			t.Fatalf("unexpected subscription target %d: %+v", n.ID, state)
		}
	}
}

func TestSyncUsesAddressReadUnderLease(t *testing.T) {
	n, _ := routingFixture(t)
	const ip = "192.0.2.17"
	afterReadOnce(t, "nodes", func(tx *gorm.DB) error {
		return tx.Exec("UPDATE nodes SET ip = ? WHERE id = ?", ip, n.ID).Error
	})
	save := pushConfig
	t.Cleanup(func() { pushConfig = save })
	pushConfig = func(p xray.SSHParams, _ string) *xray.SyncResult {
		if p.Host != ip {
			t.Fatalf("sending to stale host %q", p.Host)
		}
		return &xray.SyncResult{Success: true, ProcessID: "test"}
	}
	if r := NewSyncService().SyncNode(n.ID, "test"); !r.Success {
		t.Fatal(r.Error)
	}
}

func TestExpiryAwaitsPendingRemovalOfLastUserInbound(t *testing.T) {
	n, u := routingFixture(t)
	now := time.Now()
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", n.ID).Delete(&entity.NodeProfileKey{}).Error; err != nil {
			return err
		}
		return repository.InvalidateConfigs(tx, []uint{n.ID})
	}); err != nil {
		t.Fatal(err)
	}
	lease, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	until, _ := json.Marshal(now.Add(-time.Minute))
	if _, err = NewUserService().Update(u.ID, &dto.UpdateUserRequest{ExpiresAt: until}, ""); err != nil {
		t.Fatal(err)
	}
	r, err := NewUserService().SweepExpired(now)
	if err != nil || len(r.Swept) != 0 {
		t.Fatalf("unconfirmed old inbound was considered revoked: %+v %v", r, err)
	}
	var state entity.NodeConfigState
	if err = repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil || state.ChangeRevision != lease.ChangeRevision || !state.RecoveryPending {
		t.Fatalf("existing revision was displaced: %+v %v", state, err)
	}
}

func TestExpiryStopMustPersistReceiptUnderItsLease(t *testing.T) {
	n, u := routingFixture(t)
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "false"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := repository.DB.Model(&entity.User{}).Where("id = ?", u.ID).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	savedPush, savedStop := pushConfig, stopRevokedNode
	t.Cleanup(func() { pushConfig, stopRevokedNode = savedPush, savedStop })
	pushConfig = func(xray.SSHParams, string) *xray.SyncResult { return &xray.SyncResult{Error: "offline"} }
	calls := 0
	stopRevokedNode = func(xray.SSHParams) error {
		calls++
		return repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).
			UpdateColumns(map[string]any{"lease_owner": "another-worker", "lease_until": now.Add(time.Minute)}).Error
	}
	r, err := NewUserService().SweepExpired(now)
	if err != nil || len(r.Swept) != 0 || calls != 1 {
		t.Fatalf("unfenced stop confirmed: %+v calls=%d err=%v", r, calls, err)
	}
	targets, names, err := repository.PrepareExpirySweep(now)
	if err != nil || len(targets) != 1 || len(names) > 0 {
		t.Fatalf("lost unconfirmed revocation: %v %v %v", targets, names, err)
	}
}

func enableSyncAlerts(t *testing.T) {
	t.Helper()
	if err := NewSettingService().BatchUpdate(map[string]string{
		KeyAlertEnabled: "true", KeyFeishuEnabled: "true", KeyAlertFeishuChatID: "test", KeyAlertKinds: entity.AlertKindSyncFailed,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFailureAfterLeaseTakeover(t *testing.T) {
	for _, failure := range []string{"push failed", "ROLLED_BACK"} {
		t.Run(failure, func(t *testing.T) {
			n, _ := routingFixture(t)
			svc := NewSyncService()
			enableSyncAlerts(t)
			oldPush := pushConfig
			t.Cleanup(func() { pushConfig = oldPush })
			var receipt entity.NodeConfigState
			pushConfig = func(_ xray.SSHParams, raw string) *xray.SyncResult {
				if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
				// 第二个进程不共享 lockNode。让它先取得数据库租约并提交一份已确认的回执。
				current, release, err := repository.AcquireConfigLease(n.ID)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(release)
				if err := svc.recordApplied(current, raw, "new-worker", nil); err != nil {
					t.Fatal(err)
				}
				if err := finishNodeStatus(current, xray.ConfigHash(raw), true); err != nil {
					t.Fatal(err)
				}
				if err := repository.DB.First(&receipt, "node_id = ?", n.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := svc.alertSvc.repo.SaveObservation(&entity.NodeAlertState{NodeID: n.ID, Kind: entity.AlertKindSyncFailed, OKCount: 1}); err != nil {
					t.Fatal(err)
				}
				return &xray.SyncResult{Error: failure, ObservedHash: "old-disk", RecoveredHash: "old-recovery"}
			}
			const actor = "admin:old-worker"
			r := svc.SyncNode(n.ID, actor)
			if r.Success || !strings.Contains(r.Error, ErrConfigLeaseLost.Error()) || strings.Contains(r.Error, failure) {
				t.Fatalf("expected unconfirmed stale failure, got %+v", r)
			}
			var state entity.NodeConfigState
			if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
				t.Fatal(err)
			}
			if receipt.RuntimeHash == "" || receipt.RuntimeAuthorizationHash == "" || receipt.VerifiedAt == nil || !reflect.DeepEqual(state, receipt) {
				t.Fatalf("stale failure changed the confirmed receipt: before=%+v after=%+v", receipt, state)
			}
			node, err := svc.nodeRepo.FindByID(n.ID)
			if err != nil || node.SyncStatus != entity.SyncStatusSynced || node.ConfigHash != receipt.RuntimeHash {
				t.Fatalf("stale failure changed node: %+v %v", node, err)
			}
			alert, err := svc.alertSvc.repo.Get(n.ID, entity.AlertKindSyncFailed)
			if err != nil || alert.Alerting || alert.OKCount != 1 || alert.FailCount != 0 {
				t.Fatalf("stale failure changed alert: %+v %v", alert, err)
			}
			var audit entity.SyncLog
			if err := repository.DB.First(&audit, "actor = ? AND action = ?", actor, "sync").Error; err != nil || audit.Success ||
				!strings.Contains(audit.Message, failure) || !strings.Contains(audit.Message, ErrConfigLeaseLost.Error()) {
				t.Fatalf("missing stale failure audit: %+v %v", audit, err)
			}
		})
	}
}

func TestRemoteRollbackWithoutReadbackClearsVerifiedRuntime(t *testing.T) {
	n, _ := routingFixture(t)
	svc := NewSyncService()
	savedPush := pushConfig
	t.Cleanup(func() { pushConfig = savedPush })
	pushConfig = func(xray.SSHParams, string) *xray.SyncResult {
		return &xray.SyncResult{Success: true, ProcessID: "original"}
	}
	if r := svc.SyncNode(n.ID, "admin:test"); !r.Success {
		t.Fatal(r.Error)
	}
	var before entity.NodeConfigState
	if err := repository.DB.First(&before, "node_id = ?", n.ID).Error; err != nil || before.VerifiedAt == nil || before.RuntimeHash == "" || before.RuntimeAuthorizationHash == "" {
		t.Fatalf("missing initial receipt: %+v %v", before, err)
	}
	pushConfig = func(xray.SSHParams, string) *xray.SyncResult {
		// persisted 阶段未落库，脚本报告回滚后也没有读回远端文件。
		return &xray.SyncResult{Error: "ROLLED_BACK"}
	}
	r := svc.SyncNode(n.ID, "admin:test")
	if r.Success || !strings.Contains(r.Error, "ROLLED_BACK") {
		t.Fatalf("missing remote failure: %+v", r)
	}
	var after entity.NodeConfigState
	if err := repository.DB.First(&after, "node_id = ?", n.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.RuntimeHash != "" || after.RuntimeAuthorizationHash != "" || after.VerifiedAt != nil || !after.RecoveryPending || after.RecoveryAttempts != 1 {
		t.Fatalf("unverified rollback kept runtime proof or lost retry: %+v", after)
	}
}

func TestDeploymentPhaseRequiresCurrentLease(t *testing.T) {
	n, _ := routingFixture(t)
	db := repository.DB
	old, releaseOld, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOld()
	if err := rememberDesired(old, "same-hash"); err != nil {
		t.Fatal(err)
	}
	job := entity.ConfigDeployment{NodeID: n.ID, RequestKey: "first", DesiredHash: old.DesiredHash, Status: "applying"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	checkPhase := func(id uint, status string, revision uint) {
		t.Helper()
		var got entity.ConfigDeployment
		if err := db.First(&got, id).Error; err != nil || got.Status != status || got.DesiredRevision != revision {
			t.Fatalf("job %d: want %s/%d, got %+v %v", id, status, revision, got, err)
		}
	}
	recordOld := deploymentPhaseRecorder(old)
	recordOld("staged")
	checkPhase(job.ID, "staged", old.DesiredRevision)
	if err := db.Model(old).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	recordOld("verifying")
	checkPhase(job.ID, "staged", old.DesiredRevision)
	if err := db.Model(&job).Update("status", "unknown").Error; err != nil {
		t.Fatal(err)
	}
	current, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	retry := entity.ConfigDeployment{NodeID: n.ID, RequestKey: "retry", DesiredHash: current.DesiredHash, Status: "applying"}
	if err := db.Create(&retry).Error; err != nil {
		t.Fatal(err)
	}
	recordNew := deploymentPhaseRecorder(current)
	recordNew("persisted")
	checkPhase(retry.ID, "persisted", current.DesiredRevision)
	var persisted entity.NodeConfigState
	if err := db.First(&persisted, "node_id = ?", n.ID).Error; err != nil || persisted.DiskHash != current.DesiredHash || persisted.DiskRevision != current.DesiredRevision {
		t.Fatalf("missing current persisted receipt: %+v %v", persisted, err)
	}
	recordNew("verifying")
	if err := leaseUpdate(db, current, map[string]any{"runtime_hash": "new-proof", "verified_at": time.Now()}); err != nil {
		t.Fatal(err)
	}
	recordOld("rollback_running")
	checkPhase(retry.ID, "verifying", current.DesiredRevision)
	recordOld("persisted")
	checkPhase(retry.ID, "verifying", current.DesiredRevision)
	checkPhase(job.ID, "unknown", old.DesiredRevision)
	var after entity.NodeConfigState
	if err := db.First(&after, "node_id = ?", n.ID).Error; err != nil || after.RuntimeHash != "new-proof" || after.VerifiedAt == nil {
		t.Fatalf("stale persisted callback changed the new receipt: %+v %v", after, err)
	}
}

func TestFailureReceiptRequiresUnexpiredLease(t *testing.T) {
	n, _ := routingFixture(t)
	state, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := repository.DB.Model(state).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := recordConfigFailure(state, "push failed", nil); !errors.Is(err, ErrConfigLeaseLost) {
		t.Fatalf("expired owner recorded failure: %v", err)
	}
	fresh, err := repository.NewNodeRepository().FindByID(n.ID)
	if err != nil || fresh.SyncStatus != entity.SyncStatusSynced {
		t.Fatalf("expired owner changed node: %+v %v", fresh, err)
	}
	var after entity.NodeConfigState
	if err := repository.DB.First(&after, "node_id = ?", n.ID).Error; err != nil || after.RecoveryAttempts != 0 || after.LastError != "" {
		t.Fatalf("expired owner changed recovery: %+v %v", after, err)
	}
}

func TestSyncFailureAlertAfterLeaseTakeover(t *testing.T) {
	n, _ := routingFixture(t)
	enableSyncAlerts(t)
	svc := NewSyncService()
	old, release, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := recordConfigFailure(old, "push failed", nil); err != nil {
		t.Fatal(err)
	}
	// 失败回执已经提交，告警观测前另一个进程提交了成功观测。
	release()
	current, releaseNew, err := repository.AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseNew()
	if err := svc.alertSvc.repo.SaveObservation(&entity.NodeAlertState{NodeID: n.ID, Kind: entity.AlertKindSyncFailed, OKCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := svc.alertSvc.observeSync(current, svc.alertSvc.loadAlertConfig(), true, ""); err != nil {
		t.Fatal(err)
	}
	svc.reportSyncAlert(old, false, "stale failure")
	alert, err := svc.alertSvc.repo.Get(n.ID, entity.AlertKindSyncFailed)
	if err != nil || alert.Alerting || alert.FailCount != 0 || alert.OKCount != 2 {
		t.Fatalf("late observation changed newer alert: %+v %v", alert, err)
	}
}

// 回环节点可在握手前断开，或完成握手后拒绝读取配置，区分离线与热更新失败。
func expirySSHConnections(t *testing.T, n *entity.Node, online bool) *atomic.Int32 {
	t.Helper()
	dir := t.TempDir()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(key, "expiry-test")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			count.Add(1)
			if online {
				server, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err == nil {
					go ssh.DiscardRequests(requests)
					for channel := range channels {
						_ = channel.Reject(ssh.Prohibited, "cannot read config")
					}
					_ = server.Close()
				}
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	if err := repository.DB.Model(n).Updates(map[string]any{"ip": "127.0.0.1", "ssh_port": listener.Addr().(*net.TCPAddr).Port, "ssh_key_path": keyPath}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.NewSettingRepository().Set(KeySSHKnownHostsPath, filepath.Join(dir, "known_hosts")); err != nil {
		t.Fatal(err)
	}
	return &count
}

func TestLiveExpiryAttemptsOncePerSweep(t *testing.T) {
	for _, mode := range []string{"offline", "lease-busy", "online-fallback"} {
		t.Run(mode, func(t *testing.T) {
			n, u := routingFixture(t)
			online := mode == "online-fallback"
			connections := expirySSHConnections(t, n, online)
			now := time.Now()
			if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "true"); err != nil {
				t.Fatal(err)
			}
			if err := repository.DB.Model(u).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "lease-busy" {
				_, release, err := repository.AcquireConfigLease(n.ID)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
			}
			leases := 0
			callback := repository.DB.Callback().Update()
			if err := callback.After("gorm:update").Register("test:count_expiry_leases", func(tx *gorm.DB) {
				values, _ := tx.Statement.Dest.(map[string]any)
				if owner, _ := values["lease_owner"].(string); tx.Statement.Table == "node_config_states" && owner != "" {
					leases++
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = callback.Remove("test:count_expiry_leases") })
			oldPush, oldStop := pushConfig, stopRevokedNode
			t.Cleanup(func() { pushConfig, stopRevokedNode = oldPush, oldStop })
			pushes, stops := 0, 0
			pushConfig = func(xray.SSHParams, string) *xray.SyncResult {
				pushes++
				return &xray.SyncResult{Error: "push failed"}
			}
			stopRevokedNode = func(xray.SSHParams) error { stops++; return errors.New("stop failed") }

			svc := NewUserService()
			var ticket string
			var revision uint
			for scan := 1; scan <= 2; scan++ {
				r, err := svc.SweepExpired(now)
				if err != nil || !r.LiveApply || len(r.Swept) != 0 {
					t.Fatalf("unconfirmed revoke reported: %+v %v", r, err)
				}
				var target entity.UserExpiryTarget
				if err := repository.DB.First(&target, "node_id = ? AND user_id = ?", n.ID, u.ID).Error; err != nil {
					t.Fatal(err)
				}
				var state entity.NodeConfigState
				if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
					t.Fatal(err)
				}
				fresh, err := repository.NewUserRepository().FindByID(u.ID)
				if err != nil || fresh.ExpiredSweptAt != nil || !fresh.ExpiryQueued || !state.RecoveryPending {
					t.Fatalf("unconfirmed revocation lost: %+v %+v %v", fresh, state, err)
				}
				if scan == 1 {
					ticket, revision = target.Ticket, state.ChangeRevision
				}
				if ticket == "" || target.Ticket != ticket || revision == 0 || state.ChangeRevision != revision {
					t.Fatalf("scan %d replaced outstanding ticket or revision", scan)
				}
				wantSSH, wantAttempts := scan, scan
				if mode == "lease-busy" {
					wantSSH, wantAttempts = 0, 0
				}
				if int(connections.Load()) != wantSSH || state.RecoveryAttempts != wantAttempts {
					t.Fatalf("scan %d: SSH=%d attempts=%d, want %d/%d", scan, connections.Load(), state.RecoveryAttempts, wantSSH, wantAttempts)
				}
				if online {
					if pushes != scan || stops != scan {
						t.Fatalf("scan %d: full sync=%d stop=%d", scan, pushes, stops)
					}
				} else if leases != scan || pushes != 0 || stops != 0 {
					t.Fatalf("scan %d: leases=%d full sync=%d stop=%d", scan, leases, pushes, stops)
				}
			}
			if online {
				pushConfig = func(xray.SSHParams, string) *xray.SyncResult {
					return &xray.SyncResult{Success: true, ProcessID: "confirmed"}
				}
				r, err := svc.SweepExpired(now)
				if err != nil || !slices.Equal(r.Swept, []string{u.Username}) || stops != 2 {
					t.Fatalf("full-sync receipt did not settle the ticket: %+v stops=%d %v", r, stops, err)
				}
			}
		})
	}
}
