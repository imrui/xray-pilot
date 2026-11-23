package repository

import (
	"fmt"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	return DB.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "sqlite" {
			if err := nextSQLiteNodeID(tx, node); err != nil {
				return err
			}
		}
		if err := tx.Create(node).Error; err != nil {
			return err
		}
		return tx.Create(&entity.RoutingBinding{Scope: "node", TargetID: node.ID, Mode: "inherit", Overrides: "{}", Version: 1}).Error
	})
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
	IDs        []uint
	Keyword    string // 模糊匹配名称、地址、地区、所有者及备注
	SyncStatus string // 精确匹配同步状态
	Region     string
	Owner      string
	// Health 健康检测筛选：healthy / unhealthy / unchecked（空 = 不筛选）。
	// 语义与仪表盘一致：异常 = 检测过且失败；从未检测过的不算异常。
	Health          string
	HealthDashboard bool
}

func (f NodeListFilter) apply(db *gorm.DB) *gorm.DB {
	if f.IDs != nil {
		db = db.Where("id IN ?", f.IDs)
	}
	if keyword := strings.TrimSpace(f.Keyword); keyword != "" {
		kw := "%" + escapeLike(strings.ToLower(keyword)) + "%"
		db = db.Where(
			`(LOWER(name) LIKE ? ESCAPE '\' OR LOWER(ip) LIKE ? ESCAPE '\' OR LOWER(domain) LIKE ? ESCAPE '\'
				OR LOWER(region) LIKE ? ESCAPE '\' OR LOWER(owner) LIKE ? ESCAPE '\' OR LOWER(remark) LIKE ? ESCAPE '\')`,
			kw, kw, kw, kw, kw, kw,
		)
	}
	if f.SyncStatus == "needs_sync" {
		db = db.Where("sync_status IN ?", []string{"pending", "drifted", "failed"})
	} else if f.SyncStatus != "" {
		db = db.Where("sync_status = ?", f.SyncStatus)
	}
	if f.Region != "" {
		db = db.Where("region = ?", f.Region)
	}
	if f.Owner != "" {
		db = db.Where("owner = ?", f.Owner)
	}
	if f.HealthDashboard {
		db = nodesWithActiveProtocolsScope(db.Where("nodes.active = ?", true))
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

// Update 只写请求列。在行锁内比较连接与配置变更，回读提交时的同步状态。
func (r *NodeRepository) Update(id uint, fields map[string]any, actor string) (old, updated entity.Node, err error) {
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, id).Error; err != nil {
			return err
		}
		if len(fields) > 0 {
			if err := tx.Model(&entity.Node{}).Where("id = ?", id).Updates(fields).Error; err != nil {
				return err
			}
		}
		if err := tx.First(&updated, id).Error; err != nil {
			return err
		}
		if err := updateDNSIntent(tx, &updated, actor); err != nil {
			return err
		}
		ipChanged := old.IP != updated.IP
		if ipChanged || old.Domain != updated.Domain || old.LogLevel != updated.LogLevel {
			if err := InvalidateConfigs(tx, []uint{id}); err != nil {
				return err
			}
			updated.SyncStatus = entity.SyncStatusDrifted
		}
		if ipChanged {
			// 保留磁盘基线的覆盖保护，新地址的运行状态必须重新核验。
			result := tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND (lease_until IS NULL OR lease_until <= ? OR lease_owner = '')", id, time.Now()).
				Updates(map[string]any{"verified_at": nil, "runtime_hash": "", "runtime_authorization_hash": "", "runtime_revision": 0, "verification": "", "process_id": ""})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrConfigLeased
			}
		}
		if old.SSHPort != updated.SSHPort || old.SSHUser != updated.SSHUser || old.SSHKeyPath != updated.SSHKeyPath {
			return tx.Model(&entity.NodeConfigState{}).Where("node_id = ? AND recovery_pending = ? AND LOWER(last_error) LIKE ?", id, true, "%ssh%").
				Updates(map[string]any{"recovery_attempts": 0, "next_retry_at": time.Now()}).Error
		}
		return nil
	})
	return
}

