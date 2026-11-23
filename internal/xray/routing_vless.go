package xray

import (
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/imrui/xray-pilot/pkg/crypto"
)

// Xray v26.3.27 与 v26.11.9 的 uTLS 名称，排除仅提供 TLS 1.2 的 ClientHello。
// 表单和服务端读取同一清单，远端预检检查节点实际安装版本支持的具体指纹。
//
//go:embed reality_fingerprints.json
var realityFingerprintJSON []byte

var realityFingerprints = func() []string {
	var names []string
	if err := json.Unmarshal(realityFingerprintJSON, &names); err != nil {
		panic(err)
	}
	return names
}()

type vlessOutboundAccount struct {
	ID, Flow, Encryption string
}

type vlessOutboundStream struct {
	Network, Method, Security string
	RealitySettings           struct {
		Password, PublicKey, Fingerprint, ShortID, SpiderX, ServerName string
		ServerNames, ShortIDs                                          []string
	}
}

func (s vlessOutboundStream) network() string {
	if s.Method != "" {
		return strings.ToLower(s.Method)
	}
	return strings.ToLower(s.Network)
}

func (s vlessOutboundStream) validateReality() error {
	switch s.network() {
	case "", "raw", "tcp", "grpc", "xhttp", "splithttp":
	default:
		return fmt.Errorf("REALITY 仅支持 RAW/TCP、XHTTP 或 gRPC 传输")
	}
	r := s.RealitySettings
	if len(r.ServerNames) > 0 || len(r.ShortIDs) > 0 {
		return fmt.Errorf("REALITY 出站使用单个 serverName 和 shortId")
	}
	key := r.Password
	if key == "" {
		key = r.PublicKey
	}
	if strings.HasPrefix(key, OutboundSecretPrefix) {
		plain, err := crypto.Decrypt(strings.TrimPrefix(key, OutboundSecretPrefix))
		if err != nil {
			return fmt.Errorf("REALITY 公钥无法解密")
		}
		key = plain
	}
	if raw, err := base64.RawURLEncoding.DecodeString(key); err != nil || len(raw) != 32 {
		return fmt.Errorf("REALITY password（旧名 publicKey）必须为 32 字节的无填充 Base64URL 公钥")
	}
	if b, err := hex.DecodeString(r.ShortID); err != nil || len(b) > 8 {
		return fmt.Errorf("REALITY shortId 必须是偶数位十六进制，最多 16 位")
	}
	if r.SpiderX != "" {
		if _, err := url.Parse(r.SpiderX); !strings.HasPrefix(r.SpiderX, "/") || err != nil {
			return fmt.Errorf("REALITY spiderX 必须为以 / 开头的有效路径")
		}
	}
	if r.Fingerprint != "" && !slices.Contains(realityFingerprints, strings.ToLower(r.Fingerprint)) {
		return fmt.Errorf("REALITY fingerprint 需要受支持的 TLS 1.3 ClientHello")
	}
	return nil
}

func (u vlessOutboundAccount) validate(s vlessOutboundStream) error {
	if strings.TrimSpace(u.ID) == "" {
		return fmt.Errorf("id 不能为空")
	}
	if strings.TrimSpace(u.Encryption) == "" {
		return fmt.Errorf("VLESS 用户必须显式设置 encryption，未启用加密时请填写 \"none\"")
	}
	switch u.Flow {
	case "":
		return nil
	case "xtls-rprx-vision", "xtls-rprx-vision-udp443":
		if u.Encryption == "none" {
			raw := s.network() == "" || s.network() == "tcp" || s.network() == "raw"
			if !raw || !(strings.EqualFold(s.Security, "tls") || strings.EqualFold(s.Security, "reality")) {
				return fmt.Errorf("未开启 VLESS Encryption 时，Vision 流控需要 RAW/TCP + TLS 或 REALITY")
			}
		}
		return nil
	default:
		return fmt.Errorf("不支持的 VLESS flow")
	}
}

func validateVLESSAccount(raw json.RawMessage, stream vlessOutboundStream) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("VLESS 用户必须是对象")
	}
	// 仅限制账号字段，保留扩展配置及 HTTP 头的原始拼写。
	for key := range fields {
		lower := strings.ToLower(key)
		if (lower == "encryption" || lower == "flow") && key != lower {
			return fmt.Errorf("VLESS 用户字段 %q 请使用标准命名 %q", key, lower)
		}
	}
	var user vlessOutboundAccount
	if err := json.Unmarshal(raw, &user); err != nil {
		return fmt.Errorf("VLESS 用户字段类型无效")
	}
	return user.validate(stream)
}

func validateVLESSOutbound(co CustomOutbound) error {
	var stream vlessOutboundStream
	if len(co.StreamSettings) > 0 && json.Unmarshal(co.StreamSettings, &stream) != nil {
		return fmt.Errorf("VLESS streamSettings 字段类型无效")
	}
	if strings.EqualFold(stream.Security, "reality") {
		if err := stream.validateReality(); err != nil {
			return err
		}
	}
	var cfg struct {
		Address string
		Port    int
		Vnext   []struct {
			Address string
			Port    int
			Users   []json.RawMessage
		}
	}
	if len(co.Settings) == 0 || json.Unmarshal(co.Settings, &cfg) != nil {
		return fmt.Errorf("vless 出站必须配置有效 settings")
	}
	if cfg.Address != "" {
		if len(cfg.Vnext) > 0 {
			return fmt.Errorf("vless 出站不能同时配置 settings.address 简写与 vnext")
		}
		if err := validateHostAddress(cfg.Address); err != nil {
			return err
		}
		if err := validatePortRange(cfg.Port); err != nil {
			return err
		}
		return validateVLESSAccount(co.Settings, stream)
	}
	if len(cfg.Vnext) == 0 {
		return fmt.Errorf("vless 出站必须配置 settings.address 或非空 settings.vnext")
	}
	for i, vn := range cfg.Vnext {
		if err := validateHostAddress(vn.Address); err != nil {
			return fmt.Errorf("vnext[%d]: %w", i, err)
		}
		if err := validatePortRange(vn.Port); err != nil {
			return fmt.Errorf("vnext[%d]: %w", i, err)
		}
		if len(vn.Users) == 0 {
			return fmt.Errorf("vnext[%d]: 必须至少配置一个用户", i)
		}
		for j, u := range vn.Users {
			if err := validateVLESSAccount(u, stream); err != nil {
				return fmt.Errorf("vnext[%d].users[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}
