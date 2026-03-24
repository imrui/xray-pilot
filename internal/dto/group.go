package dto

// CreateGroupRequest 创建分组请求
type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	NodeIDs     []uint `json:"node_ids"`
}

// UpdateGroupRequest 更新分组请求
type UpdateGroupRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	NodeIDs     []uint `json:"node_ids"`
}

// GroupResponse 分组响应
type GroupResponse struct {
	ID          uint           `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Active      bool           `json:"active"`
	NodeCount   int            `json:"node_count"`
	Nodes       []NodeResponse `json:"nodes,omitempty"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}
