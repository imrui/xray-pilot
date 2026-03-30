package repository

import (
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

func (r *UserRepository) Create(user *entity.User) error {
	return DB.Create(user).Error
}

func (r *UserRepository) FindByID(id uint) (*entity.User, error) {
	var user entity.User
	err := DB.Preload("Group").First(&user, id).Error
	return &user, err
}

func (r *UserRepository) FindByToken(token string) (*entity.User, error) {
	var user entity.User
	err := DB.Preload("Group").Where("token = ?", token).First(&user).Error
	return &user, err
}

func (r *UserRepository) List(page, pageSize int) ([]entity.User, int64, error) {
	var total int64
	if err := DB.Model(&entity.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []entity.User
	offset := (page - 1) * pageSize
	err := DB.Preload("Group").Order("id desc").Offset(offset).Limit(pageSize).Find(&users).Error
	return users, total, err
}

func (r *UserRepository) Update(user *entity.User) error {
	return DB.Save(user).Error
}

func (r *UserRepository) UpdateActive(id uint, active bool) error {
	return DB.Model(&entity.User{}).Where("id = ?", id).Update("active", active).Error
}

func (r *UserRepository) Delete(id uint) error {
	return DB.Delete(&entity.User{}, id).Error
}

func (r *UserRepository) FindActiveByGroupID(groupID uint) ([]entity.User, error) {
	var users []entity.User
	err := DB.Where("group_id = ? AND active = ?", groupID, true).Order("id asc").Find(&users).Error
	return users, err
}

// FindActiveUsersByNodeID 查询某节点所在分组的所有激活用户（未过期）
func (r *UserRepository) FindActiveUsersByNodeID(nodeID uint) ([]entity.User, error) {
	var users []entity.User
	now := time.Now()
	err := DB.Where(
		"active = ? AND (expires_at IS NULL OR expires_at > ?) AND group_id IN (?)",
		true,
		now,
		DB.Table("group_nodes").Select("group_id").Where("node_id = ?", nodeID),
	).Order("id asc").Find(&users).Error
	return users, err
}

// GetExpiredUsers 查询已过期的用户
func (r *UserRepository) GetExpiredUsers() ([]entity.User, error) {
	var users []entity.User
	now := time.Now()
	err := DB.Where("active = ? AND expires_at IS NOT NULL AND expires_at <= ?", true, now).Find(&users).Error
	return users, err
}

// Count 返回用户总数
func (r *UserRepository) Count() (int64, error) {
	var total int64
	return total, DB.Model(&entity.User{}).Count(&total).Error
}

// UpdateUUID 更新用户 UUID（重置订阅身份）
func (r *UserRepository) UpdateUUID(id uint, newUUID string) error {
	return DB.Model(&entity.User{}).Where("id = ?", id).Update("uuid", newUUID).Error
}

// UpdateToken 更新用户订阅 Token
func (r *UserRepository) UpdateToken(id uint, newToken string) error {
	return DB.Model(&entity.User{}).Where("id = ?", id).Update("token", newToken).Error
}

func (r *UserRepository) CountActiveByNodeID(nodeID uint) (int64, error) {
	var total int64
	now := time.Now()
	err := DB.Model(&entity.User{}).
		Where(
			"active = ? AND (expires_at IS NULL OR expires_at > ?) AND group_id IN (?)",
			true,
			now,
			DB.Table("group_nodes").Select("group_id").Where("node_id = ?", nodeID),
		).
		Count(&total).Error
	return total, err
}
