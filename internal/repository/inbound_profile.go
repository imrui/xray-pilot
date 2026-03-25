package repository

import "github.com/imrui/xray-pilot/internal/entity"

type InboundProfileRepository struct{}

func NewInboundProfileRepository() *InboundProfileRepository {
	return &InboundProfileRepository{}
}

func (r *InboundProfileRepository) Create(p *entity.InboundProfile) error {
	return DB.Create(p).Error
}

func (r *InboundProfileRepository) FindByID(id uint) (*entity.InboundProfile, error) {
	var p entity.InboundProfile
	err := DB.First(&p, id).Error
	return &p, err
}

func (r *InboundProfileRepository) List(page, pageSize int) ([]entity.InboundProfile, int64, error) {
	var total int64
	if err := DB.Model(&entity.InboundProfile{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var profiles []entity.InboundProfile
	offset := (page - 1) * pageSize
	err := DB.Order("id asc").Offset(offset).Limit(pageSize).Find(&profiles).Error
	return profiles, total, err
}

func (r *InboundProfileRepository) FindAll() ([]entity.InboundProfile, error) {
	var profiles []entity.InboundProfile
	err := DB.Where("active = ?", true).Order("id asc").Find(&profiles).Error
	return profiles, err
}

func (r *InboundProfileRepository) Update(p *entity.InboundProfile) error {
	return DB.Save(p).Error
}

func (r *InboundProfileRepository) Delete(id uint) error {
	return DB.Delete(&entity.InboundProfile{}, id).Error
}

// FindKeysForNode 查询节点关联的所有 NodeProfileKey（含 InboundProfile）
func (r *InboundProfileRepository) FindKeysForNode(nodeID uint) ([]entity.NodeProfileKey, error) {
	var keys []entity.NodeProfileKey
	err := DB.Preload("Profile").Where("node_id = ?", nodeID).Find(&keys).Error
	return keys, err
}

// FindActiveKeysForNode 查询节点关联的所有激活 InboundProfile 的 NodeProfileKey
func (r *InboundProfileRepository) FindActiveKeysForNode(nodeID uint) ([]entity.NodeProfileKey, error) {
	var keys []entity.NodeProfileKey
	err := DB.Preload("Profile").
		Where("node_id = ?", nodeID).
		Find(&keys).Error
	if err != nil {
		return nil, err
	}
	// 过滤掉未激活的 Profile
	result := make([]entity.NodeProfileKey, 0, len(keys))
	for _, k := range keys {
		if k.Profile != nil && k.Profile.Active {
			result = append(result, k)
		}
	}
	return result, nil
}

// UpsertKey 创建或更新节点密钥材料
func (r *InboundProfileRepository) UpsertKey(key *entity.NodeProfileKey) error {
	var existing entity.NodeProfileKey
	err := DB.Where("node_id = ? AND profile_id = ?", key.NodeID, key.ProfileID).First(&existing).Error
	if err != nil {
		// 不存在则创建
		return DB.Create(key).Error
	}
	existing.Settings = key.Settings
	return DB.Save(&existing).Error
}

// DeleteKey 删除节点密钥
func (r *InboundProfileRepository) DeleteKey(nodeID, profileID uint) error {
	return DB.Where("node_id = ? AND profile_id = ?", nodeID, profileID).
		Delete(&entity.NodeProfileKey{}).Error
}
