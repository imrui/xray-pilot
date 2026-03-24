package dto

// CreateUserRequest 创建用户请求
type CreateUserRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"` // 明文，服务层 bcrypt hash
	RealName   string `json:"real_name"`
	Department string `json:"department"`
	GroupID    *uint  `json:"group_id"`
	Remark     string `json:"remark"`
}

// UpdateUserRequest 更新用户请求
type UpdateUserRequest struct {
	Password   string `json:"password"`   // 可选，非空时更新密码
	RealName   string `json:"real_name"`
	Department string `json:"department"`
	GroupID    *uint  `json:"group_id"`
	Active     *bool  `json:"active"` // 可选，允许通过 Update 直接切换状态
	Remark     string `json:"remark"`
}

// UserResponse 用户响应（UUID 和 Token 不暴露，SubscribeURL 动态拼接）
type UserResponse struct {
	ID           uint   `json:"id"`
	Username     string `json:"username"`
	RealName     string `json:"real_name"`
	Department   string `json:"department"`
	GroupID      *uint  `json:"group_id"`
	GroupName    string `json:"group_name,omitempty"`
	Active       bool   `json:"active"`
	Remark       string `json:"remark"`
	SubscribeURL string `json:"subscribe_url"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}
