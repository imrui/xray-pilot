package entity

import "time"

// User 订阅用户实体（VPN 用户，不含管理员账号）
// 管理员账号由 config.yaml 的 admins 字段管理
type User struct {
	ID                  uint    `gorm:"primaryKey"`
	RelaySubscriptionID *string `gorm:"uniqueIndex"`
	Username            string  `gorm:"uniqueIndex;not null"`
	RealName            string
	UUID                string     `gorm:"uniqueIndex;not null"` // VLESS UUID
	Token               string     `gorm:"uniqueIndex;not null"` // 订阅 Token
	LegacyGroupID       *uint      `gorm:"column:group_id"`
	Groups              []Group    `gorm:"many2many:user_groups"`
	Active              bool       `gorm:"default:true"`
	ExpiresAt           *time.Time // nil 表示永久有效
	// 过期摘除打标：调度器把过期用户从节点运行时摘除后写入，避免每周期重复扫；
	// ExpiresAt 变更（续期）时清空，使续期后的再次过期能被重新处理。
	ExpiredSweptAt *time.Time
	ExpiryQueued   bool `gorm:"not null;default:false" json:"-"`
	Remark         string
	FeishuEnabled  bool
	FeishuEmail    string
	FeishuOpenID   string
	FeishuUnionID  string
	FeishuChatID   string
	FeishuBoundAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsExpired 用户是否已过期（nil 永久有效）
func (u *User) IsExpired(now time.Time) bool {
	return u.ExpiresAt != nil && !u.ExpiresAt.After(now)
}

// EffectiveActive 用户是否应出现在节点运行时：手动启用且未过期。
// 配置生成（FindActiveUsersByNodeID）与 live-apply 的期望集必须共用此语义，否则两侧会漂移。
func (u *User) EffectiveActive(now time.Time) bool {
	return u.Active && !u.IsExpired(now)
}

type UserGroup struct {
	UserID  uint `gorm:"primaryKey"`
	GroupID uint `gorm:"primaryKey"`
}

// UserExpiryTarget 在分组改变后仍保留撤权义务。Ticket 防止续期前的回执确认下一次到期。
type UserExpiryTarget struct {
	Ticket   string `gorm:"primaryKey;size:36"`
	UserID   uint   `gorm:"uniqueIndex:idx_user_expiry_node"`
	NodeID   uint   `gorm:"uniqueIndex:idx_user_expiry_node;index"`
	Deferred bool   `gorm:"not null;default:false"` // 到期前移组，等待正常同步回执或到期时确认。
}
