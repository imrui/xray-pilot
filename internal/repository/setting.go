package repository

import "github.com/imrui/xray-pilot/internal/entity"

type SettingRepository struct{}

func NewSettingRepository() *SettingRepository { return &SettingRepository{} }

// Get 获取单个配置值，第二个返回值表示是否存在
func (r *SettingRepository) Get(key string) (string, bool) {
	var s entity.SystemSetting
	if err := DB.First(&s, "key = ?", key).Error; err != nil {
		return "", false
	}
	return s.Value, true
}

// Set 写入或覆盖配置值
func (r *SettingRepository) Set(key, value string) error {
	return DB.Save(&entity.SystemSetting{Key: key, Value: value}).Error
}

// SetIfAbsent 仅在 key 不存在时写入（用于首次启动种子数据）
func (r *SettingRepository) SetIfAbsent(key, value string) error {
	var count int64
	DB.Model(&entity.SystemSetting{}).Where("key = ?", key).Count(&count)
	if count > 0 {
		return nil
	}
	return DB.Create(&entity.SystemSetting{Key: key, Value: value}).Error
}

// GetAll 获取所有配置项
func (r *SettingRepository) GetAll() (map[string]string, error) {
	var settings []entity.SystemSetting
	if err := DB.Find(&settings).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}

// BatchSet 批量写入
func (r *SettingRepository) BatchSet(kv map[string]string) error {
	for k, v := range kv {
		if err := r.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}
