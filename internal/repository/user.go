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
	"gorm.io/gorm/clause"
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
	_, err := r.ChangeAccess(user.ID, func(tx *gorm.DB, current *entity.User) error { return r.ReplaceGroupsTx(tx, current, groupIDs) })
	return err
}

func (r *UserRepository) ReplaceGroupsTx(tx *gorm.DB, user *entity.User, groupIDs []uint) error {
	groupIDs = uniqueSortedIDs(groupIDs)
	groups, err := r.findGroupsByIDsTx(tx, groupIDs)
	if err != nil {
		return err
	}
	if user.LegacyGroupID != nil {
		if err := tx.Model(user).UpdateColumn("group_id", nil).Error; err != nil {
			return err
		}
		user.LegacyGroupID = nil
	}
	return tx.Omit("Groups.*").Model(user).Association("Groups").Replace(groups)
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

type UserListFilter struct {
	Keyword string
	Status  string
	GroupID uint
}

func (r *UserRepository) List(page, pageSize int, filter UserListFilter) ([]entity.User, int64, error) {
	query := DB.Model(&entity.User{})
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		pattern := "%" + escapeLike(strings.ToLower(keyword)) + "%"
		query = query.Where(`(LOWER(users.username) LIKE ? ESCAPE '\' OR LOWER(users.real_name) LIKE ? ESCAPE '\'
			OR LOWER(users.feishu_email) LIKE ? ESCAPE '\' OR LOWER(users.remark) LIKE ? ESCAPE '\'
			OR EXISTS (SELECT 1 FROM user_groups ug JOIN groups g ON g.id = ug.group_id
				WHERE ug.user_id = users.id AND LOWER(g.name) LIKE ? ESCAPE '\'))`,
			pattern, pattern, pattern, pattern, pattern)
	}
	if filter.GroupID != 0 {
		query = query.Where("EXISTS (SELECT 1 FROM user_groups ug WHERE ug.user_id = users.id AND ug.group_id = ?)", filter.GroupID)
	}
	switch filter.Status {
	case "active":
		query = query.Where("active = ? AND (expires_at IS NULL OR "+expiryInstant(DB, "expires_at")+" > ?)", true, expiryBound(DB, time.Now()))
	case "inactive":
		query = query.Where("active = ?", false)
	case "expired":
		query = query.Where(expiryInstant(DB, "expires_at")+" <= ?", expiryBound(DB, time.Now()))
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []entity.User
	offset := (page - 1) * pageSize
	err := preloadUserGroups(query).Order("id desc").Offset(offset).Limit(pageSize).Find(&users).Error
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

func (r *UserRepository) UpdateFeishu(user *entity.User) error {
	return DB.Model(user).Select("FeishuEnabled", "FeishuEmail", "FeishuOpenID", "FeishuUnionID", "FeishuChatID", "FeishuBoundAt").Updates(user).Error
}

func nodeDifference(ids, exclude []uint) []uint {
	seen := make(map[uint]bool, len(exclude))
	for _, id := range exclude {
		seen[id] = true
	}
	var diff []uint
	for _, id := range ids {
		if !seen[id] {
			diff = append(diff, id)
		}
	}
	return diff
}

// UserAccessChange 包含事务提交后仍需尝试热更新的节点。
type UserAccessChange struct {
	User     *entity.User
	Username string
	Renamed  bool
	NodeIDs  []uint
}

// ChangeAccess 在同一事务内读取变更前后的有效授权。普通字段编辑仅写回请求指定列。
func (r *UserRepository) ChangeAccess(id uint, edit func(*gorm.DB, *entity.User) error) (UserAccessChange, error) {
	var result UserAccessChange
	err := AuthorizationTransaction(func(tx *gorm.DB) error {
		var old entity.User
		if err := preloadUserGroups(tx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, id).Error; err != nil {
			return err
		}
		now := time.Now()
		oldNodes, err := UserConfigNodes(tx, id)
		if err != nil {
			return err
		}
		var potential, expiryNodes []uint
		if old.EffectiveActive(now) || dueForExpiry(&old, now) {
			// 移除最后一个用户入站后，未确认的旧磁盘或运行态仍可能包含这个账号。
			potential, err = userConfigNodes(tx, id, true)
			if err != nil {
				return err
			}
		}
		if dueForExpiry(&old, now) {
			if err := queueExpiry(tx, id, potential); err != nil {
				return err
			}
			expiryNodes = oldNodes
		}
		result.Username = old.Username
		if !old.EffectiveActive(now) {
			oldNodes = nil
		}
		current := old
		if err := edit(tx, &current); err != nil {
			return err
		}
		var next entity.User
		var nextNodes []uint
		err = preloadUserGroups(tx).First(&next, id).Error
		switch {
		case err == nil:
			result.User = &next
			result.Renamed = old.Username != next.Username
			if next.EffectiveActive(now) {
				nextNodes, err = UserConfigNodes(tx, id)
				if err != nil {
					return err
				}
				// 续期只取消重新获得授权的节点，移组前未确认的旧节点继续撤权。
				if len(nextNodes) > 0 {
					if err := tx.Where("user_id = ? AND node_id IN ?", id, nextNodes).Delete(&entity.UserExpiryTarget{}).Error; err != nil {
						return err
					}
				}
			} else if dueForExpiry(&next, now) {
				// 一次请求可以同时移组并改为过期，旧授权才需要撤销。
				if err := queueExpiry(tx, id, potential); err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 删除账号后仍可凭独立的节点待办确认历史撤权。
		default:
			return err
		}
		removed := nodeDifference(oldNodes, nextNodes)
		if result.User != nil && next.ExpiresAt != nil && next.ExpiresAt.After(now) {
			var pairs []expiryMembership
			for _, n := range nodeDifference(potential, nextNodes) {
				pairs = append(pairs, expiryMembership{id, n})
			}
			if err := rememberExpiryDepartures(tx, pairs); err != nil {
				return err
			}
		} else if result.User == nil || next.ExpiresAt == nil {
			if err := tx.Where("user_id = ? AND deferred = ?", id, true).Delete(&entity.UserExpiryTarget{}).Error; err != nil {
				return err
			}
		}
		if result.Renamed || (result.User != nil && old.UUID != next.UUID) {
			result.NodeIDs = append(result.NodeIDs, oldNodes...)
			result.NodeIDs = append(result.NodeIDs, nextNodes...)
		} else {
			result.NodeIDs = append(removed, nodeDifference(nextNodes, oldNodes)...)
		}
		result.NodeIDs = uniqueSortedIDs(result.NodeIDs)
		affected := append(expiryNodes, result.NodeIDs...)
		if len(affected) > 0 {
			return InvalidateConfigs(tx, affected)
		}
		return nil
	})
	return result, err
}

func (r *UserRepository) UpdateActive(id uint, active bool) error {
	_, err := r.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		return tx.Model(&entity.User{}).Where("id = ?", u.ID).Update("active", active).Error
	})
	return err
}

func (r *UserRepository) DeleteAccess(id uint) (UserAccessChange, error) {
	change, err := r.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		if err := tx.Where("user_id = ?", id).Delete(&entity.UserGroup{}).Error; err != nil {
			return err
		}
		return tx.Delete(u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserAccessChange{}, nil
	}
	return change, err
}

func (r *UserRepository) Delete(id uint) error {
	_, err := r.DeleteAccess(id)
	return err
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
		Joins("JOIN groups g ON g.id = ug.group_id AND g.active = ?", true).
		Where(
			"users.active = ? AND (users.expires_at IS NULL OR "+expiryInstant(DB, "users.expires_at")+" > ?) AND gn.node_id = ?",
			true,
			expiryBound(DB, now),
			nodeID,
		).
		Order("users.id asc").
		Find(&users).Error
	return users, err
}

// Count 返回用户总数
func (r *UserRepository) Count() (int64, error) {
	var total int64
	return total, DB.Model(&entity.User{}).Count(&total).Error
}

// UpdateUUID 更新用户 UUID（重置订阅身份）
func (r *UserRepository) UpdateUUID(id uint, newUUID string) error {
	_, err := r.ChangeAccess(id, func(tx *gorm.DB, u *entity.User) error {
		return tx.Model(&entity.User{}).Where("id = ?", u.ID).Update("uuid", newUUID).Error
	})
	return err
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
		Joins("JOIN groups g ON g.id = ug.group_id AND g.active = ?", true).
		Where(
			"users.active = ? AND (users.expires_at IS NULL OR "+expiryInstant(DB, "users.expires_at")+" > ?) AND gn.node_id = ?",
			true,
			expiryBound(DB, now),
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
