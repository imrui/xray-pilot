package repository

import (
	"fmt"
	"sort"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GroupRepository struct{}

func NewGroupRepository() *GroupRepository {
	return &GroupRepository{}
}

func (r *GroupRepository) Create(group *entity.Group) error {
	return DB.Omit("Nodes.*").Create(group).Error
}

func (r *GroupRepository) FindByID(id uint) (*entity.Group, error) {
	var group entity.Group
	err := DB.Preload("Nodes").First(&group, id).Error
	return &group, err
}

func (r *GroupRepository) List(page, pageSize int) ([]entity.Group, int64, error) {
	var total int64
	if err := DB.Model(&entity.Group{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var groups []entity.Group
	offset := (page - 1) * pageSize
	err := DB.Preload("Nodes").Order("id desc").Offset(offset).Limit(pageSize).Find(&groups).Error
	return groups, total, err
}

// Update saves metadata and optional membership changes atomically. A nil list
// preserves membership; an empty list clears it.
func (r *GroupRepository) Update(group *entity.Group, nodeIDs []uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var old entity.Group
		if err := tx.First(&old, group.ID).Error; err != nil {
			return err
		}
		before, err := groupNodeIDs(tx, group.ID)
		if err != nil {
			return err
		}
		// Preloaded associations must not recreate memberships removed by another edit.
		if err := tx.Omit(clause.Associations).Save(group).Error; err != nil {
			return err
		}
		after := before
		if nodeIDs != nil {
			if after, err = replaceGroupNodes(tx, group, nodeIDs); err != nil {
				return err
			}
		}
		return invalidateGroupChanges(tx, old.Active, group.Active, before, after)
	})
}

func (r *GroupRepository) UpdateActive(id uint, active bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var old entity.Group
		if err := tx.First(&old, id).Error; err != nil {
			return err
		}
		ids, err := groupNodeIDs(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Model(&entity.Group{}).Where("id = ?", id).Update("active", active).Error; err != nil {
			return err
		}
		return invalidateGroupChanges(tx, old.Active, active, ids, ids)
	})
}

func (r *GroupRepository) Delete(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var old entity.Group
		if err := tx.First(&old, id).Error; err != nil {
			return err
		}
		ids, err := groupNodeIDs(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Where("scope = ? AND target_id = ?", "group", id).Delete(&entity.RoutingBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Exec("DELETE FROM group_nodes WHERE group_id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&entity.UserGroup{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&entity.Group{}, id).Error; err != nil {
			return err
		}
		return invalidateGroupChanges(tx, old.Active, false, ids, nil)
	})
}

// ReplaceNodes 替换分组关联节点（many2many）
func (r *GroupRepository) ReplaceNodes(group *entity.Group, nodes []entity.Node) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var current entity.Group
		if err := tx.First(&current, group.ID).Error; err != nil {
			return err
		}
		before, err := groupNodeIDs(tx, group.ID)
		if err != nil {
			return err
		}
		ids := make([]uint, 0, len(nodes))
		for _, n := range nodes {
			ids = append(ids, n.ID)
		}
		after, err := replaceGroupNodes(tx, group, ids)
		if err != nil {
			return err
		}
		return invalidateGroupChanges(tx, current.Active, current.Active, before, after)
	})
}

func groupNodeIDs(tx *gorm.DB, id uint) ([]uint, error) {
	var ids []uint
	err := tx.Table("group_nodes").Where("group_id = ?", id).Order("node_id").Pluck("node_id", &ids).Error
	return ids, err
}

func replaceGroupNodes(tx *gorm.DB, group *entity.Group, ids []uint) ([]uint, error) {
	ids = uniqueSortedIDs(ids)
	nodes := []entity.Node{}
	if len(ids) > 0 {
		if err := tx.Where("id IN ?", ids).Find(&nodes).Error; err != nil {
			return nil, err
		}
		if len(nodes) != len(ids) {
			return nil, fmt.Errorf("选择的节点不存在，请刷新后重试")
		}
	}
	if err := tx.Omit("Nodes.*").Model(group).Association("Nodes").Replace(nodes); err != nil {
		return nil, err
	}
	return ids, nil
}

// Only membership changes in an enabled group can affect its users or routing.
// InvalidateConfigs(nil) means ALL enabled nodes, never an empty impact set.
func invalidateGroupChanges(tx *gorm.DB, beforeActive, afterActive bool, before, after []uint) error {
	changed := map[uint]bool{}
	if beforeActive {
		for _, id := range before {
			changed[id] = true
		}
	}
	if afterActive {
		for _, id := range after {
			changed[id] = !changed[id]
		}
	}
	ids := make([]uint, 0, len(changed))
	for id, different := range changed {
		if different {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return InvalidateConfigs(tx, ids)
}

func (r *GroupRepository) FindNamesByNodeID(nodeID uint) ([]string, error) {
	var names []string
	err := DB.
		Table("groups").
		Select("groups.name").
		Joins("JOIN group_nodes ON group_nodes.group_id = groups.id").
		Where("group_nodes.node_id = ?", nodeID).
		Order("groups.id asc").
		Scan(&names).Error
	return names, err
}
