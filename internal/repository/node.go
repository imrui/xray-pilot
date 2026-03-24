package repository

import (
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

type NodeRepository struct{}

func NewNodeRepository() *NodeRepository {
	return &NodeRepository{}
}

func (r *NodeRepository) Create(node *entity.Node) error {
	return DB.Create(node).Error
}

func (r *NodeRepository) FindByID(id uint) (*entity.Node, error) {
	var node entity.Node
	err := DB.First(&node, id).Error
	return &node, err
}

func (r *NodeRepository) List(page, pageSize int) ([]entity.Node, int64, error) {
	var total int64
	if err := DB.Model(&entity.Node{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var nodes []entity.Node
	offset := (page - 1) * pageSize
	err := DB.Order("id desc").Offset(offset).Limit(pageSize).Find(&nodes).Error
	return nodes, total, err
}

func (r *NodeRepository) FindAll() ([]entity.Node, error) {
	var nodes []entity.Node
	err := DB.Where("active = ?", true).Find(&nodes).Error
	return nodes, err
}

func (r *NodeRepository) Update(node *entity.Node) error {
	return DB.Save(node).Error
}

func (r *NodeRepository) UpdateActive(id uint, active bool) error {
	return DB.Model(&entity.Node{}).Where("id = ?", id).Update("active", active).Error
}

func (r *NodeRepository) Delete(id uint) error {
	return DB.Delete(&entity.Node{}, id).Error
}

// UpdateSyncStatus 更新节点同步状态和配置哈希
func (r *NodeRepository) UpdateSyncStatus(id uint, status entity.SyncStatus, hash string) error {
	updates := map[string]any{"sync_status": status}
	if hash != "" {
		updates["config_hash"] = hash
	}
	return DB.Model(&entity.Node{}).Where("id = ?", id).Updates(updates).Error
}

// GetDriftedNodes 查询需要同步的节点（drifted 或 failed）
func (r *NodeRepository) GetDriftedNodes() ([]entity.Node, error) {
	var nodes []entity.Node
	err := DB.Where("sync_status IN ? AND active = ?",
		[]entity.SyncStatus{entity.SyncStatusDrifted, entity.SyncStatusFailed},
		true,
	).Find(&nodes).Error
	return nodes, err
}

// FindByIDs 批量查询节点
func (r *NodeRepository) FindByIDs(ids []uint) ([]entity.Node, error) {
	var nodes []entity.Node
	err := DB.Where("id IN ?", ids).Find(&nodes).Error
	return nodes, err
}

// UpdateLastSync 更新最后同步时间
func (r *NodeRepository) UpdateLastSync(id uint, status entity.SyncStatus, hash string) error {
	now := time.Now()
	updates := map[string]any{
		"sync_status": status,
		"last_sync_at": &now,
	}
	if hash != "" {
		updates["config_hash"] = hash
	}
	return DB.Model(&entity.Node{}).Where("id = ?", id).Updates(updates).Error
}

// UpdateLastCheck 更新健康检测结果
func (r *NodeRepository) UpdateLastCheck(id uint, ok bool, latencyMs int) error {
	now := time.Now()
	return DB.Model(&entity.Node{}).Where("id = ?", id).Updates(map[string]any{
		"last_check_at":   &now,
		"last_check_ok":   ok,
		"last_latency_ms": latencyMs,
	}).Error
}

// FindActiveWithSSH 查询有 SSH 配置的激活节点
func (r *NodeRepository) FindActiveWithSSH() ([]entity.Node, error) {
	var nodes []entity.Node
	err := DB.Where("active = ? AND ssh_key_path != ''", true).Find(&nodes).Error
	return nodes, err
}

// MarkAllDrifted 将所有激活节点标记为 drifted（用于全局配置变更后批量触发）
func (r *NodeRepository) MarkAllDrifted() error {
	return DB.Model(&entity.Node{}).
		Where("active = ?", true).
		Update("sync_status", entity.SyncStatusDrifted).Error
}

// FindHealthyByGroupID 查询分组内 Active=true AND LastCheckOK=true 的节点（用于订阅生成）
// 若 lastCheckOKFilter=false 则不过滤 LastCheckOK（允许返回未经检测的节点）
func (r *NodeRepository) FindHealthyByGroupID(groupID uint, lastCheckOKFilter bool) ([]entity.Node, error) {
	var nodes []entity.Node
	query := DB.Where(
		"active = ? AND id IN (?)",
		true,
		DB.Table("group_nodes").Select("node_id").Where("group_id = ?", groupID),
	)
	if lastCheckOKFilter {
		query = query.Where("last_check_ok = ?", true)
	}
	err := query.Find(&nodes).Error
	return nodes, err
}
