package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrConfigLeaseLost = errors.New("配置租约已失效，结果未确认")

func currentConfigLease(tx *gorm.DB, state *entity.NodeConfigState) *gorm.DB {
	return tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ? AND lease_until > ?", state.NodeID, state.LeaseOwner, time.Now())
}

func leaseUpdate(tx *gorm.DB, state *entity.NodeConfigState, updates map[string]any) error {
	r := currentConfigLease(tx, state).Updates(updates)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrConfigLeaseLost
	}
	return nil
}
func rememberDesired(state *entity.NodeConfigState, hash string) error {
	if state.DesiredHash != hash {
		state.DesiredRevision++
		state.DesiredHash = hash
	}
	return leaseUpdate(repository.DB, state, map[string]any{"desired_revision": state.DesiredRevision, "desired_hash": hash})
}
func (s *SyncService) stillCurrent(state *entity.NodeConfigState, hash string) bool {
	var latest entity.NodeConfigState
	if repository.DB.First(&latest, "node_id = ?", state.NodeID).Error != nil || latest.LeaseOwner != state.LeaseOwner || latest.ChangeRevision != state.ChangeRevision || latest.LeaseUntil == nil || !latest.LeaseUntil.After(time.Now()) {
		return false
	}
	n, err := s.nodeRepo.FindByID(state.NodeID)
	if err != nil {
		return false
	}
	content, _, err := s.buildConfig(n)
	return err == nil && xray.ConfigHash(content) == hash
}
func (s *SyncService) recordApplied(state *entity.NodeConfigState, content, processID string, healthy *EffectiveRouting) error {
	hash := xray.ConfigHash(content)
	now := time.Now()
	updates := map[string]any{"disk_revision": state.DesiredRevision, "disk_hash": hash,
		"runtime_revision": state.DesiredRevision, "runtime_hash": hash, "process_id": processID, "verified_at": now,
		"runtime_authorization_hash": xssh.AuthorizationHash(content),
		"last_error":                 "", "recovery_attempts": 0, "initial_baseline": false, "initial_baseline_hash": ""}
	verification := "service_only"
	var cfg struct {
		Inbounds []struct {
			Protocol string
			Settings struct{ Clients []any }
		}
	}
	if json.Unmarshal([]byte(content), &cfg) == nil {
		for _, in := range cfg.Inbounds {
			if (in.Protocol == "vless" || in.Protocol == "trojan") && len(in.Settings.Clients) > 0 {
				verification = "protocol"
			}
		}
	}
	if healthy == nil {
		verification = "grpc_receipt"
	}
	updates["verification"] = verification
	if healthy != nil {
		updates["last_healthy_revision"] = state.DesiredRevision
		effective := healthy
		raw := "legacy"
		var encryptedPolicy *xray.RoutingPolicy
		if effective.Policy != nil {
			// 历史库可能含旧版遗漏的简写凭据，健康快照同样必须执行静态加密。
			policy, err := xray.EncryptPolicySecrets(*effective.Policy, effective.Policy)
			if err != nil {
				return fmt.Errorf("保存健康策略凭据失败: %w", err)
			}
			encryptedPolicy = &policy
			data, err := json.Marshal(policy)
			if err != nil {
				return err
			}
			raw = string(data)
		}
		updates["last_healthy_policy"] = raw
		// 保留原覆盖图，合并策略已丢失 remap 来源，不能再从缺失出站猜测。
		overrides := effective.Binding.Overrides
		if overrides == "" {
			overrides = "{}"
		}
		encryptedOverrides, err := xray.EncryptOverridesSecrets(overrides, overrides, encryptedPolicy)
		if err != nil {
			return fmt.Errorf("保存健康节点覆盖凭据失败: %w", err)
		}
		updates["last_healthy_overrides"] = encryptedOverrides
		updates["last_healthy_template_id"] = effective.TemplateID
		updates["last_healthy_template_revision"] = effective.Revision
	}
	// A receipt belongs to its snapshot. Never clear work queued during the SSH call.
	return repository.AuthorizationTransaction(func(tx *gorm.DB) error {
		if err := leaseUpdate(tx, state, updates); err != nil {
			return err
		}
		current := tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND change_revision = ? AND lease_owner = ?", state.NodeID, state.ChangeRevision, state.LeaseOwner).
			Update("recovery_pending", false)
		if current.Error != nil || current.RowsAffected == 0 {
			return current.Error
		}
		// 当前配置已确认，到期前移组产生的旧授权随这张回执一并撤销。
		return tx.Where("node_id = ? AND deferred = ?", state.NodeID, true).Delete(&entity.UserExpiryTarget{}).Error
	})
}
func recordConfigFailure(state *entity.NodeConfigState, message string, updates map[string]any) error {
	status := entity.SyncStatusFailed
	if strings.Contains(message, "ROLLED_BACK") {
		status = entity.SyncStatusDrifted
	}
	if updates == nil {
		updates = configFailureUpdates(message)
	}
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := updateLeasedNode(tx, state, map[string]any{"sync_status": status}, false); err != nil {
			return err
		}
		return leaseUpdate(tx, state, updates)
	})
}

