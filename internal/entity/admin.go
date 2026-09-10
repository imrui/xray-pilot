package entity

import "time"

// 管理员角色（v0.5.0 两级，不做 RBAC）
const (
	// RoleSuperAdmin 超级管理员：全部业务操作 + 管理员账号管理
	RoleSuperAdmin = "super_admin"
	// RoleAdmin 普通管理员：全部业务操作，仅可改自己密码
	RoleAdmin = "admin"
)

// AdminUser 面板管理员账号（v0.5.0 起入库；config.yaml admins 仅作首次启动种子）
//
// 与订阅用户 User 完全独立：User 无密码、通过 token 匿名取订阅；AdminUser 有密码、登录面板。
type AdminUser struct {
	ID           uint       `gorm:"primaryKey"                    json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null"  json:"username"`
	PasswordHash string     `gorm:"not null"                      json:"-"`
	Role         string     `gorm:"size:16;not null;default:'admin'" json:"role"`
	Active       bool       `gorm:"default:true"                  json:"active"`
	LastLoginAt  *time.Time `                                     json:"last_login_at"`
	CreatedAt    time.Time  `                                     json:"created_at"`
	UpdatedAt    time.Time  `                                     json:"updated_at"`
}

// IsSuperAdmin 是否超级管理员
func (a *AdminUser) IsSuperAdmin() bool {
	return a.Role == RoleSuperAdmin
}

// ValidAdminRole 校验角色取值
func ValidAdminRole(role string) bool {
	return role == RoleSuperAdmin || role == RoleAdmin
}
