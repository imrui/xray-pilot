package repository

import (
	"time"

	"github.com/google/uuid"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PostgreSQL 的 READ COMMITTED 不能固定跨表的成员关系。在短事务内串行化授权变更，
// 使分组编辑、用户编辑和到期登记观察同一份关系。SQLite 使用仓储的单连接。
func AuthorizationTransaction(fn func(*gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", 910302).Error; err != nil {
				return err
			}
		}
		return fn(tx)
	})
}

// SQLite 把带时区的时间保存为文本，直接比较会按当地钟点排序。
func expiryInstant(db *gorm.DB, expression string) string {
	if db.Dialector.Name() == "sqlite" {
		return sqliteInstant(expression)
	}
	return expression
}

func expiryBound(db *gorm.DB, now time.Time) any {
	if db.Dialector.Name() == "sqlite" {
		return now.UnixNano()
	}
	return now
}

func dueExpiries(tx *gorm.DB, now time.Time) *gorm.DB {
	return tx.Model(&entity.User{}).Where("active = ? AND expired_swept_at IS NULL AND expiry_queued = ?", true, false).
		Where(expiryInstant(tx, "expires_at")+" <= ?", expiryBound(tx, now))
}

func userNodeMembership(tx *gorm.DB) *gorm.DB {
	return tx.Table("user_groups AS ug").
		Joins("JOIN groups g ON g.id = ug.group_id AND g.active = ?", true).
		Joins("JOIN group_nodes gn ON gn.group_id = ug.group_id")
}

// 只有这些入站消费订阅用户。撤权还要覆盖已移除入站、旧配置尚待恢复的节点。
func configUserMembership(tx *gorm.DB, includePending bool) *gorm.DB {
	profiles := tx.Table("node_profile_keys AS k").Select("1").Joins("JOIN inbound_profiles p ON p.id = k.profile_id").
		Where("k.node_id = gn.node_id AND p.active = ? AND p.protocol IN ?", true, []string{types.ProtocolVlessReality, types.ProtocolVlessWSTLS, types.ProtocolTrojan})
	q := userNodeMembership(tx)
	if includePending {
		pending := tx.Model(&entity.NodeConfigState{}).Select("1").Where("node_id = gn.node_id AND recovery_pending = ?", true)
		return q.Where("(EXISTS (?) OR EXISTS (?))", profiles, pending)
	}
	return q.Where("EXISTS (?)", profiles)
}

func UserConfigNodes(tx *gorm.DB, userID uint) ([]uint, error) {
	return userConfigNodes(tx, userID, false)
}

func userConfigNodes(tx *gorm.DB, userID uint, includePending bool) ([]uint, error) {
	var ids []uint
	err := configUserMembership(tx, includePending).Where("ug.user_id = ?", userID).Distinct("gn.node_id").Order("gn.node_id").Pluck("gn.node_id", &ids).Error
	return ids, err
}

type expiryMembership struct{ UserID, NodeID uint }

// 到期前撤销但尚未下发的授权，也可能在到期时仍留在旧节点。
func rememberExpiryDepartures(tx *gorm.DB, pairs []expiryMembership) error {
	if len(pairs) == 0 {
		return nil
	}
	rows := make([]entity.UserExpiryTarget, 0, len(pairs))
	for _, p := range pairs {
		rows = append(rows, entity.UserExpiryTarget{Ticket: uuid.NewString(), UserID: p.UserID, NodeID: p.NodeID, Deferred: true})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&rows, 100).Error
}

func rememberGroupDepartures(tx *gorm.DB, groupID uint, before, after []uint, wasActive, active bool) error {
	if !wasActive {
		return nil
	}
	if !active {
		after = nil
	}
	leaving := nodeDifference(before, after)
	if len(leaving) == 0 {
		return nil
	}
	var pairs []expiryMembership
	other := userNodeMembership(tx).Select("1").Where("ug.user_id = u.id AND gn.node_id = removed.node_id AND ug.group_id <> ?", groupID)
	relevant := configUserMembership(tx, true).Select("gn.node_id").Where("ug.user_id = u.id")
	err := tx.Table("user_groups AS members").Joins("JOIN users u ON u.id = members.user_id").
		Joins("JOIN group_nodes removed ON removed.group_id = members.group_id").
		Select("u.id AS user_id, removed.node_id").Where("members.group_id = ? AND removed.node_id IN ? AND u.active = ? AND u.expires_at IS NOT NULL", groupID, leaving, true).
		Where("u.expired_swept_at IS NULL AND u.expiry_queued = ?", false).
		Where("removed.node_id IN (?) AND NOT EXISTS (?)", relevant, other).Scan(&pairs).Error
	if err != nil {
		return err
	}
	return rememberExpiryDepartures(tx, pairs)
}

