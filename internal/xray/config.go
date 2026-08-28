package xray

import (
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"github.com/imrui/xray-pilot/pkg/types"
)

// ---- Xray JSON 数据结构 ----

type Config struct {
	Log       Log        `json:"log"`
	API       *API       `json:"api,omitempty"`
	Stats     *struct{}  `json:"stats,omitempty"`
	Policy    *Policy    `json:"policy,omitempty"`
	Routing   *Routing   `json:"routing,omitempty"`
	Inbounds  []Inbound  `json:"inbounds"`
	Outbounds []Outbound `json:"outbounds"`
}

// Policy 启用按用户/inbound 维度的流量统计
// 仅当 Stats 模块同时开启时生效。每个 client 配置里必须设置 email，
// 否则该用户在 StatsService 中不会被建立计数器
type Policy struct {
	Levels map[string]LevelPolicy `json:"levels"`
	System SystemPolicy           `json:"system"`
}

type LevelPolicy struct {
	StatsUserUplink   bool `json:"statsUserUplink"`
	StatsUserDownlink bool `json:"statsUserDownlink"`
}

type SystemPolicy struct {
	StatsInboundUplink   bool `json:"statsInboundUplink"`
	StatsInboundDownlink bool `json:"statsInboundDownlink"`
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
	DomainStrategy string        `json:"domainStrategy,omitempty"`
	Rules          []RoutingRule `json:"rules"`
}

type RoutingRule struct {
	Type        string   `json:"type,omitempty"`
	IP          []string `json:"ip,omitempty"`     // 目标 IP 匹配
	Source      []string `json:"source,omitempty"` // 来源 IP 匹配（http/socks 白名单用）
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
	Fingerprint string   `json:"fingerprint,omitempty"`
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
	ServerName   string    `json:"serverName"`
	Certificates []tlsCert `json:"certificates,omitempty"`
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

// http 代理入站结构（accounts 为空即无认证，依赖白名单路由拦截）
type httpInboundSettings struct {
	Accounts []types.ProxyAccount `json:"accounts,omitempty"`
}

// socks 代理入站结构
type socksInboundSettings struct {
	Auth     string               `json:"auth"` // "password" / "noauth"
	Accounts []types.ProxyAccount `json:"accounts,omitempty"`
	UDP      bool                 `json:"udp"`
}

// ---- 配置生成 ----

// LogConfig xray 日志配置（由 SettingService 提供）
type LogConfig struct {
	Access string // 访问日志路径，"none" 表示关闭
	Error  string // 错误日志路径，空表示 stderr
	Level  string // 日志级别：warning/info/debug
}

// GenerateConfig 根据节点、关联协议密钥和用户列表生成 Xray JSON 配置
// 返回 (configJSON, inboundWarnings, error)：单个协议生成失败不中断整体，通过 warnings 上报
func GenerateConfig(node *entity.Node, profileKeys []entity.NodeProfileKey, users []entity.User, logCfg LogConfig) (string, []string, error) {
	sort.Slice(profileKeys, func(i, j int) bool {
		if profileKeys[i].ProfileID == profileKeys[j].ProfileID {
			return profileKeys[i].ID < profileKeys[j].ID
		}
		return profileKeys[i].ProfileID < profileKeys[j].ProfileID
	})
	sort.Slice(users, func(i, j int) bool {
		if users[i].ID == users[j].ID {
			return users[i].Username < users[j].Username
		}
		return users[i].ID < users[j].ID
	})

	var inbounds []Inbound
	var warnings []string
	// http/socks 入站的白名单路由规则。分两组：allow_private 的 direct 规则
	// 必须排在 geoip:private 屏蔽之前才放得开内网目标，其余排在之后。
	var proxyRulesBefore, proxyRulesAfter []RoutingRule

	// usedPorts 兜底拦截同节点端口冲突（key: "传输层/端口"）。
	// 正常流程在保存密钥时已校验，此处防御旧数据或手工改库导致 Xray 起不来。
	usedPorts := make(map[string]string)

	for _, key := range profileKeys {
		if key.Profile == nil || !key.Profile.Active {
			continue
		}
		portKey := fmt.Sprintf("%s/%d", types.ProtocolTransport(key.Profile.Protocol), effectivePort(key.Profile, &key))
		if owner, ok := usedPorts[portKey]; ok {
			warnings = append(warnings, fmt.Sprintf("协议[%s]端口 %s 与[%s]冲突，已跳过该入站", key.Profile.Name, portKey, owner))
			continue
		}
		inbound, err := buildInbound(node, key.Profile, &key, users)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("协议[%s](%s): %v", key.Profile.Name, key.Profile.Protocol, err))
			continue
		}
		usedPorts[portKey] = key.Profile.Name
		inbounds = append(inbounds, inbound)
		if key.Profile.Protocol == types.ProtocolHTTP || key.Profile.Protocol == types.ProtocolSocks {
			before, after := proxyRoutingRules(key.Profile, &key)
			proxyRulesBefore = append(proxyRulesBefore, before...)
			proxyRulesAfter = append(proxyRulesAfter, after...)
		}
	}

	// gRPC API 入站（供远端管理，监听本地 10085）
	inbounds = append(inbounds, buildAPIInbound())

	level := logCfg.Level
	if level == "" {
		level = "warning"
	}
	cfg := Config{
		Log: Log{
			Loglevel: level,
			Access:   logCfg.Access,
			Error:    logCfg.Error,
		},
		API: &API{
			Tag:      "api",
			Services: []string{"HandlerService", "LoggerService", "StatsService"},
		},
		// 启用 stats 模块（空对象即可）+ policy 段，让 xray 按 email 维度记录每用户上下行
		Stats: &struct{}{},
		Policy: &Policy{
			Levels: map[string]LevelPolicy{
				"0": {StatsUserUplink: true, StatsUserDownlink: true},
			},
			System: SystemPolicy{
				StatsInboundUplink:   true,
				StatsInboundDownlink: true,
			},
		},
		Routing: &Routing{
			DomainStrategy: "IPIfNonMatch",
			// 规则顺序敏感：默认代理白名单 direct 排在 geoip:private 屏蔽之后，
			// 白名单来源无法通过代理访问内网地址；仅 allow_private=true 的入站
			// 其 direct 规则前置（proxyRulesBefore）放行内网目标
			Rules: buildRoutingRules(proxyRulesBefore, proxyRulesAfter),
		},
		Inbounds: inbounds,
		Outbounds: []Outbound{
			{Protocol: "freedom", Tag: "direct"},
			{Protocol: "blackhole", Tag: "block"},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", warnings, fmt.Errorf("序列化配置失败: %w", err)
	}
	return string(data), warnings, nil
}

