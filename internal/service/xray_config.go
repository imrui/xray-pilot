package service

import (
	"encoding/json"
	"fmt"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/crypto"
)

// ---- Xray config 数据结构 ----

type xrayConfig struct {
	Log       xrayLog        `json:"log"`
	Inbounds  []xrayInbound  `json:"inbounds"`
	Outbounds []xrayOutbound `json:"outbounds"`
}

type xrayLog struct {
	Access   string `json:"access"`
	Error    string `json:"error"`
	Loglevel string `json:"loglevel"`
}

type xrayInbound struct {
	Listen         string           `json:"listen"`
	Port           int              `json:"port"`
	Protocol       string           `json:"protocol"`
	Settings       xrayVlessInbound `json:"settings"`
	StreamSettings xrayStream       `json:"streamSettings"`
	Sniffing       xraySniffing     `json:"sniffing"`
}

type xrayVlessInbound struct {
	Clients    []xrayClient `json:"clients"`
	Decryption string       `json:"decryption"`
}

type xrayClient struct {
	ID    string `json:"id"`   // 用户 UUID
	Flow  string `json:"flow"` // xtls-rprx-vision
	Email string `json:"email,omitempty"`
}

type xrayStream struct {
	Network         string          `json:"network"`
	Security        string          `json:"security"`
	RealitySettings xrayReality     `json:"realitySettings"`
}

type xrayReality struct {
	Show        bool     `json:"show"`
	Dest        string   `json:"dest"` // SNI:443
	Xver        int      `json:"xver"`
	ServerNames []string `json:"serverNames"`
	PrivateKey  string   `json:"privateKey"`
	ShortIds    []string `json:"shortIds"`
}

type xraySniffing struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"destOverride"`
}

type xrayOutbound struct {
	Protocol string `json:"protocol"`
	Tag      string `json:"tag"`
}

// XrayConfigService 负责生成 Xray 配置
type XrayConfigService struct{}

func NewXrayConfigService() *XrayConfigService {
	return &XrayConfigService{}
}

// GenerateConfig 根据节点和用户列表生成 Xray JSON 配置
func (s *XrayConfigService) GenerateConfig(node *entity.Node, users []entity.User) (string, error) {
	// 解密节点 Reality 私钥
	privateKey, err := decryptNodeKey(node.PrivateKey)
	if err != nil {
		return "", fmt.Errorf("解密节点私钥失败: %w", err)
	}

	// 构建客户端列表（仅激活用户）
	clients := make([]xrayClient, 0, len(users))
	for _, u := range users {
		if !u.Active {
			continue
		}
		clients = append(clients, xrayClient{
			ID:    u.UUID,
			Flow:  "xtls-rprx-vision",
			Email: u.Username,
		})
	}

	sni := node.SNI
	if sni == "" {
		sni = "www.microsoft.com" // 默认 SNI
	}

	shortID := node.ShortID
	if shortID == "" {
		shortID = ""
	}

	cfg := xrayConfig{
		Log: xrayLog{
			Loglevel: "warning",
			Access:   "none",
		},
		Inbounds: []xrayInbound{
			{
				Listen:   "0.0.0.0",
				Port:     node.Port,
				Protocol: "vless",
				Settings: xrayVlessInbound{
					Clients:    clients,
					Decryption: "none",
				},
				StreamSettings: xrayStream{
					Network:  "tcp",
					Security: "reality",
					RealitySettings: xrayReality{
						Show:        false,
						Dest:        fmt.Sprintf("%s:443", sni),
						Xver:        0,
						ServerNames: []string{sni},
						PrivateKey:  privateKey,
						ShortIds:    []string{shortID},
					},
				},
				Sniffing: xraySniffing{
					Enabled:      true,
					DestOverride: []string{"http", "tls", "quic"},
				},
			},
		},
		Outbounds: []xrayOutbound{
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

// decryptNodeKey 解密节点存储的 Reality 私钥
// 如果私钥为空（未配置）则直接返回空字符串
func decryptNodeKey(encryptedKey string) (string, error) {
	if encryptedKey == "" {
		return "", nil
	}
	plain, err := crypto.Decrypt(encryptedKey)
	if err != nil {
		// 可能是明文存储（兼容旧数据），直接返回原值
		return encryptedKey, nil
	}
	return plain, nil
}

// EncryptNodeKey 加密 Reality 私钥后存储
func EncryptNodeKey(plainKey string) (string, error) {
	if plainKey == "" {
		return "", nil
	}
	return crypto.Encrypt(plainKey)
}

// ConfigHash 计算配置内容的 SHA256
func ConfigHash(content string) string {
	return crypto.HashConfig(content)
}