func configFailureUpdates(message string) map[string]any {
	updates := recoveryRetryUpdates(message)
	updates["runtime_hash"] = ""
	updates["runtime_authorization_hash"] = ""
	updates["verified_at"] = nil
	return updates
}

func recoveryRetryUpdates(message string) map[string]any {
	terminal := false
	for _, marker := range []string{"ROLLBACK_FAILED", "AUTHORIZATION_STOPPED", "DRIFT_CONFLICT", "PATH_MISMATCH", "VERSION_UNSUPPORTED", "PREFLIGHT_FAILED", "IPV6_UNAVAILABLE"} {
		if strings.Contains(message, marker) {
			terminal = true
		}
	}
	updates := map[string]any{"last_error": truncateRunes(message, 400), "recovery_attempts": gorm.Expr("recovery_attempts + 1"),
		"next_retry_at": time.Now().Add(time.Minute), "recovery_pending": true}
	if terminal {
		updates["recovery_attempts"] = 3
	}
	if strings.Contains(message, "ROLLED_BACK") {
		updates["next_retry_at"] = time.Now().Add(10 * time.Minute)
	}
	return updates
}

func finishNodeStatus(state *entity.NodeConfigState, hash string, current bool) error {
	status := entity.SyncStatusDrifted
	if current {
		status = entity.SyncStatusSynced
	}
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		return updateLeasedNode(tx, state, map[string]any{"sync_status": status, "config_hash": hash, "last_sync_at": time.Now()}, current)
	})
}

func updateLeasedNode(tx *gorm.DB, state *entity.NodeConfigState, updates map[string]any, requireCurrent bool) error {
	lease := tx.Model(&entity.NodeConfigState{}).Select("1").
		Where("node_id = nodes.id AND lease_owner = ? AND lease_until > ?", state.LeaseOwner, time.Now())
	if requireCurrent {
		lease = lease.Where("change_revision = ?", state.ChangeRevision)
	}
	r := tx.Model(&entity.Node{}).Where("id = ? AND EXISTS (?)", state.NodeID, lease).Updates(updates)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrConfigLeaseLost
	}
	return nil
}

var activeDeploymentPhases = []string{"validating", "staged", "persisted", "applying", "verifying", "rollback_running"}

