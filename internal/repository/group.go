package repository

import (
	"github.com/imrui/xray-pilot/internal/entity"
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
	return DB.Save(group).Error
}

func (r *GroupRepository) UpdateActive(id uint, active bool) error {
	return DB.Model(&entity.Group{}).Where("id = ?", id).Update("active", active).Error
}

func (r *GroupRepository) Delete(id uint) error {
	return DB.Delete(&entity.Group{}, id).Error
}

// ReplaceNodes 替换分组关联节点（many2many）
func (r *GroupRepository) ReplaceNodes(group *entity.Group, nodes []entity.Node) error {
	return DB.Model(group).Association("Nodes").Replace(nodes)
}
