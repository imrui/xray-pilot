package repository

import (
	"errors"

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

func (r *AlertStateRepository) Save(st *entity.NodeAlertState) error {
	return DB.Save(st).Error
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
