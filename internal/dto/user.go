package dto

import "time"

// CreateUserRequest 创建订阅用户请求
type CreateUserRequest struct {
	Username  string     `json:"username" binding:"required"`
	RealName  string     `json:"real_name"`
	GroupID   *uint      `json:"group_id"`
	ExpiresAt *time.Time `json:"expires_at"` // nil 表示永久有效
	Remark    string     `json:"remark"`
}

// UpdateUserRequest 更新订阅用户请求
type UpdateUserRequest struct {
	RealName  string     `json:"real_name"`
	GroupID   *uint      `json:"group_id"`
	Active    *bool      `json:"active"`
	ExpiresAt *time.Time `json:"expires_at"`
	Remark    string     `json:"remark"`
}

// UserResponse 用户响应 DTO（Token/UUID 不直接暴露，通过 subscribe_url 间接暴露）
type UserResponse struct {
	ID           uint   `json:"id"`
	Username     string `json:"username"`
	RealName     string `json:"real_name"`
	GroupID      *uint  `json:"group_id"`
	GroupName    string `json:"group_name,omitempty"`
	Active       bool   `json:"active"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Remark       string `json:"remark"`
	SubscribeURL string `json:"subscribe_url"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}