// 分组重新启用或新增节点，可能重新授予续期用户权限，旧票据随事务取消。
func cancelRegrantedGroupTargets(tx *gorm.DB, groupID uint, nodes []uint) error {
	if len(nodes) == 0 {
		return nil
	}
	members := configUserMembership(tx, false).Select("1").Joins("JOIN users u ON u.id = ug.user_id").
		Where("ug.group_id = ? AND ug.user_id = user_expiry_targets.user_id AND gn.node_id = user_expiry_targets.node_id AND u.active = ?", groupID, true).
		Where("u.expires_at IS NULL OR "+expiryInstant(tx, "u.expires_at")+" > ?", expiryBound(tx, time.Now()))
	return tx.Where("node_id IN ? AND EXISTS (?)", nodes, members).Delete(&entity.UserExpiryTarget{}).Error
}

// 在修改旧成员关系之前登记一次。重复扫描仅消费已登记的 Ticket。
func queueExpiry(tx *gorm.DB, userID uint, nodes []uint) error {
	pairs := make([]expiryMembership, 0, len(nodes))
	for _, id := range nodes {
		pairs = append(pairs, expiryMembership{userID, id})
	}
	return queueExpiries(tx, []uint{userID}, pairs)
}

func queueExpiries(tx *gorm.DB, users []uint, pairs []expiryMembership) error {
	for len(users) > 0 {
		n := min(len(users), 500)
		if err := tx.Model(&entity.User{}).Where("id IN ?", users[:n]).Update("expiry_queued", true).Error; err != nil {
			return err
		}
		if err := tx.Model(&entity.UserExpiryTarget{}).Where("user_id IN ? AND deferred = ?", users[:n], true).Update("deferred", false).Error; err != nil {
			return err
		}
		users = users[n:]
	}
	if len(pairs) == 0 {
		return nil
	}
	targets := make([]entity.UserExpiryTarget, 0, len(pairs))
	for _, p := range pairs {
		targets = append(targets, entity.UserExpiryTarget{Ticket: uuid.NewString(), UserID: p.UserID, NodeID: p.NodeID})
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"ticket", "deferred"}),
	}).CreateInBatches(&targets, 100).Error
}

func dueForExpiry(u *entity.User, now time.Time) bool {
	return u.Active && u.IsExpired(now) && u.ExpiredSweptAt == nil && !u.ExpiryQueued
}

func registerDueExpiries(tx *gorm.DB, now time.Time, groupID uint) ([]uint, error) {
	due := dueExpiries(tx, now)
	if groupID != 0 {
		due = due.Where("id IN (?)", tx.Table("user_groups").Select("user_id").Where("group_id = ?", groupID))
	}
	var users []uint
	if err := due.Session(&gorm.Session{}).Order("id").Pluck("id", &users).Error; err != nil || len(users) == 0 {
		return nil, err
	}
	var pairs []expiryMembership
	if err := configUserMembership(tx, true).Where("ug.user_id IN (?)", due.Select("id")).Distinct("ug.user_id, gn.node_id").Order("ug.user_id, gn.node_id").Scan(&pairs).Error; err != nil {
		return nil, err
	}
	// 仅待恢复的旧入站已有配置修订；保留它的撤权义务和现有重试进度。
	var affected []uint
	if err := configUserMembership(tx, false).Where("ug.user_id IN (?)", due.Select("id")).Distinct("gn.node_id").Pluck("gn.node_id", &affected).Error; err != nil {
		return nil, err
	}
	return affected, queueExpiries(tx, users, pairs)
}

// 只有属于当前登记、所有节点都已确认的到期用户才返回给调度器。
func finishExpiries(tx *gorm.DB, now time.Time) ([]string, error) {
	ready := tx.Model(&entity.User{}).Where("expiry_queued = ? AND expired_swept_at IS NULL", true).
		Where(expiryInstant(tx, "expires_at")+" <= ?", expiryBound(tx, now)).
		Where("NOT EXISTS (SELECT 1 FROM user_expiry_targets t WHERE t.user_id = users.id)")
	var names []string
	if err := ready.Session(&gorm.Session{}).Order("id").Pluck("username", &names).Error; err != nil || len(names) == 0 {
		return nil, err
	}
	return names, ready.Updates(map[string]any{"expired_swept_at": now, "expiry_queued": false}).Error
}

func PrepareExpirySweep(now time.Time) (targets []entity.UserExpiryTarget, swept []string, err error) {
	err = AuthorizationTransaction(func(tx *gorm.DB) error {
		ids, e := registerDueExpiries(tx, now, 0)
		if e != nil {
			return e
		}
		if len(ids) > 0 {
			if e = InvalidateConfigs(tx, ids); e != nil {
				return e
			}
		}
		if swept, e = finishExpiries(tx, now); e != nil {
			return e
		}
		return tx.Where("deferred = ?", false).Order("node_id, user_id").Find(&targets).Error
	})
	return
}

func ConfirmExpiryTargets(tickets []string, now time.Time) ([]string, error) {
	var swept []string
	err := AuthorizationTransaction(func(tx *gorm.DB) error {
		// 分批避免集中到期时超过 SQLite / PostgreSQL 的绑定参数上限。
		for len(tickets) > 0 {
			n := min(len(tickets), 500)
			if err := tx.Where("ticket IN ?", tickets[:n]).Delete(&entity.UserExpiryTarget{}).Error; err != nil {
				return err
			}
			tickets = tickets[n:]
		}
		var e error
		swept, e = finishExpiries(tx, now)
		return e
	})
	return swept, err
}
