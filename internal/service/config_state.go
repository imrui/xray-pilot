package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"gorm.io/gorm"
	"strings"
	"time"
)

var ErrConfigLeaseLost = errors.New("配置租约已失效，结果未确认")

func leaseUpdate(tx *gorm.DB, state *entity.NodeConfigState, updates map[string]any) error {
	r := tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ? AND lease_until > ?", state.NodeID, state.LeaseOwner, time.Now()).Updates(updates)
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
		if effective.Policy != nil {
			data, _ := json.Marshal(effective.Policy)
			raw = string(data)
		}
		updates["last_healthy_policy"] = raw
		updates["last_healthy_template_id"] = effective.TemplateID
		updates["last_healthy_template_revision"] = effective.Revision
	}
	// A receipt belongs to its snapshot. Never clear work queued during the SSH call.
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := leaseUpdate(tx, state, updates); err != nil {
			return err
		}
		return tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND change_revision = ? AND lease_owner = ?", state.NodeID, state.ChangeRevision, state.LeaseOwner).
			Update("recovery_pending", false).Error
	})
}
func recordConfigFailure(id uint, message string) {
	terminal := false
	for _, marker := range []string{"ROLLBACK_FAILED", "AUTHORIZATION_STOPPED", "DRIFT_CONFLICT", "PATH_MISMATCH", "VERSION_UNSUPPORTED", "PREFLIGHT_FAILED", "IPV6_UNAVAILABLE"} {
		if strings.Contains(message, marker) {
			terminal = true
		}
	}
	updates := map[string]any{"last_error": truncateRunes(message, 400), "recovery_attempts": gorm.Expr("recovery_attempts + 1"),
		"next_retry_at": time.Now().Add(time.Minute), "recovery_pending": true, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil}
	if terminal {
		updates["recovery_attempts"] = 3
	}
	if strings.Contains(message, "ROLLED_BACK") {
		delete(updates, "runtime_hash")
		delete(updates, "runtime_authorization_hash")
		delete(updates, "verified_at")
		updates["recovery_pending"] = true
		updates["next_retry_at"] = time.Now().Add(10 * time.Minute)
	}
	repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", id).Updates(updates)
}

func finishNodeStatus(state *entity.NodeConfigState, hash string, current bool) error {
	status := entity.SyncStatusDrifted
	if current {
		status = entity.SyncStatusSynced
	}
	now := time.Now()
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&entity.Node{}).Where("id = ?", state.NodeID)
		q = q.Where("EXISTS (SELECT 1 FROM node_config_states s WHERE s.node_id = nodes.id AND s.lease_owner = ? AND s.lease_until > ?)", state.LeaseOwner, time.Now())
		if current {
			q = q.Where("EXISTS (SELECT 1 FROM node_config_states s WHERE s.node_id = nodes.id AND s.change_revision = ? AND s.lease_owner = ?)", state.ChangeRevision, state.LeaseOwner)
		}
		r := q.Updates(map[string]any{"sync_status": status, "config_hash": hash, "last_sync_at": now})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return ErrConfigLeaseLost
		}
		return nil
	})
}

var activeDeploymentPhases = []string{"validating", "staged", "persisted", "applying", "verifying", "rollback_running"}

func deploymentPhaseRecorder(state *entity.NodeConfigState) func(string) {
	return func(phase string) {
		allowed := false
		for _, v := range activeDeploymentPhases {
			if v == phase {
				allowed = true
			}
		}
		if !allowed {
			return
		}
		repository.DB.Model(&entity.ConfigDeployment{}).Where("node_id = ? AND desired_hash = ? AND status IN ?", state.NodeID, state.DesiredHash, activeDeploymentPhases).Updates(map[string]any{"status": phase, "desired_revision": state.DesiredRevision})
		if phase == "persisted" {
			_ = leaseUpdate(repository.DB, state, map[string]any{"disk_revision": state.DesiredRevision, "disk_hash": state.DesiredHash, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil})
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
		b.Mode = "template"
		b.TemplateID = state.LastHealthyTemplateID
		b.Revision = state.LastHealthyTemplateRevision
		b.Overrides = state.LastHealthyPolicy
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
