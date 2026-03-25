package xray

import (
	"encoding/json"
	"fmt"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"github.com/imrui/xray-pilot/pkg/types"
)

// ---- Xray JSON 数据结构 ----

type Config struct {
	Log       Log        `json:"log"`
	API       *API       `json:"api,omitempty"`
	Routing   *Routing   `json:"routing,omitempty"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
}

type Log struct {
	Access   string `json:"access"`
	Error    string `json:"error"`
	Loglevel string `json:"loglevel"`
}

type API struct {
	Tag      string   `json:"tag"`
	Services []string `json:"services"`
}

type Routing struct {
	Rules []RoutingRule `json:"rules"`
}

type RoutingRule struct {
	InboundTag  []string `json:"inboundTag,omitempty"`
	OutboundTag string   `json:"outboundTag"`
}

type Inbound struct {
	Listen         string      `json:"listen"`
	Port           int         `json:"port"`
	Protocol       string      `json:"protocol"`
	Tag            string      `json:"tag,omitempty"`
	Settings       interface{} `json:"settings"`
	StreamSettings interface{} `json:"streamSettings,omitempty"`
	Sniffing       *Sniffing   `json:"sniffing,omitempty"`
}

type Sniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

type Outbound struct {
	Protocol string `json:"protocol"`
	Tag      string `json:"tag"`
}

// VLESS 入站结构
type vlessInboundSettings struct {
	Clients    []VlessClient `json:"clients"`
	Decryption string        `json:"decryption"`
}

type VlessClient struct {
	ID    string `json:"id"`
	Flow  string `json:"flow,omitempty"`
	Email string `json:"email,omitempty"`
}

// Reality 流配置
type realityStream struct {
	Network         string          `json:"network"`
	Security        string          `json:"security"`
	RealitySettings realitySettings `json:"realitySettings"`
}

type realitySettings struct {
	Show        bool     `json:"show"`
	Dest        string   `json:"dest"`
	Xver        int      `json:"xver"`
	ServerNames []string `json:"serverNames"`
	PrivateKey  string   `json:"privateKey"`
	ShortIds    []string `json:"shortIds"`
}

// WebSocket+TLS 流配置
type wsStream struct {
	Network     string      `json:"network"`
	Security    string      `json:"security"`
	TLSSettings tlsSettings `json:"tlsSettings"`
	WSSettings  wsSettings  `json:"wsSettings"`
}

type wsSettings struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
}

type tlsSettings struct {
	ServerName  string        `json:"serverName"`
	Certificates []tlsCert   `json:"certificates,omitempty"`
}

type tlsCert struct {
	CertificateFile string `json:"certificateFile"`
	KeyFile         string `json:"keyFile"`
}

// Trojan 入站结构
type trojanInboundSettings struct {
	Clients []trojanClient `json:"clients"`
}

type trojanClient struct {
	Password string `json:"password"`
	Email    string `json:"email,omitempty"`
}

// DokodemoSettings API 入站（dokodemo-door）
type dokodemoSettings struct {
	Address string `json:"address"`
}

// ---- 配置生成 ----

// GenerateConfig 根据节点、关联协议密钥和用户列表生成 Xray JSON 配置
func GenerateConfig(node *entity.Node, profileKeys []entity.NodeProfileKey, users []entity.User) (string, error) {
	var inbounds []Inbound

	for _, key := range profileKeys {
		if key.Profile == nil || !key.Profile.Active {
			continue
		}
		inbound, err := buildInbound(node, key.Profile, &key, users)
		if err != nil {
			// 单个协议生成失败不中断整体，继续处理其他协议
			continue
		}
		inbounds = append(inbounds, inbound)
	}

	// gRPC API 入站（供远端管理，监听本地 10085）
	inbounds = append(inbounds, buildAPIInbound())

	cfg := Config{
		Log: Log{
			Loglevel: "warning",
			Access:   "none",
		},
		API: &API{
			Tag:      "api",
			Services: []string{"HandlerService", "LoggerService", "StatsService"},
		},
		Routing: &Routing{
			Rules: []RoutingRule{
				{InboundTag: []string{"api-inbound"}, OutboundTag: "api"},
			},
		},
		Inbounds: inbounds,
		Outbounds: []Outbound{
			{Protocol: "freedom", Tag: "direct"},
			{Protocol: "blackhole", Tag: "block"},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化配置失败: %w", err)
	}
	return string(data), nil
}

func buildInbound(node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	switch profile.Protocol {
	case types.ProtocolVlessReality:
		return buildVlessRealityInbound(profile, key, users)
	case types.ProtocolVlessWSTLS:
		return buildVlessWSTLSInbound(profile, key, users)
	case types.ProtocolTrojan:
		return buildTrojanInbound(profile, key, users)
	default:
		return Inbound{}, fmt.Errorf("不支持的协议: %s", profile.Protocol)
	}
}

func buildVlessRealityInbound(profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	// 解析协议共享参数（SNI、指纹、可选默认密钥）
	var ps types.VlessRealitySettings
	if profile.Settings != "" {
		if err := json.Unmarshal([]byte(profile.Settings), &ps); err != nil {
			return Inbound{}, fmt.Errorf("解析协议配置失败: %w", err)
		}
	}

	// 解析节点密钥材料（覆盖协议默认值）
	var km types.RealityKeyMaterial
	if key != nil && key.Settings != "" {
		if err := json.Unmarshal([]byte(key.Settings), &km); err != nil {
			return Inbound{}, fmt.Errorf("解析密钥材料失败: %w", err)
		}
	}

	// 优先使用节点密钥，回退到协议级默认值
	privateKeyEnc := km.PrivateKey
	if privateKeyEnc == "" {
		privateKeyEnc = ps.PrivateKey
	}
	if privateKeyEnc == "" {
		return Inbound{}, fmt.Errorf("vless-reality 缺少私钥（请在节点密钥或协议配置中提供 private_key）")
	}

	privateKey, err := decryptKey(privateKeyEnc)
	if err != nil {
		return Inbound{}, fmt.Errorf("解密私钥失败: %w", err)
	}

	shortIds := km.ShortIds
	if len(shortIds) == 0 {
		shortIds = ps.ShortIds
	}
	if len(shortIds) == 0 {
		shortIds = []string{""} // xray 要求至少一个元素
	}

	sni := ps.SNI
	if sni == "" {
		sni = "www.microsoft.com"
	}

	clients := buildVlessClients(users, "xtls-rprx-vision")

	return Inbound{
		Listen:   "0.0.0.0",
		Port:     profile.Port,
		Protocol: "vless",
		Tag:      fmt.Sprintf("vless-reality-%d", profile.ID),
		Settings: vlessInboundSettings{
			Clients:    clients,
			Decryption: "none",
		},
		StreamSettings: realityStream{
			Network:  "tcp",
			Security: "reality",
			RealitySettings: realitySettings{
				Show:        false,
				Dest:        fmt.Sprintf("%s:443", sni),
				Xver:        0,
				ServerNames: []string{sni},
				PrivateKey:  privateKey,
				ShortIds:    shortIds,
			},
		},
		Sniffing: &Sniffing{
			Enabled:      true,
			DestOverride: []string{"http", "tls", "quic"},
		},
	}, nil
}

func buildVlessWSTLSInbound(profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	var ps types.VlessWSTLSSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	var cm types.TLSCertMaterial
	if key.Settings != "" {
		_ = json.Unmarshal([]byte(key.Settings), &cm)
	}

	clients := buildVlessClients(users, "")

	stream := wsStream{
		Network:  "ws",
		Security: "tls",
		TLSSettings: tlsSettings{
			ServerName: ps.Host,
		},
		WSSettings: wsSettings{
			Path: ps.Path,
		},
	}
	if cm.CertPath != "" {
		stream.TLSSettings.Certificates = []tlsCert{{
			CertificateFile: cm.CertPath,
			KeyFile:         cm.KeyPath,
		}}
	}

	return Inbound{
		Listen:   "0.0.0.0",
		Port:     profile.Port,
		Protocol: "vless",
		Tag:      fmt.Sprintf("vless-ws-%d", profile.ID),
		Settings: vlessInboundSettings{
			Clients:    clients,
			Decryption: "none",
		},
		StreamSettings: stream,
		Sniffing: &Sniffing{
			Enabled:      true,
			DestOverride: []string{"http", "tls"},
		},
	}, nil
}

func buildTrojanInbound(profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	var ps types.TrojanSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	var cm types.TLSCertMaterial
	if key.Settings != "" {
		_ = json.Unmarshal([]byte(key.Settings), &cm)
	}

	clients := make([]trojanClient, 0, len(users))
	for _, u := range users {
		if !u.Active {
			continue
		}
		clients = append(clients, trojanClient{
			Password: u.UUID,
			Email:    u.Username,
		})
	}

	stream := map[string]interface{}{
		"network":  "tcp",
		"security": "tls",
		"tlsSettings": tlsSettings{
			ServerName:   ps.SNI,
			Certificates: []tlsCert{{CertificateFile: cm.CertPath, KeyFile: cm.KeyPath}},
		},
	}

	return Inbound{
		Listen:         "0.0.0.0",
		Port:           profile.Port,
		Protocol:       "trojan",
		Tag:            fmt.Sprintf("trojan-%d", profile.ID),
		Settings:       trojanInboundSettings{Clients: clients},
		StreamSettings: stream,
	}, nil
}

func buildAPIInbound() Inbound {
	return Inbound{
		Listen:   "127.0.0.1",
		Port:     10085,
		Protocol: "dokodemo-door",
		Tag:      "api-inbound",
		Settings: dokodemoSettings{Address: "127.0.0.1"},
	}
}

func buildVlessClients(users []entity.User, flow string) []VlessClient {
	clients := make([]VlessClient, 0, len(users))
	for _, u := range users {
		if !u.Active {
			continue
		}
		clients = append(clients, VlessClient{
			ID:    u.UUID,
			Flow:  flow,
			Email: u.Username,
		})
	}
	return clients
}

// decryptKey 解密 AES-GCM 加密的密钥
func decryptKey(encryptedKey string) (string, error) {
	if encryptedKey == "" {
		return "", nil
	}
	plain, err := crypto.Decrypt(encryptedKey)
	if err != nil {
		// 兼容明文存储（旧数据）
		return encryptedKey, nil
	}
	return plain, nil
}

// ConfigHash 计算配置内容的 SHA256（供漂移检测使用）
func ConfigHash(content string) string {
	return crypto.HashConfig(content)
}
