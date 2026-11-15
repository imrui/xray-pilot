package service

import (
	"context"
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

func EnqueueDeployments(ids []uint, key, actor string, expected ...map[uint]string) ([]entity.ConfigDeployment, error) {
	jobs := []entity.ConfigDeployment{}
	seen := map[uint]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		var old entity.ConfigDeployment
		requestKey := fmt.Sprintf("%s:%d", key, id)
		if repository.DB.Where("request_key = ?", requestKey).First(&old).Error == nil {
			jobs = append(jobs, old)
			continue
		}
		n, err := repository.NewNodeRepository().FindByID(id)
		if err != nil {
			return nil, err
		}
		content, _, err := NewSyncService().buildConfig(n)
		if err != nil {
			return nil, err
		}
		if len(expected) > 0 && expected[0][id] != xray.ConfigHash(content) {
			return nil, ErrRoutingConflict
		}
		jobs = append(jobs, entity.ConfigDeployment{NodeID: id, RequestKey: requestKey, DesiredHash: xray.ConfigHash(content), Status: "pending", Actor: actor})
	}
	err := repository.DB.Transaction(func(tx *gorm.DB) error {
		for i := range jobs {
			if jobs[i].ID == 0 {
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&jobs[i]).Error; err != nil {
					return err
				}
				if jobs[i].ID == 0 {
					if err := tx.Where("request_key = ?", jobs[i].RequestKey).First(&jobs[i]).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return jobs, err
}

// Durable queue survives page closure. Interrupted jobs are unknown, never assumed successful.
func RunConfigDeployments(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		repository.DB.Model(&entity.ConfigDeployment{}).Where("status IN ? AND updated_at < ?", activeDeploymentPhases, time.Now().Add(-5*time.Minute)).Updates(map[string]any{"status": "unknown", "error": "执行中断，请核对节点后重试"})
		var unknown []entity.ConfigDeployment
		repository.DB.Where("status = ? AND reconcile_attempts < ? AND updated_at < ?", "unknown", 2, time.Now().Add(-time.Minute)).Limit(3).Find(&unknown)
		for _, job := range unknown {
			r := repository.DB.Model(&entity.ConfigDeployment{}).Where("id = ? AND reconcile_attempts = ?", job.ID, job.ReconcileAttempts).Update("reconcile_attempts", job.ReconcileAttempts+1)
			if r.Error == nil && r.RowsAffected == 1 {
				_, _ = ReconcileDeployment(job.ID)
			}
		}
		recoverConfigOutbox(ctx)
		var jobs []entity.ConfigDeployment
		if repository.DB.Where("status = ?", "pending").Order("id").Limit(10).Find(&jobs).Error != nil {
			continue
		}
		for _, job := range jobs {
			if ctx.Err() != nil {
				return
			}
			claim := repository.DB.Model(&entity.ConfigDeployment{}).Where("id = ? AND status = ?", job.ID, "pending").Updates(map[string]any{"status": "applying", "attempts": gorm.Expr("attempts + 1")})
			if claim.Error != nil || claim.RowsAffected != 1 {
				continue
			}
			state, msg := "failed", ""
			n, err := repository.NewNodeRepository().FindByID(job.NodeID)
			if err != nil {
				msg = "节点已不存在"
			} else {
				content, _, err := NewSyncService().buildConfig(n)
				if err != nil {
					msg = err.Error()
				} else if xray.ConfigHash(content) != job.DesiredHash {
					state = "superseded"
					msg = "期望配置已更新，请重新下发"
				} else {
					r := NewSyncService().syncExpected(job.NodeID, job.Actor, job.DesiredHash)
					if r.Success {
						state = "succeeded"
					} else {
						msg = r.Error
						if strings.Contains(msg, "期望配置已更新") {
							state = "superseded"
						}
						if strings.Contains(msg, "ROLLBACK_FAILED") {
							state = "rollback_failed"
						} else if strings.Contains(msg, "ROLLED_BACK") {
							state = "rolled_back"
						}
						if strings.Contains(msg, "未完成") {
							state = "unknown"
						}
					}
				}
			}
			repository.DB.Model(&entity.ConfigDeployment{}).Where("id = ? AND status IN ?", job.ID, activeDeploymentPhases).Updates(map[string]any{"status": state, "error": msg})
			repository.NewLogRepository().RecordWithActor("config_deployment", fmt.Sprintf("node:%d/job:%d", job.NodeID, job.ID), job.Actor, state == "succeeded", state+": "+msg, 0)
		}
	}
}

func recoverConfigOutbox(ctx context.Context) {
	settings := NewSettingService()
	if settings.GetInt(KeySchedulerDriftInterval) <= 0 && settings.Get(KeyXrayLiveApply) != "true" {
		return
	}
	var pending []entity.NodeConfigState
	if repository.DB.Where("recovery_pending = ? AND recovery_attempts < ? AND (next_retry_at IS NULL OR next_retry_at <= ?)", true, 3, time.Now()).Order("node_id").Limit(10).Find(&pending).Error != nil {
		return
	}
	for _, state := range pending {
		if ctx.Err() != nil {
			return
		}
		var activeJobs int64
		repository.DB.Model(&entity.ConfigDeployment{}).Where("node_id = ? AND status IN ?", state.NodeID, append(append([]string{}, activeDeploymentPhases...), "pending", "unknown")).Count(&activeJobs)
		if activeJobs > 0 {
			continue
		}
		n, err := repository.NewNodeRepository().FindByID(state.NodeID)
		if err != nil || !n.Active {
			continue
		}
		content, _, err := NewSyncService().buildConfig(n)
		if err == nil && xray.ConfigHash(content) == state.RuntimeHash && state.RuntimeHash == state.DiskHash && state.VerifiedAt != nil {
			repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND change_revision = ?", state.NodeID, state.ChangeRevision).Update("recovery_pending", false)
			continue
		}
		NewSyncService().SyncNode(state.NodeID, "system:config_recovery")
	}
}
