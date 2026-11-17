package repository

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
	return DB.Transaction(func(tx *gorm.DB) error {
		var previous entity.InboundProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&previous, p.ID).Error; err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations).Save(p).Error; err != nil {
			return err
		}
		if !previous.Active && !p.Active {
			return nil
		}
		keys := tx.Model(&entity.NodeProfileKey{}).Where("profile_id = ?", p.ID)
		if previous.Active == p.Active && previous.Protocol == p.Protocol && profileSettingsEqual(previous.Settings, p.Settings) {
			if previous.Port == p.Port {
				return nil
			}
			// A node with its own port does not follow the shared default port.
			keys = keys.Where("port = 0")
		}
		var ids []uint
		if err := keys.Distinct("node_id").Pluck("node_id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		return InvalidateConfigs(tx, ids)
	})
}

func (r *InboundProfileRepository) Delete(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var profile entity.InboundProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		var ids []uint
		if profile.Active {
			if err := tx.Model(&entity.NodeProfileKey{}).Where("profile_id = ?", id).Distinct("node_id").Pluck("node_id", &ids).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("profile_id = ?", id).Delete(&entity.NodeProfileKey{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&entity.InboundProfile{}, id).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		return InvalidateConfigs(tx, ids)
	})
}

// FindKeysForNode 查询节点关联的所有 NodeProfileKey（含 InboundProfile）
func (r *InboundProfileRepository) FindKeysForNode(nodeID uint) ([]entity.NodeProfileKey, error) {
	var keys []entity.NodeProfileKey
	err := DB.Preload("Profile").Where("node_id = ?", nodeID).Find(&keys).Error
	return keys, err
}

// FindActiveKeysForNode 查询节点显式绑定的所有激活协议
// 约定：只有存在 NodeProfileKey 记录的协议才会参与该节点的配置生成。
// 若节点希望复用协议默认参数而不做覆盖，可保存空对象 `{}` 作为 Settings。
func (r *InboundProfileRepository) FindActiveKeysForNode(nodeID uint) ([]entity.NodeProfileKey, error) {
	var keys []entity.NodeProfileKey
	err := DB.Preload("Profile", "active = ?", true).
		Where("node_id = ?", nodeID).
		Order("profile_id asc").
		Find(&keys).Error
	if err != nil {
		return nil, err
	}

	result := make([]entity.NodeProfileKey, 0, len(keys))
	for _, key := range keys {
		if key.Profile == nil || !key.Profile.Active {
			continue
		}
		result = append(result, key)
	}
	return result, nil
}

// UpsertKey 创建或更新节点密钥材料
func (r *InboundProfileRepository) UpsertKey(key *entity.NodeProfileKey) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var profile entity.InboundProfile
		if err := tx.First(&profile, key.ProfileID).Error; err != nil {
			return err
		}
		var node entity.Node
		if err := tx.Select("id").First(&node, key.NodeID).Error; err != nil {
			return err
		}
		var existing entity.NodeProfileKey
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("node_id = ? AND profile_id = ?", key.NodeID, key.ProfileID).First(&existing).Error
		changed := true
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Omit(clause.Associations).Create(key).Error; err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			if existing.Locked {
				return ErrNodeKeyLocked
			}
			existing.Profile, key.Profile = &profile, &profile
			changed = existing.EffectivePort() != key.EffectivePort() || !profileSettingsEqual(existing.Settings, key.Settings)
			existing.Settings, existing.Port = key.Settings, key.Port
			if err := tx.Omit(clause.Associations).Save(&existing).Error; err != nil {
				return err
			}
			*key = existing
		}
		if !profile.Active || !changed {
			return nil
		}
		return InvalidateConfigs(tx, []uint{key.NodeID})
	})
}

// DeleteKey 删除节点密钥
func (r *InboundProfileRepository) DeleteKey(nodeID, profileID uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing entity.NodeProfileKey
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Profile").Where("node_id = ? AND profile_id = ?", nodeID, profileID).First(&existing).Error; err != nil {
			return err
		}
		if existing.Locked {
			return ErrNodeKeyLocked
		}
		if err := tx.Where("node_id = ? AND profile_id = ?", nodeID, profileID).Delete(&entity.NodeProfileKey{}).Error; err != nil {
			return err
		}
		if existing.Profile == nil || !existing.Profile.Active {
			return nil
		}
		return InvalidateConfigs(tx, []uint{nodeID})
	})
}

// Saving the same Reality private key produces a new AES nonce. Compare its
// plaintext value in memory so an unchanged form does not invalidate the node.
func profileSettingsEqual(a, b string) bool {
	decode := func(raw string) (map[string]any, error) {
		if raw == "" {
			raw = "{}"
		}
		var value map[string]any
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		if key, ok := value["private_key"].(string); ok && key != "" {
			if plain, err := crypto.Decrypt(key); err == nil {
				value["private_key"] = plain
			}
		}
		return value, nil
	}
	av, ae := decode(a)
	bv, be := decode(b)
	if ae != nil || be != nil {
		return a == b
	}
	return reflect.DeepEqual(av, bv)
}

// SetKeyLocked 更新节点协议锁定状态
func (r *InboundProfileRepository) SetKeyLocked(nodeID, profileID uint, locked bool) error {
	return DB.Model(&entity.NodeProfileKey{}).
		Where("node_id = ? AND profile_id = ?", nodeID, profileID).
		Update("locked", locked).Error
}
