package dto

// AdminResponse 管理员账号响应（不含密码哈希）
type AdminResponse struct {
	ID          uint   `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
	LastLoginAt string `json:"last_login_at,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// CreateAdminRequest 新建管理员
type CreateAdminRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role"`
}

// UpdateAdminRequest 修改管理员角色 / 启停
type UpdateAdminRequest struct {
	Role   *string `json:"role"`
	Active *bool   `json:"active"`
}

// SetAdminPasswordRequest 超级管理员重置他人密码
type SetAdminPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

// ChangePasswordRequest 修改自己的密码
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// MeResponse 当前登录管理员
type MeResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}