// InboundTag 返回协议对应的 xray inbound tag（单一来源）。
// 配置生成（buildXxxInbound）与 gRPC live-apply（按 tag 定位 inbound 增删用户）必须共用此函数，
// 否则两侧 tag 命名漂移会导致 AlterInbound 找不到 inbound。
// 第二返回值表示该协议是否需要 gRPC per-user 账号管理
// （Hysteria2 不在 xray-core；http/socks 虽在 xray 中但账号来自协议配置而非用户系统，均返回 false 让 live-apply 跳过）。
func InboundTag(protocol string, profileID uint) (string, bool) {
	switch protocol {
	case types.ProtocolVlessReality:
		return fmt.Sprintf("vless-reality-%d", profileID), true
	case types.ProtocolVlessWSTLS:
		return fmt.Sprintf("vless-ws-%d", profileID), true
	case types.ProtocolTrojan:
		return fmt.Sprintf("trojan-%d", profileID), true
	case types.ProtocolHTTP:
		return fmt.Sprintf("http-proxy-%d", profileID), false
	case types.ProtocolSocks:
		return fmt.Sprintf("socks-proxy-%d", profileID), false
	default:
		return "", false
	}
}

// mustInboundTag 供 buildXxxInbound 内部使用：协议已确定在受支持类型内，直接取 tag。
func mustInboundTag(profile *entity.InboundProfile) string {
	tag, _ := InboundTag(profile.Protocol, profile.ID)
	return tag
}

// effectivePort 计算入站实际监听端口：节点级覆盖优先，回退协议模板端口。
func effectivePort(profile *entity.InboundProfile, key *entity.NodeProfileKey) int {
	if key != nil && key.Port > 0 {
		return key.Port
	}
	return profile.Port
}

