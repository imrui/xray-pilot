package repository

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

func preloadUserGroups(db *gorm.DB) *gorm.DB {
	return db.Preload("Groups", func(tx *gorm.DB) *gorm.DB {
		return tx.Order("groups.id asc")
	})
}

func (r *UserRepository) Create(user *entity.User) error {
	return DB.Create(user).Error
}

func (r *UserRepository) ReplaceGroups(user *entity.User, groupIDs []uint) error {
	return r.ReplaceGroupsTx(DB, user, groupIDs)
}

func (r *UserRepository) ReplaceGroupsTx(tx *gorm.DB, user *entity.User, groupIDs []uint) error {
	groupIDs = uniqueSortedIDs(groupIDs)
	groups, err := r.findGroupsByIDsTx(tx, groupIDs)
	if err != nil {
		return err
	}
	return tx.Model(user).Association("Groups").Replace(groups)
}

func (r *UserRepository) FindByID(id uint) (*entity.User, error) {
	var user entity.User
	err := preloadUserGroups(DB).First(&user, id).Error
	return &user, err
}

func (r *UserRepository) FindByToken(token string) (*entity.User, error) {
	var user entity.User
	err := preloadUserGroups(DB).Where("token = ?", token).First(&user).Error
	return &user, err
}

func (r *UserRepository) FindByFeishuIdentity(openID, unionID string) (*entity.User, error) {
	var user entity.User
	query := preloadUserGroups(DB)

	switch {
	case openID != "" && unionID != "":
		err := query.Where("feishu_open_id = ? OR feishu_union_id = ?", openID, unionID).Limit(1).Find(&user).Error
		if err != nil {
			return nil, err
		}
		if user.ID == 0 {
			return nil, nil
		}
		return &user, nil
	case openID != "":
		err := query.Where("feishu_open_id = ?", openID).Limit(1).Find(&user).Error
		if err != nil {
			return nil, err
		}
		if user.ID == 0 {
			return nil, nil
		}
		return &user, nil
	case unionID != "":
		err := query.Where("feishu_union_id = ?", unionID).Limit(1).Find(&user).Error
		if err != nil {
			return nil, err
		}
		if user.ID == 0 {
			return nil, nil
		}
		return &user, nil
	default:
		return nil, errors.New("missing feishu identity")
	}
}

func (r *UserRepository) FindByFeishuEmail(email string) (*entity.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, errors.New("missing feishu email")
	}

	var users []entity.User
	err := preloadUserGroups(DB).Where("LOWER(TRIM(feishu_email)) = ?", email).Limit(2).Find(&users).Error
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errors.New("user not found")
	}
	if len(users) > 1 {
		return nil, fmt.Errorf("multiple users matched feishu email: %s", email)
	}
	return &users[0], nil
}

// FindAll 不分页返回所有用户（流量统计场景下需要构建完整的 username→ID 映射）
// 注意：当前不包含 Groups 预加载，避免无意义的 join
func (r *UserRepository) FindAll() ([]entity.User, error) {
	var users []entity.User
	err := DB.Order("id asc").Find(&users).Error
	return users, err
}

func (r *UserRepository) List(page, pageSize int) ([]entity.User, int64, error) {
	var total int64
	if err := DB.Model(&entity.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []entity.User
	offset := (page - 1) * pageSize
	err := preloadUserGroups(DB).Order("id desc").Offset(offset).Limit(pageSize).Find(&users).Error
	return users, total, err
}

func (r *UserRepository) FindByIDs(ids []uint) ([]entity.User, error) {
	if len(ids) == 0 {
		return []entity.User{}, nil
	}
	var users []entity.User
	err := preloadUserGroups(DB).Where("id IN ?", ids).Find(&users).Error
	return users, err
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
	err := preloadUserGroups(DB).
		Distinct("users.*").
		Joins("JOIN user_groups ug ON ug.user_id = users.id").
		Where("ug.group_id = ? AND users.active = ?", groupID, true).
		Order("users.id asc").
		Find(&users).Error
	return users, err
}

// FindActiveUsersByNodeID 查询某节点所在分组的所有激活用户（未过期）
func (r *UserRepository) FindActiveUsersByNodeID(nodeID uint) ([]entity.User, error) {
	var users []entity.User
	now := time.Now()
	err := preloadUserGroups(DB).
		Distinct("users.*").
		Joins("JOIN user_groups ug ON ug.user_id = users.id").
		Joins("JOIN group_nodes gn ON gn.group_id = ug.group_id").
		Where(
			"users.active = ? AND (users.expires_at IS NULL OR users.expires_at > ?) AND gn.node_id = ?",
			true,
			now,
			nodeID,
		).
		Order("users.id asc").
		Find(&users).Error
	return users, err
}

// GetExpiredUsers 查询已过期的用户
func (r *UserRepository) GetExpiredUsers() ([]entity.User, error) {
	var users []entity.User
	now := time.Now()
	err := preloadUserGroups(DB).Where("active = ? AND expires_at IS NOT NULL AND expires_at <= ?", true, now).Find(&users).Error
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
		Joins("JOIN user_groups ug ON ug.user_id = users.id").
		Joins("JOIN group_nodes gn ON gn.group_id = ug.group_id").
		Where(
			"users.active = ? AND (users.expires_at IS NULL OR users.expires_at > ?) AND gn.node_id = ?",
			true,
			now,
			nodeID,
		).
		Distinct("users.id").
		Count(&total).Error
	return total, err
}

func (r *UserRepository) findGroupsByIDsTx(tx *gorm.DB, groupIDs []uint) ([]entity.Group, error) {
	if len(groupIDs) == 0 {
		return []entity.Group{}, nil
	}

	var groups []entity.Group
	if err := tx.Where("id IN ?", groupIDs).Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) != len(groupIDs) {
		found := make([]uint, 0, len(groups))
		for _, group := range groups {
			found = append(found, group.ID)
		}
		missing := make([]uint, 0)
		for _, id := range groupIDs {
			if !slices.Contains(found, id) {
				missing = append(missing, id)
			}
		}
		return nil, fmt.Errorf("分组不存在: %v", missing)
	}

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].ID < groups[j].ID
	})
	return groups, nil
}

func uniqueSortedIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	cloned := append([]uint(nil), ids...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i] < cloned[j] })
	result := cloned[:0]
	var prev uint
	for i, id := range cloned {
		if i == 0 || id != prev {
			result = append(result, id)
			prev = id
		}
	}
	return result
}
