package repository

import (
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

type NodeRepository struct{}

func nodesWithActiveProtocolsScope(db *gorm.DB) *gorm.DB {
	return db.Where(
		`EXISTS (
			SELECT 1
			FROM node_profile_keys npk
			JOIN inbound_profiles ip ON ip.id = npk.profile_id
			WHERE npk.node_id = nodes.id AND ip.active = ?
		)`,
		true,
	)
}

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

// FindByName 按节点名查找；当前主要用于 install 流程的同名冲突预检。
// 其他路径仍允许同名（历史行为兼容）。
func (r *NodeRepository) FindByName(name string) (*entity.Node, error) {
	var node entity.Node
	err := DB.Where("name = ?", name).First(&node).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

// NodeListFilter 节点列表的服务端筛选条件。
// 筛选必须在 DB 层做：前端拿到的只是当前页数据，客户端过滤会漏掉其他页。
type NodeListFilter struct {
	Keyword    string // 模糊匹配 name/ip/domain/region/owner
	SyncStatus string // 精确匹配同步状态
	Region     string
	Owner      string
	// Health 健康检测筛选：healthy / unhealthy / unchecked（空 = 不筛选）。
	// 语义与仪表盘一致：异常 = 检测过且失败；从未检测过的不算异常。
	Health string
}

func (f NodeListFilter) apply(db *gorm.DB) *gorm.DB {
	if f.Keyword != "" {
		kw := "%" + strings.ToLower(f.Keyword) + "%"
		db = db.Where(
			"LOWER(name) LIKE ? OR LOWER(ip) LIKE ? OR LOWER(domain) LIKE ? OR LOWER(region) LIKE ? OR LOWER(owner) LIKE ?",
			kw, kw, kw, kw, kw,
		)
	}
	if f.SyncStatus != "" {
		db = db.Where("sync_status = ?", f.SyncStatus)
	}
	if f.Region != "" {
		db = db.Where("region = ?", f.Region)
	}
	if f.Owner != "" {
		db = db.Where("owner = ?", f.Owner)
	}
	switch f.Health {
	case "healthy":
		db = db.Where("last_check_at IS NOT NULL AND last_check_ok = ?", true)
	case "unhealthy":
		db = db.Where("last_check_at IS NOT NULL AND last_check_ok = ?", false)
	case "unchecked":
		db = db.Where("last_check_at IS NULL")
	}
	return db
}

func (r *NodeRepository) List(page, pageSize int, filter NodeListFilter) ([]entity.Node, int64, error) {
	var total int64
	if err := filter.apply(DB.Model(&entity.Node{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var nodes []entity.Node
	offset := (page - 1) * pageSize
	err := filter.apply(DB.Model(&entity.Node{})).
		Order("id desc").Offset(offset).Limit(pageSize).Find(&nodes).Error
	return nodes, total, err
}

// DistinctRegions 返回所有非空地区值，供列表筛选下拉使用（不受分页影响）
func (r *NodeRepository) DistinctRegions() ([]string, error) {
	var values []string
	err := DB.Model(&entity.Node{}).
		Where("region <> ''").
		Distinct("region").
		Order("region asc").
		Pluck("region", &values).Error
	return values, err
}

// DistinctOwners 返回所有非空所有者值，供列表筛选下拉使用（不受分页影响）
func (r *NodeRepository) DistinctOwners() ([]string, error) {
	var values []string
	err := DB.Model(&entity.Node{}).
		Where("owner <> ''").
		Distinct("owner").
		Order("owner asc").
		Pluck("owner", &values).Error
	return values, err
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

// Delete 删除节点并级联清理所有关联表
//
// GORM 多对多默认不会清理中间表，且 SQLite 在删除最大 ID 行后会复用 ID。
// 这两点叠加会让"删节点 → 新建节点"撞到旧节点 ID 的 orphan 关联，新节点
// 会"继承"旧节点的分组、协议密钥，造成数据串。本方法用事务保证三处一起删：
//   - group_nodes（节点-分组多对多中间表）
//   - node_profile_keys（节点-协议密钥绑定）
//   - nodes（节点本身）
func (r *NodeRepository) Delete(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM group_nodes WHERE node_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.NodeProfileKey{}).Error; err != nil {
			return err
		}
		return tx.Delete(&entity.Node{}, id).Error
	})
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
	err := nodesWithActiveProtocolsScope(DB).
		Where("sync_status IN ? AND active = ?",
			[]entity.SyncStatus{entity.SyncStatusDrifted, entity.SyncStatusFailed, entity.SyncStatusPending},
			true,
		).
		Find(&nodes).Error
	return nodes, err
}

func (r *NodeRepository) CountSyncStatuses() (map[entity.SyncStatus]int64, error) {
	rows, err := nodesWithActiveProtocolsScope(DB.Model(&entity.Node{})).
		Select("sync_status, COUNT(*) AS total").
		Where("active = ?", true).
		Group("sync_status").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[entity.SyncStatus]int64{
		entity.SyncStatusDrifted: 0,
		entity.SyncStatusFailed:  0,
		entity.SyncStatusPending: 0,
		entity.SyncStatusSynced:  0,
	}

	for rows.Next() {
		var (
			status string
			total  int64
		)
		if scanErr := rows.Scan(&status, &total); scanErr != nil {
			return nil, scanErr
		}
		counts[entity.SyncStatus(status)] = total
	}

	return counts, rows.Err()
}

// BatchUpdateSyncStatus 批量更新节点同步状态
func (r *NodeRepository) BatchUpdateSyncStatus(ids []uint, status entity.SyncStatus) error {
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&entity.Node{}).Where("id IN ?", ids).Update("sync_status", status).Error
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
		"sync_status":  status,
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

// UpdateXrayStatus 更新节点 Xray 运行状态和版本
func (r *NodeRepository) UpdateXrayStatus(id uint, active bool, version string) error {
	return DB.Model(&entity.Node{}).Where("id = ?", id).Updates(map[string]any{
		"xray_active":  active,
		"xray_version": version,
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

// FindHealthyByGroupIDs 查询分组集合内的可用节点。
// 若 lastCheckOKFilter=false 则不过滤 LastCheckOK（允许返回未经检测的节点）
func (r *NodeRepository) FindHealthyByGroupIDs(groupIDs []uint, lastCheckOKFilter bool) ([]entity.Node, error) {
	var nodes []entity.Node
	if len(groupIDs) == 0 {
		return nodes, nil
	}
	query := DB.Where(
		"active = ? AND id IN (?)",
		true,
		DB.Table("group_nodes").Select("DISTINCT node_id").Where("group_id IN ?", groupIDs),
	)
	if lastCheckOKFilter {
		query = query.Where("last_check_ok = ?", true)
	}
	err := query.Order("id asc").Find(&nodes).Error
	return nodes, err
}

// FindHealthyByGroupID 向后兼容单分组查询。
func (r *NodeRepository) FindHealthyByGroupID(groupID uint, lastCheckOKFilter bool) ([]entity.Node, error) {
	return r.FindHealthyByGroupIDs([]uint{groupID}, lastCheckOKFilter)
}
