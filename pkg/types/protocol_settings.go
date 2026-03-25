package types

// Protocol 协议类型常量
const (
	ProtocolVlessReality = "vless-reality"
	ProtocolVlessWSTLS   = "vless-ws-tls"
	ProtocolTrojan       = "trojan"
	ProtocolHysteria2    = "hysteria2"
)

// VlessRealitySettings VLESS+Reality 协议共享配置
type VlessRealitySettings struct {
	SNI         string `json:"sni"`
	Fingerprint string `json:"fingerprint"` // TLS 指纹，如 chrome
}

// VlessWSTLSSettings VLESS+WebSocket+TLS 协议共享配置
type VlessWSTLSSettings struct {
	Host string `json:"host"` // CDN 域名
	Path string `json:"path"` // WebSocket 路径
}

// TrojanSettings Trojan 协议共享配置
type TrojanSettings struct {
	SNI string `json:"sni"`
}

// Hysteria2Settings Hysteria2 协议共享配置
type Hysteria2Settings struct {
	SNI      string `json:"sni"`
	UpMbps   int    `json:"up_mbps"`
	DownMbps int    `json:"down_mbps"`
}

// RealityKeyMaterial 节点 Reality 密钥材料
type RealityKeyMaterial struct {
	PrivateKey string `json:"private_key"` // AES-GCM 加密存储
	PublicKey  string `json:"public_key"`
	ShortID    string `json:"short_id"`
}

// TLSCertMaterial 节点 TLS 证书材料
type TLSCertMaterial struct {
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}
