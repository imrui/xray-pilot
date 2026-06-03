package types

// Protocol 协议类型常量
const (
	ProtocolVlessReality = "vless-reality"
	ProtocolVlessWSTLS   = "vless-ws-tls"
	ProtocolTrojan       = "trojan"
	ProtocolHysteria2    = "hysteria2"
)

// 传输层常量，用于节点内端口冲突判定
const (
	TransportTCP = "tcp"
	TransportUDP = "udp"
)

// ProtocolTransport 返回协议占用的传输层。
// TCP 与 UDP 各自独立编址：同一端口号在不同传输层互不冲突
// （如 Reality 占 TCP 443，Hysteria2 可同时占 UDP 443）。
func ProtocolTransport(protocol string) string {
	switch protocol {
	case ProtocolHysteria2:
		return TransportUDP
	default:
		return TransportTCP
	}
}

// VlessRealitySettings VLESS+Reality 协议共享配置
// PrivateKey/PublicKey/ShortIds 为可选默认值，各节点可通过 NodeProfileKey 覆盖
type VlessRealitySettings struct {
	SNI         string   `json:"sni"`
	Fingerprint string   `json:"fingerprint"`          // TLS 指纹，如 chrome
	PrivateKey  string   `json:"private_key,omitempty"` // 默认私钥（AES-GCM 加密存储）
	PublicKey   string   `json:"public_key,omitempty"`  // 默认公钥
	ShortIds    []string `json:"short_ids,omitempty"`   // 默认 short_id 列表
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

// RealityKeyMaterial 节点 Reality 密钥材料（per-node 覆盖，为空则 fallback 到 VlessRealitySettings）
type RealityKeyMaterial struct {
	PrivateKey string   `json:"private_key"` // AES-GCM 加密存储
	PublicKey  string   `json:"public_key"`
	ShortIds   []string `json:"short_ids"` // short_id 列表，Xray 要求数组格式
}

// TLSCertMaterial 节点 TLS 证书材料
type TLSCertMaterial struct {
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}
