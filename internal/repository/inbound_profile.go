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
// （已废弃：仅返回有 NodeProfileKey 记录的协议，使用 FindActiveProfilesWithKeys 代替）
func (r *InboundProfileRepository) FindActiveKeysForNode(nodeID uint) ([]entity.NodeProfileKey, error) {
	return r.FindActiveProfilesWithKeys(nodeID)
}

// FindActiveProfilesWithKeys 返回所有激活协议及其节点密钥（LEFT JOIN 语义）
// 若节点尚未为某协议配置密钥，则返回 Settings="" 的空占位记录，由上层 fallback 到协议默认值
func (r *InboundProfileRepository) FindActiveProfilesWithKeys(nodeID uint) ([]entity.NodeProfileKey, error) {
	// 1. 取所有激活协议
	var profiles []entity.InboundProfile
	if err := DB.Where("active = ?", true).Order("id asc").Find(&profiles).Error; err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}

	// 2. 取该节点已有的密钥记录
	profileIDs := make([]uint, len(profiles))
	for i, p := range profiles {
		profileIDs[i] = p.ID
	}
	var nodeKeys []entity.NodeProfileKey
	if err := DB.Where("node_id = ? AND profile_id IN ?", nodeID, profileIDs).Find(&nodeKeys).Error; err != nil {
		return nil, err
	}

	// 3. 构建 profileID → nodeKey 映射
	keyMap := make(map[uint]entity.NodeProfileKey, len(nodeKeys))
	for _, k := range nodeKeys {
		keyMap[k.ProfileID] = k
	}

	// 4. 合并：每个激活协议对应一条记录，无节点密钥时 Settings 为空（由 buildConfig 使用协议默认值）
	result := make([]entity.NodeProfileKey, 0, len(profiles))
	for i := range profiles {
		p := profiles[i]
		if k, ok := keyMap[p.ID]; ok {
			k.Profile = &p
			result = append(result, k)
		} else {
			result = append(result, entity.NodeProfileKey{
				NodeID:    nodeID,
				ProfileID: p.ID,
				Profile:   &p,
				Settings:  "",
			})
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
