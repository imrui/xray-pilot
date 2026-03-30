package dto

import "encoding/json"

// CreateProfileRequest 创建协议接入配置请求
type CreateProfileRequest struct {
	Name     string          `json:"name" binding:"required"`
	Protocol string          `json:"protocol" binding:"required"` // vless-reality | vless-ws-tls | trojan | hysteria2
	Port     int             `json:"port" binding:"required,min=1,max=65535"`
	Settings json.RawMessage `json:"settings"` // JSON 对象: VlessRealitySettings / TrojanSettings / ...
	Active   *bool           `json:"active"`
	Remark   string          `json:"remark"`
}

// UpdateProfileRequest 更新协议接入配置请求
type UpdateProfileRequest struct {
	Name     string          `json:"name"`
	Protocol string          `json:"protocol"`
	Port     int             `json:"port" binding:"omitempty,min=1,max=65535"`
	Settings json.RawMessage `json:"settings"`
	Active   *bool           `json:"active"`
	Remark   string          `json:"remark"`
}

// ProfileResponse 协议接入配置响应
type ProfileResponse struct {
	ID        uint            `json:"id"`
	Name      string          `json:"name"`
	Protocol  string          `json:"protocol"`
	Port      int             `json:"port"`
	Settings  json.RawMessage `json:"settings"` // 返回为 JSON 对象
	Active    bool            `json:"active"`
	Remark    string          `json:"remark"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

// UpsertNodeKeyRequest 创建或更新节点密钥材料请求
type UpsertNodeKeyRequest struct {
	Settings json.RawMessage `json:"settings" binding:"required"` // JSON 对象: RealityKeyMaterial / TLSCertMaterial
}

// NodeKeyResponse 节点密钥材料响应（编辑场景返回可直接修改的 settings）
type NodeKeyResponse struct {
	NodeID    uint            `json:"node_id"`
	ProfileID uint            `json:"profile_id"`
	Settings  json.RawMessage `json:"settings"` // 返回 JSON 对象；Reality private_key 已解密
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}