func buildInbound(node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	switch profile.Protocol {
	case types.ProtocolVlessReality:
		return buildVlessRealityInbound(profile, key, users)
	case types.ProtocolVlessWSTLS:
		return buildVlessWSTLSInbound(profile, key, users)
	case types.ProtocolTrojan:
		return buildTrojanInbound(profile, key, users)
	case types.ProtocolHTTP, types.ProtocolSocks:
		return buildProxyInbound(profile, key)
	default:
		return Inbound{}, fmt.Errorf("不支持的协议: %s", profile.Protocol)
	}
}

func buildVlessRealityInbound(profile *entity.InboundProfile, key *entity.NodeProfileKey, users []entity.User) (Inbound, error) {
	// 解析协议共享参数（SNI、指纹、可选默认密钥）
	var ps types.VlessRealitySettings
	if err := parseSettings(profile.Settings, &ps); err != nil {
		return Inbound{}, fmt.Errorf("解析协议配置失败: %w", err)
	}

	// 解析节点密钥材料（覆盖协议默认值）
	var km types.RealityKeyMaterial
	if key != nil {
		if err := parseSettings(key.Settings, &km); err != nil {
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

	// SNI / 指纹走 helper：节点密钥覆盖 > 协议模板 > 默认
	sni := types.EffectiveRealitySNI(&ps, &km)
	fingerprint := types.EffectiveRealityFingerprint(&ps, &km)

	clients := buildVlessClients(users, "xtls-rprx-vision")

	return Inbound{
		Listen:   "0.0.0.0",
		Port:     effectivePort(profile, key),
		Protocol: "vless",
		Tag:      mustInboundTag(profile),
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
				Fingerprint: fingerprint,
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
	_ = parseSettings(profile.Settings, &ps)

	var cm types.TLSCertMaterial
	if key != nil {
		_ = parseSettings(key.Settings, &cm)
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
		Port:     effectivePort(profile, key),
		Protocol: "vless",
		Tag:      mustInboundTag(profile),
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
	_ = parseSettings(profile.Settings, &ps)

	var cm types.TLSCertMaterial
	if key != nil {
		_ = parseSettings(key.Settings, &cm)
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
		Port:           effectivePort(profile, key),
		Protocol:       "trojan",
		Tag:            mustInboundTag(profile),
		Settings:       trojanInboundSettings{Clients: clients},
		StreamSettings: stream,
	}, nil
}

// buildProxyInbound 构建 http/socks 明文代理入站（供运行时程序使用，非订阅协议）。
// 安全护栏：白名单与认证至少配置一项，否则拒绝生成——公网 VPS 上开放无鉴权
// 代理会被扫描滥用。白名单条目在此校验合法性，坏条目只跳过该入站（转为
// warning），不会让 xray 整体起不来。
func buildProxyInbound(profile *entity.InboundProfile, key *entity.NodeProfileKey) (Inbound, error) {
	ps, km, err := parseProxySettings(profile, key)
	if err != nil {
		return Inbound{}, err
	}

	// 白名单 / 认证账号走 helper：节点密钥覆盖 > 协议模板（消费方单一来源）
	allowedIPs := types.EffectiveProxyAllowedIPs(ps, km)
	accounts := types.EffectiveProxyAccounts(ps, km)
	if len(allowedIPs) == 0 && len(accounts) == 0 {
		return Inbound{}, fmt.Errorf("必须配置 allowed_ips 白名单或 accounts 认证之一，拒绝生成公网开放代理")
	}
	if err := validateAllowedIPs(allowedIPs); err != nil {
		return Inbound{}, err
	}

	var settings interface{}
	switch profile.Protocol {
	case types.ProtocolHTTP:
		settings = httpInboundSettings{Accounts: accounts}
	case types.ProtocolSocks:
		auth := "noauth"
		if len(accounts) > 0 {
			auth = "password"
		}
		settings = socksInboundSettings{Auth: auth, Accounts: accounts, UDP: false}
	}

	return Inbound{
		Listen:   "0.0.0.0",
		Port:     effectivePort(profile, key),
		Protocol: profile.Protocol, // "http" / "socks" 与 xray 协议名一致
		Tag:      mustInboundTag(profile),
		Settings: settings,
	}, nil
}

// parseProxySettings 解析 http/socks 的协议模板与节点密钥配置（同一结构）。
func parseProxySettings(profile *entity.InboundProfile, key *entity.NodeProfileKey) (*types.ProxyInboundSettings, *types.ProxyInboundSettings, error) {
	var ps types.ProxyInboundSettings
	if err := parseSettings(profile.Settings, &ps); err != nil {
		return nil, nil, fmt.Errorf("解析协议配置失败: %w", err)
	}
	var km types.ProxyInboundSettings
	if key != nil {
		if err := parseSettings(key.Settings, &km); err != nil {
			return nil, nil, fmt.Errorf("解析节点密钥配置失败: %w", err)
		}
	}
	return &ps, &km, nil
}

// validateAllowedIPs 校验白名单条目为合法 IP 或 CIDR。
// 非法条目会让 xray 路由初始化失败进而整体起不来，必须在生成期拦截。
func validateAllowedIPs(ips []string) error {
	for _, entry := range ips {
		if strings.Contains(entry, "/") {
			if _, _, err := net.ParseCIDR(entry); err != nil {
				return fmt.Errorf("白名单条目 %q 不是合法 CIDR", entry)
			}
			continue
		}
		if net.ParseIP(entry) == nil {
			return fmt.Errorf("白名单条目 %q 不是合法 IP", entry)
		}
	}
	return nil
}

// buildRoutingRules 组装最终路由规则，顺序：api → allow_private 代理 direct →
// geoip:private 屏蔽 → 普通代理白名单规则。
func buildRoutingRules(proxyBefore, proxyAfter []RoutingRule) []RoutingRule {
	rules := []RoutingRule{{InboundTag: []string{"api-inbound"}, OutboundTag: "api"}}
	rules = append(rules, proxyBefore...)
	rules = append(rules, RoutingRule{Type: "field", IP: []string{"geoip:private"}, OutboundTag: "block"})
	return append(rules, proxyAfter...)
}

// proxyRoutingRules 生成 http/socks 入站的白名单路由规则：
// 白名单来源 → direct；其余同 inbound 流量 → blackhole。
// 返回 (before, after)：before 需置于 geoip:private 屏蔽之前（allow_private 放行内网），
// after 置于其后（默认，内网目标被屏蔽）。
// 白名单为空（纯认证模式）时由 accounts 认证兜底，仅 allow_private 时需前置整入站 direct。
// 仅在 buildProxyInbound 成功后调用，settings 已校验过，解析失败静默返回空。
func proxyRoutingRules(profile *entity.InboundProfile, key *entity.NodeProfileKey) (before, after []RoutingRule) {
	ps, km, err := parseProxySettings(profile, key)
	if err != nil {
		return nil, nil
	}
	allowedIPs := types.EffectiveProxyAllowedIPs(ps, km)
	allowPrivate := types.EffectiveProxyAllowPrivate(ps, km)
	tag := mustInboundTag(profile)

	if len(allowedIPs) == 0 {
		if allowPrivate {
			return []RoutingRule{{Type: "field", InboundTag: []string{tag}, OutboundTag: "direct"}}, nil
		}
		return nil, nil
	}

	direct := RoutingRule{Type: "field", InboundTag: []string{tag}, Source: allowedIPs, OutboundTag: "direct"}
	block := RoutingRule{Type: "field", InboundTag: []string{tag}, OutboundTag: "block"}
	if allowPrivate {
		return []RoutingRule{direct}, []RoutingRule{block}
	}
	return nil, []RoutingRule{direct, block}
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

// parseSettings 将 settings 字符串反序列化到 v
// 兼容两种存储形式：
//   - 直接 JSON 对象：{"sni":"..."} → 正常解析
//   - JSON 字符串（二次编码）："{\"sni\":\"...\"}" → 先展开再解析
func parseSettings(raw string, v interface{}) error {
	if raw == "" {
		return nil
	}
	if raw[0] == '"' {
		// 二次编码：先将 JSON string 展开为原始 JSON
		var unwrapped string
		if err := json.Unmarshal([]byte(raw), &unwrapped); err == nil {
			raw = unwrapped
		}
	}
	return json.Unmarshal([]byte(raw), v)
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
