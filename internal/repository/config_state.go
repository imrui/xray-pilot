package repository

import (
	"errors"
	"github.com/google/uuid"
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var ErrConfigLeased = errors.New("节点已有配置任务执行中")

// InvalidateConfigs is called inside the same transaction as a desired-state edit.
// The row doubles as a durable recovery outbox, even if the caller dies before live apply.
func InvalidateConfigs(tx *gorm.DB, ids []uint) error {
	if ids == nil {
		if err := tx.Model(&entity.Node{}).Where("active = ?", true).Pluck("id", &ids).Error; err != nil {
			return err
		}
	}
	for _, id := range uniqueSortedIDs(ids) {
		if err := tx.Model(&entity.Node{}).Where("id = ?", id).Update("sync_status", entity.SyncStatusDrifted).Error; err != nil {
			return err
		}
		if err := EnsureConfigState(tx, id); err != nil {
			return err
		}
		if err := tx.Model(&entity.NodeConfigState{}).Where("node_id = ?", id).Updates(map[string]any{
			"change_revision": gorm.Expr("change_revision + 1"), "recovery_pending": true,
			"recovery_attempts": 0, "next_retry_at": time.Now().Add(15 * time.Second),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
func EnsureConfigState(tx *gorm.DB, id uint) error {
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entity.NodeConfigState{NodeID: id}).Error
}
func AcquireConfigLease(id uint) (*entity.NodeConfigState, func(), error) {
	if err := EnsureConfigState(DB, id); err != nil {
		return nil, nil, err
	}
	owner := uuid.NewString()
	now := time.Now()
	r := DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND (lease_owner = '' OR lease_until < ? OR lease_until IS NULL)", id, now).
		UpdateColumns(map[string]any{"lease_owner": owner, "lease_until": now.Add(5 * time.Minute)})
	if r.Error != nil {
		return nil, nil, r.Error
	}
	if r.RowsAffected != 1 {
		return nil, nil, ErrConfigLeased
	}
	release := func() {
		DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", id, owner).UpdateColumns(map[string]any{"lease_owner": "", "lease_until": nil})
	}
	var state entity.NodeConfigState
	if err := DB.First(&state, "node_id = ?", id).Error; err != nil {
		release()
		return nil, nil, err
	}
	return &state, release, nil
}
