package service

import (
	"encoding/json"
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"gorm.io/gorm"
	"strings"
	"time"
)

func rememberDesired(state *entity.NodeConfigState, hash string) error {
	if state.DesiredHash != hash {
		state.DesiredRevision++
		state.DesiredHash = hash
	}
	return repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", state.NodeID, state.LeaseOwner).
		Updates(map[string]any{"desired_revision": state.DesiredRevision, "desired_hash": hash}).Error
}
func (s *SyncService) stillCurrent(state *entity.NodeConfigState, hash string) bool {
	var latest entity.NodeConfigState
	if repository.DB.First(&latest, "node_id = ?", state.NodeID).Error != nil || latest.LeaseOwner != state.LeaseOwner || latest.ChangeRevision != state.ChangeRevision {
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
		"last_error": "", "recovery_attempts": 0}
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
		if err := tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", state.NodeID, state.LeaseOwner).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND change_revision = ? AND lease_owner = ?", state.NodeID, state.ChangeRevision, state.LeaseOwner).
			Update("recovery_pending", false).Error
	})
}
func recordConfigFailure(id uint, message string) {
	terminal := false
	for _, marker := range []string{"ROLLED_BACK", "ROLLBACK_FAILED", "AUTHORIZATION_STOPPED", "DRIFT_CONFLICT", "PATH_MISMATCH", "VERSION_UNSUPPORTED", "PREFLIGHT_FAILED", "IPV6_UNAVAILABLE"} {
		if strings.Contains(message, marker) {
			terminal = true
		}
	}
	updates := map[string]any{"last_error": truncateRunes(message, 400), "recovery_attempts": gorm.Expr("recovery_attempts + 1"),
		"next_retry_at": time.Now().Add(time.Minute), "recovery_pending": true, "runtime_hash": "", "verified_at": nil}
	if terminal {
		updates["recovery_attempts"] = 3
	}
	if strings.Contains(message, "ROLLED_BACK") {
		delete(updates, "runtime_hash")
		delete(updates, "verified_at")
		updates["recovery_pending"] = false
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
		if current {
			q = q.Where("EXISTS (SELECT 1 FROM node_config_states s WHERE s.node_id = nodes.id AND s.change_revision = ? AND s.lease_owner = ?)", state.ChangeRevision, state.LeaseOwner)
		}
		r := q.Updates(map[string]any{"sync_status": status, "config_hash": hash, "last_sync_at": now})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return tx.Model(&entity.Node{}).Where("id = ?", state.NodeID).Updates(map[string]any{"sync_status": entity.SyncStatusDrifted, "config_hash": hash, "last_sync_at": now}).Error
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
			repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", state.NodeID, state.LeaseOwner).Updates(map[string]any{"disk_revision": state.DesiredRevision, "disk_hash": state.DesiredHash, "runtime_hash": "", "verified_at": nil})
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
func (s *SyncService) recoverHealthyPolicy(nodeID, bindingVersion uint, actor, baseline string) error {
	if err := s.RestorePolicy(nodeID, bindingVersion, "system:rollback:"+actor); err != nil {
		return err
	}
	var state entity.NodeConfigState
	if err := repository.DB.First(&state, "node_id = ?", nodeID).Error; err != nil {
		return err
	}
	if state.LeaseUntil == nil || state.LeaseUntil.Before(time.Now()) {
		return fmt.Errorf("配置租约已过期，停止自动恢复")
	}
	if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", nodeID, state.LeaseOwner).Update("lease_until", time.Now().Add(5*time.Minute)).Error; err != nil {
		return err
	}
	n, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return err
	}
	content, _, err := s.buildConfig(n)
	if err != nil {
		return err
	}
	snapshot, err := NewRoutingService().Effective(nodeID)
	if err != nil {
		return err
	}
	if err := rememberDesired(&state, xray.ConfigHash(content)); err != nil {
		return err
	}
	p := s.sshParams(n)
	p.BaselineHash = baseline
	p.BeforeApply = func() error {
		if !s.stillCurrent(&state, xray.ConfigHash(content)) {
			return fmt.Errorf("恢复期间期望配置再次变化，停止恢复")
		}
		return nil
	}
	r := pushConfig(p, content)
	if !r.Success {
		return fmt.Errorf("%s", r.Error)
	}
	if err := s.recordApplied(&state, content, r.ProcessID, snapshot); err != nil {
		return err
	}
	return s.nodeRepo.UpdateLastSync(nodeID, entity.SyncStatusFailed, xray.ConfigHash(content))
}
