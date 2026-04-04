package entity

import "time"

// User 订阅用户实体（VPN 用户，不含管理员账号）
// 管理员账号由 config.yaml 的 admins 字段管理
type User struct {
	ID            uint   `gorm:"primaryKey"`
	Username      string `gorm:"uniqueIndex;not null"`
	RealName      string
	UUID          string     `gorm:"uniqueIndex;not null"` // VLESS UUID
	Token         string     `gorm:"uniqueIndex;not null"` // 订阅 Token
	LegacyGroupID *uint      `gorm:"column:group_id"`
	Groups        []Group    `gorm:"many2many:user_groups"`
	Active        bool       `gorm:"default:true"`
	ExpiresAt     *time.Time // nil 表示永久有效
	Remark        string
	FeishuEnabled bool
	FeishuEmail   string
	FeishuOpenID  string
	FeishuUnionID string
	FeishuChatID  string
	FeishuBoundAt *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type UserGroup struct {
	UserID  uint `gorm:"primaryKey"`
	GroupID uint `gorm:"primaryKey"`
}