func (r *NodeRepository) UpdateActive(id uint, active bool) error {
	return DB.Model(&entity.Node{}).Where("id = ?", id).Update("active", active).Error
}

// ReplaceHost invalidates receipts from the old machine while preserving routing bindings.
// The caller holds the configuration lease throughout replacement registration.
func (r *NodeRepository) ReplaceHost(node *entity.Node, bootstrapHash, actor string) error {
	var updated entity.Node
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := InvalidateConfigs(tx, []uint{node.ID}); err != nil {
			return err
		}
		// 登记回调持有的是读到的旧快照。只替换机器自身的字段，保留并发的管理与健康检测更新。
		if err := tx.Model(&entity.Node{}).Where("id = ?", node.ID).Updates(map[string]any{
			"ip": node.IP, "ssh_user": node.SSHUser, "ssh_port": node.SSHPort,
			"xray_version": node.XrayVersion, "xray_active": false, "registered_at": node.RegisteredAt,
			"sync_status": entity.SyncStatusPending, "config_hash": "",
		}).Error; err != nil {
			return err
		}
		if err := tx.First(&updated, node.ID).Error; err != nil {
			return err
		}
		if err := updateDNSIntent(tx, &updated, actor); err != nil {
			return err
		}
		if err := tx.Model(&entity.NodeConfigState{}).Where("node_id = ?", node.ID).Updates(map[string]any{
			"initial_baseline": bootstrapHash != "", "initial_baseline_hash": bootstrapHash,
			"desired_hash": "", "disk_hash": "", "runtime_hash": "", "runtime_authorization_hash": "", "disk_revision": 0, "runtime_revision": 0,
			"verified_at": nil, "process_id": "", "verification": "", "last_error": "",
		}).Error; err != nil {
			return err
		}
		return tx.Model(&entity.ConfigDeployment{}).Where("node_id = ? AND status IN ?", node.ID, []string{"pending", "validating", "staged", "persisted", "applying", "verifying", "unknown"}).Updates(map[string]any{"status": "superseded", "error": "节点已更换服务器，请重新下发"}).Error
	})
	if err == nil {
		*node = updated
	}
	return err
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
	return AuthorizationTransaction(func(tx *gorm.DB) error {
		var node entity.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&node, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		// 删除节点会销毁 SSH 连接信息，先确认该节点尚未完成的到期撤权。
		var pending int64
		if err := tx.Model(&entity.UserExpiryTarget{}).Where("node_id = ?", id).Count(&pending).Error; err != nil {
			return err
		}
		if pending == 0 {
			members := configUserMembership(tx, true).Select("ug.user_id").Where("gn.node_id = ?", id)
			if err := dueExpiries(tx, time.Now()).Where("id IN (?)", members).Count(&pending).Error; err != nil {
				return err
			}
		}
		if pending > 0 {
			return fmt.Errorf("节点仍有未确认的到期撤权，完成撤权后可删除")
		}
		if err := tx.Create(&entity.DeletedNode{NodeID: id, Name: node.Name, CreatedAt: node.CreatedAt, DeletedAt: time.Now()}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.DNSBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM group_nodes WHERE node_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.NodeProfileKey{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.NodeAlertState{}).Error; err != nil {
			return err
		}
		if err := tx.Where("scope = ? AND target_id = ?", "node", id).Delete(&entity.RoutingBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.ConfigDeployment{}).Where("node_id = ? AND status IN ?", id, []string{"pending", "validating", "staged", "persisted", "applying", "verifying", "unknown"}).Updates(map[string]any{"status": "superseded", "error": "节点已删除"}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.NodeConfigState{}).Error; err != nil {
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

// UpdateXrayActive 仅更新 Xray 运行状态（健康检测代理端口探测结果，不动版本）
func (r *NodeRepository) UpdateXrayActive(id uint, active bool) error {
	return DB.Model(&entity.Node{}).Where("id = ?", id).Update("xray_active", active).Error
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
		DB.Table("group_nodes").Select("DISTINCT node_id").Joins("JOIN groups g ON g.id = group_nodes.group_id AND g.active = ?", true).Where("group_id IN ?", groupIDs),
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
