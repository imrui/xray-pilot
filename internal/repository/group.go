package repository

import (
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

type GroupRepository struct{}

func NewGroupRepository() *GroupRepository {
	return &GroupRepository{}
}

func (r *GroupRepository) Create(group *entity.Group) error {
	return DB.Create(group).Error
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

func (r *GroupRepository) Update(group *entity.Group) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(group).Error; err != nil {
			return err
		}
		return InvalidateConfigs(tx, nil)
	})
}

func (r *GroupRepository) UpdateActive(id uint, active bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entity.Group{}).Where("id = ?", id).Update("active", active).Error; err != nil {
			return err
		}
		return InvalidateConfigs(tx, nil)
	})
}

func (r *GroupRepository) Delete(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
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
		return InvalidateConfigs(tx, nil)
	})
}

// ReplaceNodes 替换分组关联节点（many2many）
func (r *GroupRepository) ReplaceNodes(group *entity.Group, nodes []entity.Node) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(group).Association("Nodes").Replace(nodes); err != nil {
			return err
		}
		return InvalidateConfigs(tx, nil)
	})
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