func deploymentPhaseRecorder(state *entity.NodeConfigState) func(string) {
	return func(phase string) {
		if !slices.Contains(activeDeploymentPhases, phase) {
			return
		}
		err := repository.DB.Transaction(func(tx *gorm.DB) error {
			if phase == "persisted" {
				if err := leaseUpdate(tx, state, map[string]any{"disk_revision": state.DesiredRevision, "disk_hash": state.DesiredHash, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil}); err != nil {
					return err
				}
			}
			lease := currentConfigLease(tx, state).Select("1").Clauses(clause.Locking{Strength: "UPDATE"})
			return tx.Model(&entity.ConfigDeployment{}).
				Where("node_id = ? AND desired_hash = ? AND status IN ?", state.NodeID, state.DesiredHash, activeDeploymentPhases).
				Where("EXISTS (?)", lease).Updates(map[string]any{"status": phase, "desired_revision": state.DesiredRevision}).Error
		})
		if err != nil && !errors.Is(err, ErrConfigLeaseLost) {
			zap.L().Warn("记录配置部署阶段失败", zap.Uint("nodeID", state.NodeID), zap.String("phase", phase), zap.Error(err))
		}
	}
}
func (s *SyncService) RestorePolicy(nodeID uint, expected uint, actor string) error {
	var state entity.NodeConfigState
	if err := repository.DB.First(&state, "node_id = ?", nodeID).Error; err != nil {
		return err
	}
	if state.LastHealthyPolicy == "" {
		return fmt.Errorf("没有已验证的历史策略")
	}
	b, err := NewRoutingService().Binding("node", nodeID)
	if err != nil {
		return err
	}
	if state.LastHealthyPolicy == "legacy" {
		b.Mode = "legacy"
		b.TemplateID = 0
		b.Revision = 0
		b.Overrides = "{}"
	} else {
		// Store only routing policy; current users and keys are regenerated on execution.
		var rev entity.RoutingTemplateRevision
		if err := repository.DB.Where("template_id = ? AND revision = ?", state.LastHealthyTemplateID, state.LastHealthyTemplateRevision).First(&rev).Error; err != nil {
			return fmt.Errorf("历史模板修订不存在: %w", err)
		}
		overrides := state.LastHealthyOverrides
		var ovErr error
		if overrides == "" {
			// 旧库快照没有原覆盖，重建后仍必须验证完全等价。
			overrides, ovErr = xray.BuildOverridesForTarget(rev.Payload, state.LastHealthyPolicy)
		}
		if ovErr != nil {
			return ovErr
		}
		merged, mergeErr := xray.MergeRoutingPolicy(rev.Payload, overrides)
		if mergeErr != nil {
			return fmt.Errorf("历史节点覆盖无法恢复: %w", mergeErr)
		}
		mergedJSON, marshalErr := json.Marshal(merged)
		if marshalErr != nil || !xray.PoliciesSemanticallyEqual(string(mergedJSON), state.LastHealthyPolicy) {
			return fmt.Errorf("历史节点覆盖与已验证策略不一致，拒绝恢复")
		}
		b.Mode = "template"
		b.TemplateID = state.LastHealthyTemplateID
		b.Revision = state.LastHealthyTemplateRevision
		b.Overrides = overrides
	}
	return NewRoutingService().SaveBinding(b, expected, actor, true)
}

// Caller holds both the process lock and DB lease. Only policy is rolled back;
// every credential and permission is rebuilt from the latest database snapshot.
func (s *SyncService) recoverHealthyPolicy(nodeID uint, leaseOwner, actor, baseline string) error {
	var state entity.NodeConfigState
	if err := repository.DB.First(&state, "node_id = ?", nodeID).Error; err != nil {
		return err
	}
	if state.LeaseOwner != leaseOwner || state.LeaseUntil == nil || !state.LeaseUntil.After(time.Now()) {
		return ErrConfigLeaseLost
	}
	n, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return err
	}
	desired, _, err := s.buildConfig(n)
	if err != nil {
		return err
	}
	content, _, err := s.buildConfigPolicy(n, state.LastHealthyPolicy)
	if err != nil {
		return err
	}
	if err := rememberDesired(&state, xray.ConfigHash(desired)); err != nil {
		return err
	}
	p := s.sshParams(n)
	p.BaselineHash = baseline
	p.RollbackContent = content
	p.BeforeApply = func() error {
		if !s.stillCurrent(&state, xray.ConfigHash(desired)) {
			return fmt.Errorf("恢复期间期望配置再次变化")
		}
		return nil
	}
	r := pushConfig(p, content)
	if !r.Success {
		return fmt.Errorf("%s", r.Error)
	}
	// 自动恢复只记录实际文件，不改变绑定、期望修订或期望哈希。
	if err := leaseUpdate(repository.DB, &state, map[string]any{"disk_hash": xray.ConfigHash(content), "runtime_hash": xray.ConfigHash(content), "runtime_authorization_hash": xssh.AuthorizationHash(content), "verified_at": time.Now(), "process_id": r.ProcessID, "verification": "recovery_verified", "recovery_pending": true, "next_retry_at": time.Now().Add(10 * time.Minute)}); err != nil {
		return err
	}
	return finishNodeStatus(&state, xray.ConfigHash(content), false)
}
