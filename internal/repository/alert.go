package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/entity"
)

type AlertStateRepository struct{}

func NewAlertStateRepository() *AlertStateRepository { return &AlertStateRepository{} }

// Get 取节点 × 类型的状态；不存在时返回零值状态（未入库），调用方 Save 时创建
func (r *AlertStateRepository) Get(nodeID uint, kind string) (*entity.NodeAlertState, error) {
	var st entity.NodeAlertState
	err := DB.Where("node_id = ? AND kind = ?", nodeID, kind).First(&st).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &entity.NodeAlertState{NodeID: nodeID, Kind: kind}, nil
	}
	if err != nil {
		return nil, err
	}
	return &st, nil
}

// SaveObservation 只写检测推进的字段（计数 / 判定 / 起点 / 错误），不碰送达字段，
// 避免与 Flush 回写送达状态互相覆盖（v0.5.1 实测：并发时恢复消息丢失）
func (r *AlertStateRepository) SaveObservation(st *entity.NodeAlertState) error {
	if st.ID == 0 {
		return DB.Create(st).Error
	}
	return DB.Model(st).
		Select("fail_count", "ok_count", "alerting", "since", "last_error", "updated_at").
		Updates(st).Error
}

// MarkNotified 只写送达字段：notified 为本次送达时的判定状态
func (r *AlertStateRepository) MarkNotified(id uint, notified bool, at time.Time) error {
	return DB.Model(&entity.NodeAlertState{}).Where("id = ?", id).Updates(map[string]any{
		"notified_alerting": notified,
		"last_notified_at":  at,
	}).Error
}

// FindPending 查询待发送或可能需要重复提醒的状态（当前告警中，或判定与已送达状态不一致）
func (r *AlertStateRepository) FindPending() ([]entity.NodeAlertState, error) {
	var states []entity.NodeAlertState
	err := DB.Where("alerting = ? OR alerting <> notified_alerting", true).
		Order("node_id asc, kind asc").
		Find(&states).Error
	return states, err
}

// DeleteExceptNodes 清掉不在 keep 列表里的节点级状态（禁用节点 / 已删节点，不发恢复），面板级（node_id=0）保留
func (r *AlertStateRepository) DeleteExceptNodes(keep []uint) error {
	q := DB.Where("node_id <> 0")
	if len(keep) > 0 {
		q = q.Where("node_id NOT IN ?", keep)
	}
	return q.Delete(&entity.NodeAlertState{}).Error
}
