package dto

// CreateNodeRequest 创建节点请求
type CreateNodeRequest struct {
	Name       string `json:"name" binding:"required"`
	Region     string `json:"region"`
	IP         string `json:"ip" binding:"required"`
	Port       int    `json:"port"`
	PrivateKey string `json:"private_key"` // 明文传入，服务层 AES 加密存储
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	SNI        string `json:"sni"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	SSHKeyPath string `json:"ssh_key_path"`
	Remark     string `json:"remark"`
}

// UpdateNodeRequest 更新节点请求
type UpdateNodeRequest struct {
	Name       string `json:"name"`
	Region     string `json:"region"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	PrivateKey string `json:"private_key"` // 非空时更新，AES 加密存储
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
	SNI        string `json:"sni"`
	SSHPort    int    `json:"ssh_port"`
	SSHUser    string `json:"ssh_user"`
	SSHKeyPath string `json:"ssh_key_path"`
	Remark     string `json:"remark"`
}

// NodeResponse 节点响应（包含同步状态）
type NodeResponse struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	Region        string `json:"region"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	PublicKey     string `json:"public_key"`
	ShortID       string `json:"short_id"`
	SNI           string `json:"sni"`
	SSHPort       int    `json:"ssh_port"`
	SSHUser       string `json:"ssh_user"`
	SSHKeyPath    string `json:"ssh_key_path"`
	Active        bool   `json:"active"`
	SyncStatus    string `json:"sync_status"`
	LastCheckOK   bool   `json:"last_check_ok"`
	LastLatencyMs int    `json:"last_latency_ms"`
	LastSyncAt    string `json:"last_sync_at,omitempty"`
	LastCheckAt   string `json:"last_check_at,omitempty"`
	Remark        string `json:"remark"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// KeygenResponse 密钥对生成响应
type KeygenResponse struct {
	PrivateKey string `json:"private_key"`
	PublicKey  string `json:"public_key"`
}
