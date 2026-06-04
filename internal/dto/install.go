package dto

import "time"

// CreateInstallTokenRequest 管理员侧创建一次性安装 token 的请求
type CreateInstallTokenRequest struct {
	Name     string `json:"name"      binding:"required"` // 节点名
	Region   string `json:"region"`                       // 地区
	Owner    string `json:"owner"`                        // 所有者
	Remark   string `json:"remark"`                       // 备注
	SSHUser  string `json:"ssh_user"`                     // 默认 root
	SSHPort  int    `json:"ssh_port"`                     // 默认 22

	// PanelURL 用于拼装 curl 命令；前端通常传 window.location.origin。
	// 不持久化到 token 表。
	PanelURL string `json:"panel_url" binding:"required"`

	// TTLSeconds token 有效期；缺省 600（10 分钟），上限 86400（24 小时）。
	TTLSeconds int `json:"ttl_seconds"`
}

// InstallTokenResponse 创建后返回给前端展示
type InstallTokenResponse struct {
	ID           uint      `json:"id"`
	Token        string    `json:"token"`
	ExpiresAt    time.Time `json:"expires_at"`
	CurlCommand  string    `json:"curl_command"`   // 一行 curl 命令，含 PANEL_URL + INSTALL_TOKEN
	NodeName     string    `json:"node_name"`      // 回显方便对话框显示
	Used         bool      `json:"used"`
	NodeID       *uint     `json:"node_id,omitempty"`
	UsedByIP     string    `json:"used_by_ip,omitempty"`
}

// RegisterNodeRequest 节点装机脚本回调时上报的自检信息
type RegisterNodeRequest struct {
	PublicIP    string `json:"public_ip"`
	XrayVersion string `json:"xray_version"`
	Kernel      string `json:"kernel"`
	Distro      string `json:"distro"`
}

// RegisterNodeResponse 注册成功后返回
type RegisterNodeResponse struct {
	NodeID uint   `json:"node_id"`
	Name   string `json:"name"`
}
