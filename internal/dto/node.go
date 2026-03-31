package dto

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Name       string `json:"name" binding:"required"`
	Region     string `json:"region"`
	IP         string `json:"ip" binding:"required"`
	Domain     string `json:"domain"` // 可选：客户端连接域名（CDN/中转）
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	SSHKeyPath string `json:"ssh_key_path"`
	Remark     string `json:"remark"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Name       *string `json:"name"`
	Region     *string `json:"region"`
	IP         *string `json:"ip"`
	Domain     *string `json:"domain"`
	SSHPort    *int    `json:"ssh_port"`
	SSHUser    *string `json:"ssh_user"`
	SSHKeyPath *string `json:"ssh_key_path"`
	Remark     *string `json:"remark"`
}

// NodeResponse 节点响应（含同步状态，不含私钥）
type NodeResponse struct {
	ID              uint     `json:"id"`
	Name            string   `json:"name"`
	Region          string   `json:"region"`
	IP              string   `json:"ip"`
	Domain          string   `json:"domain"`
	GroupNames      []string `json:"group_names,omitempty"`
	OnlineUserCount int      `json:"online_user_count"`
	SSHPort         int      `json:"ssh_port"`
	SSHUser         string   `json:"ssh_user"`
	SSHKeyPath      string   `json:"ssh_key_path"`
	Active          bool     `json:"active"`
	XrayActive      bool     `json:"xray_active"`
	XrayVersion     string   `json:"xray_version"`
	SyncStatus      string   `json:"sync_status"`
	LastCheckOK     bool     `json:"last_check_ok"`
	LastLatencyMs   int      `json:"last_latency_ms"`
	LastSyncAt      string   `json:"last_sync_at,omitempty"`
	LastCheckAt     string   `json:"last_check_at,omitempty"`
	Remark          string   `json:"remark"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

// KeygenResponse 密钥对生成响应
type KeygenResponse struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}
