package repository

import (
	"fmt"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
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

func (r *GroupRepository) List(page, pageSize int, keyword string, active *bool) ([]entity.Group, int64, error) {
	query := DB.Model(&entity.Group{})
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + escapeLike(strings.ToLower(keyword)) + "%"
		query = query.Where("LOWER(name) LIKE ? ESCAPE '\\' OR LOWER(description) LIKE ? ESCAPE '\\'", like, like)
	}
	if active != nil {
		query = query.Where("active = ?", *active)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var groups []entity.Group
	offset := (page - 1) * pageSize
	err := query.Preload("Nodes").Order("id desc").Offset(offset).Limit(pageSize).Find(&groups).Error
	return groups, total, err
}

// Update saves metadata and optional membership changes atomically. A nil list
// preserves membership; an empty list clears it.
func (r *GroupRepository) Update(id uint, fields map[string]any, nodeIDs []uint) error {
	return groupMembershipTransaction(id, func(tx *gorm.DB, expiring []uint) error {
		var old entity.Group
		if err := tx.First(&old, id).Error; err != nil {
			return err
		}
		before, err := groupNodeIDs(tx, id)
		if err != nil {
			return err
		}
		active := old.Active
		if v, ok := fields["active"].(bool); ok {
			active = v
		}
		after := before
		if nodeIDs != nil {
			after = nodeIDs
		}
		if err := rememberGroupDepartures(tx, id, before, after, old.Active, active); err != nil {
			return err
		}
		if len(fields) != 0 {
			if err := tx.Model(&entity.Group{}).Where("id = ?", id).Updates(fields).Error; err != nil {
				return err
			}
		}
		if nodeIDs != nil {
			if after, err = replaceGroupNodes(tx, &old, nodeIDs); err != nil {
				return err
			}
		}
		return invalidateGroupChanges(tx, id, expiring, old.Active, active, before, after)
	})
}

func (r *GroupRepository) ToggleActive(id uint) error {
	return groupMembershipTransaction(id, func(tx *gorm.DB, expiring []uint) error {
		var old entity.Group
		if err := tx.First(&old, id).Error; err != nil {
			return err
		}
		ids, err := groupNodeIDs(tx, id)
		if err != nil {
			return err
		}
		if err := rememberGroupDepartures(tx, id, ids, ids, old.Active, !old.Active); err != nil {
			return err
		}
		if err := tx.Model(&entity.Group{}).Where("id = ?", id).Update("active", !old.Active).Error; err != nil {
			return err
		}
		return invalidateGroupChanges(tx, id, expiring, old.Active, !old.Active, ids, ids)
	})
}

func (r *GroupRepository) Delete(id uint) error {
	return groupMembershipTransaction(id, func(tx *gorm.DB, expiring []uint) error {
		var old entity.Group
		if err := tx.First(&old, id).Error; err != nil {
			return err
		}
		ids, err := groupNodeIDs(tx, id)
		if err != nil {
			return err
		}
		if err := rememberGroupDepartures(tx, id, ids, nil, old.Active, false); err != nil {
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
		return invalidateGroupChanges(tx, id, expiring, old.Active, false, ids, nil)
	})
}

// ReplaceNodes 替换分组关联节点（many2many）
func (r *GroupRepository) ReplaceNodes(group *entity.Group, nodes []entity.Node) error {
	return groupMembershipTransaction(group.ID, func(tx *gorm.DB, expiring []uint) error {
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
		if err := rememberGroupDepartures(tx, group.ID, before, ids, current.Active, current.Active); err != nil {
			return err
		}
		after, err := replaceGroupNodes(tx, group, ids)
		if err != nil {
			return err
		}
		return invalidateGroupChanges(tx, group.ID, expiring, current.Active, current.Active, before, after)
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

// 先固定分组内已到期用户的原节点，成员关系变化与到期失效合并成一次修订。
func groupMembershipTransaction(id uint, edit func(*gorm.DB, []uint) error) error {
	return AuthorizationTransaction(func(tx *gorm.DB) error {
		nodes, err := registerDueExpiries(tx, time.Now(), id)
		if err != nil {
			return err
		}
		return edit(tx, nodes)
	})
}

// Only membership changes in an enabled group can affect its users or routing.
// InvalidateConfigs(nil) means ALL enabled nodes, never an empty impact set.
func invalidateGroupChanges(tx *gorm.DB, groupID uint, expiring []uint, beforeActive, afterActive bool, before, after []uint) error {
	if !beforeActive {
		before = nil
	}
	if !afterActive {
		after = nil
	}
	added := nodeDifference(after, before)
	if err := cancelRegrantedGroupTargets(tx, groupID, added); err != nil {
		return err
	}
	ids := append(expiring, nodeDifference(before, after)...)
	ids = append(ids, added...)
	if len(ids) == 0 {
		return nil
	}
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
