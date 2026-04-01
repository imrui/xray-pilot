package dto

import (
	"encoding/json"
	"time"
)

// CreateUserRequest 创建订阅用户请求
type CreateUserRequest struct {
	Username      string     `json:"username" binding:"required"`
	RealName      string     `json:"real_name"`
	GroupID       *uint      `json:"group_id"`
	ExpiresAt     *time.Time `json:"expires_at"` // nil 表示永久有效
	Remark        string     `json:"remark"`
	FeishuEnabled bool       `json:"feishu_enabled"`
	FeishuEmail   string     `json:"feishu_email"`
	FeishuOpenID  string     `json:"feishu_open_id"`
	FeishuUnionID string     `json:"feishu_union_id"`
	FeishuChatID  string     `json:"feishu_chat_id"`
}

// UpdateUserRequest 更新订阅用户请求
type UpdateUserRequest struct {
	Username      *string         `json:"username"`
	RealName      *string         `json:"real_name"`
	GroupID       json.RawMessage `json:"group_id"`
	Active        *bool           `json:"active"`
	ExpiresAt     json.RawMessage `json:"expires_at"`
	Remark        *string         `json:"remark"`
	FeishuEnabled *bool           `json:"feishu_enabled"`
	FeishuEmail   *string         `json:"feishu_email"`
	FeishuOpenID  *string         `json:"feishu_open_id"`
	FeishuUnionID *string         `json:"feishu_union_id"`
	FeishuChatID  *string         `json:"feishu_chat_id"`
}

// UserResponse 用户响应 DTO（Token/UUID 不直接暴露，通过 subscribe_url 间接暴露）
type UserResponse struct {
	ID            uint   `json:"id"`
	Username      string `json:"username"`
	RealName      string `json:"real_name"`
	GroupID       *uint  `json:"group_id"`
	GroupName     string `json:"group_name,omitempty"`
	Active        bool   `json:"active"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	Remark        string `json:"remark"`
	SubscribeURL  string `json:"subscribe_url"`
	FeishuEnabled bool   `json:"feishu_enabled"`
	FeishuEmail   string `json:"feishu_email,omitempty"`
	FeishuOpenID  string `json:"feishu_open_id,omitempty"`
	FeishuUnionID string `json:"feishu_union_id,omitempty"`
	FeishuChatID  string `json:"feishu_chat_id,omitempty"`
	FeishuBoundAt string `json:"feishu_bound_at,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}
