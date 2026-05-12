package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
)

// ---- sing-box JSON 输出结构 ----
// 基线版本：sing-box 1.8.0（兼容 Hiddify / Karing / NekoBox / sing-box GUI）
// 字段命名严格匹配 https://sing-box.sagernet.org/configuration/

type singboxConfig struct {
	Log          singboxLog           `json:"log"`
	DNS          singboxDNS           `json:"dns"`
	Inbounds     []map[string]any     `json:"inbounds"`
	Outbounds    []map[string]any     `json:"outbounds"`
	Route        singboxRoute         `json:"route"`
	Experimental *singboxExperimental `json:"experimental,omitempty"`
}

type singboxLog struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type singboxDNS struct {
	Servers  []singboxDNSServer `json:"servers"`
	Final    string             `json:"final,omitempty"`
	Strategy string             `json:"strategy,omitempty"`
}

type singboxDNSServer struct {
	Tag     string `json:"tag"`
	Address string `json:"address"`
	Detour  string `json:"detour,omitempty"`
}

type singboxRoute struct {
	Rules               []map[string]any `json:"rules"`
	Final               string           `json:"final,omitempty"`
	AutoDetectInterface bool             `json:"auto_detect_interface"`
}

type singboxExperimental struct {
	CacheFile *singboxCacheFile `json:"cache_file,omitempty"`
	ClashAPI  *singboxClashAPI  `json:"clash_api,omitempty"`
}

type singboxCacheFile struct {
	Enabled bool `json:"enabled"`
}

type singboxClashAPI struct {
	ExternalController string `json:"external_controller,omitempty"`
	DefaultMode        string `json:"default_mode,omitempty"`
}

// GenerateSingbox 生成 sing-box 原生 JSON 订阅
//
// 设计要点：
//  1. 输出完整可用 Profile（log/dns/inbounds/outbounds/route），导入即可使用
//  2. 不引入 rule_set 远程依赖，避免首次启动需下载 geosite/geoip 拖累体验
//  3. 多节点用 selector + urltest 组织，对应 GUI 客户端的"代理组"
//  4. inbounds 包含 tun + mixed，桌面/移动端均可用
func (s *SubscribeService) GenerateSingbox(token string) (string, error) {
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return "", errors.New("无效的订阅令牌")
	}
	if !user.Active {
		return "", errors.New("用户已禁用")
	}
	if user.ExpiresAt != nil && user.ExpiresAt.Before(time.Now()) {
		return "", errors.New("订阅已过期")
	}

	groupIDs := userGroupIDs(user.Groups)
	if len(groupIDs) == 0 {
		return s.marshalSingbox(buildEmptySingboxConfig()), nil
	}

	// 健康节点优先，无健康节点回退到全部激活节点
	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(groupIDs, true)
	if err != nil {
		return "", fmt.Errorf("查询节点失败: %w", err)
	}
	if len(nodes) == 0 {
		nodes, err = s.nodeRepo.FindHealthyByGroupIDs(groupIDs, false)
		if err != nil {
			return "", fmt.Errorf("查询节点失败: %w", err)
		}
	}
	sortNodesByName(nodes)

	// 收集所有节点的 outbound 与 tag 列表
	var proxyOutbounds []map[string]any
	var proxyTags []string
	for _, node := range nodes {
		node := node
		profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
		if err != nil {
			continue
		}
		for _, key := range profileKeys {
			key := key
			ob, tag := s.buildSingboxOutbound(user, &node, key.Profile, &key)
			if ob == nil {
				continue
			}
			proxyOutbounds = append(proxyOutbounds, ob)
			proxyTags = append(proxyTags, tag)
		}
	}

	cfg := buildSingboxBaseConfig()
	cfg.Outbounds = append(cfg.Outbounds, proxyOutbounds...)
	cfg.Outbounds = append(cfg.Outbounds,
		buildSingboxSelector(proxyTags),
		buildSingboxURLTest(proxyTags),
		map[string]any{"type": "direct", "tag": "direct"},
		map[string]any{"type": "block", "tag": "block"},
		map[string]any{"type": "dns", "tag": "dns-out"},
	)
	cfg.Route.Final = "select"

	return s.marshalSingbox(cfg), nil
}

// buildSingboxOutbound 根据协议类型分发 outbound 构建
// 返回 (outbound, tag)，tag 用于 selector/urltest 引用
func (s *SubscribeService) buildSingboxOutbound(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey) (map[string]any, string) {
	if profile == nil {
		return nil, ""
	}
	transport := singboxTransportForProtocol(profile.Protocol)
	tag := s.buildRemark(node, user, string(profile.Protocol), transport)
	if tag == "" {
		tag = fmt.Sprintf("%s-%d", profile.Protocol, profile.ID)
	}

	switch profile.Protocol {
	case types.ProtocolVlessReality:
		return buildSingboxVlessReality(user, node, profile, key, tag), tag
	case types.ProtocolVlessWSTLS:
		return buildSingboxVlessWSTLS(user, node, profile, tag), tag
	case types.ProtocolTrojan:
		return buildSingboxTrojan(user, node, profile, tag), tag
	case types.ProtocolHysteria2:
		return buildSingboxHysteria2(user, node, profile, tag), tag
	}
	return nil, ""
}

// singboxTransportForProtocol 返回协议对应的传输层短名，用于填充
// 订阅备注模板里的 {transport} 占位符，避免输出 "[vless-reality - ]" 这类空尾
func singboxTransportForProtocol(protocol string) string {
	switch protocol {
	case types.ProtocolVlessReality:
		return "reality"
	case types.ProtocolVlessWSTLS:
		return "ws"
	case types.ProtocolTrojan:
		return "tcp"
	case types.ProtocolHysteria2:
		return "udp"
	}
	return ""
}

func buildSingboxVlessReality(user *entity.User, node *entity.Node, profile *entity.InboundProfile, key *entity.NodeProfileKey, tag string) map[string]any {
	var ps types.VlessRealitySettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}
	var km types.RealityKeyMaterial
	if key != nil && key.Settings != "" {
		_ = json.Unmarshal([]byte(key.Settings), &km)
	}

	sni := ps.SNI
	if sni == "" {
		sni = "www.microsoft.com"
	}
	fp := ps.Fingerprint
	if fp == "" {
		fp = "chrome"
	}
	shortID := ""
	if len(km.ShortIds) > 0 {
		shortID = km.ShortIds[0]
	}

	return map[string]any{
		"type":            "vless",
		"tag":             tag,
		"server":          node.ConnectAddr(),
		"server_port":     profile.Port,
		"uuid":            user.UUID,
		"flow":            "xtls-rprx-vision",
		"packet_encoding": "xudp",
		"tls": map[string]any{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]any{
				"enabled":     true,
				"fingerprint": fp,
			},
			"reality": map[string]any{
				"enabled":    true,
				"public_key": km.PublicKey,
				"short_id":   shortID,
			},
		},
	}
}

func buildSingboxVlessWSTLS(user *entity.User, node *entity.Node, profile *entity.InboundProfile, tag string) map[string]any {
	var ps types.VlessWSTLSSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}

	host := ps.Host
	if host == "" {
		host = node.ConnectAddr()
	}
	path := ps.Path
	if path == "" {
		path = "/"
	}

	return map[string]any{
		"type":            "vless",
		"tag":             tag,
		"server":          node.ConnectAddr(),
		"server_port":     profile.Port,
		"uuid":            user.UUID,
		"packet_encoding": "xudp",
		"tls": map[string]any{
			"enabled":     true,
			"server_name": host,
			"alpn":        []string{"h2", "http/1.1"},
			"utls": map[string]any{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		},
		"transport": map[string]any{
			"type":                   "ws",
			"path":                   path,
			"headers":                map[string]string{"Host": host},
			"max_early_data":         2048,
			"early_data_header_name": "Sec-WebSocket-Protocol",
		},
	}
}

func buildSingboxTrojan(user *entity.User, node *entity.Node, profile *entity.InboundProfile, tag string) map[string]any {
	var ps types.TrojanSettings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}
	sni := ps.SNI
	if sni == "" {
		sni = node.ConnectAddr()
	}

	return map[string]any{
		"type":        "trojan",
		"tag":         tag,
		"server":      node.ConnectAddr(),
		"server_port": profile.Port,
		"password":    user.UUID,
		"tls": map[string]any{
			"enabled":     true,
			"server_name": sni,
			"alpn":        []string{"h2", "http/1.1"},
			"utls": map[string]any{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		},
	}
}

func buildSingboxHysteria2(user *entity.User, node *entity.Node, profile *entity.InboundProfile, tag string) map[string]any {
	var ps types.Hysteria2Settings
	if profile.Settings != "" {
		_ = json.Unmarshal([]byte(profile.Settings), &ps)
	}
	sni := ps.SNI
	if sni == "" {
		sni = node.ConnectAddr()
	}

	ob := map[string]any{
		"type":        "hysteria2",
		"tag":         tag,
		"server":      node.ConnectAddr(),
		"server_port": profile.Port,
		"password":    user.UUID,
		"tls": map[string]any{
			"enabled":     true,
			"server_name": sni,
			"alpn":        []string{"h3"},
		},
	}
	if ps.UpMbps > 0 {
		ob["up_mbps"] = ps.UpMbps
	}
	if ps.DownMbps > 0 {
		ob["down_mbps"] = ps.DownMbps
	}
	return ob
}

func buildSingboxSelector(proxyTags []string) map[string]any {
	outbounds := append([]string{"auto"}, proxyTags...)
	return map[string]any{
		"type":      "selector",
		"tag":       "select",
		"outbounds": outbounds,
		"default":   "auto",
	}
}

func buildSingboxURLTest(proxyTags []string) map[string]any {
	tags := proxyTags
	if len(tags) == 0 {
		tags = []string{"direct"}
	}
	return map[string]any{
		"type":         "urltest",
		"tag":          "auto",
		"outbounds":    tags,
		"url":          "https://www.gstatic.com/generate_204",
		"interval":     "3m",
		"tolerance":    50,
		"idle_timeout": "30m",
	}
}

// buildSingboxBaseConfig 构建公共骨架（log/dns/inbounds/route 基础规则），outbounds 留给调用方填充
func buildSingboxBaseConfig() singboxConfig {
	return singboxConfig{
		Log: singboxLog{Level: "warn", Timestamp: true},
		DNS: singboxDNS{
			Servers: []singboxDNSServer{
				{Tag: "remote", Address: "https://1.1.1.1/dns-query", Detour: "select"},
				{Tag: "local", Address: "223.5.5.5", Detour: "direct"},
			},
			Final:    "remote",
			Strategy: "ipv4_only",
		},
		Inbounds: []map[string]any{
			{
				"type":         "tun",
				"tag":          "tun-in",
				"address":      []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route":   true,
				"strict_route": true,
				"stack":        "system",
				"sniff":        true,
			},
			{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      "127.0.0.1",
				"listen_port": 2080,
				"sniff":       true,
			},
		},
		Outbounds: []map[string]any{},
		Route: singboxRoute{
			Rules: []map[string]any{
				{"protocol": "dns", "outbound": "dns-out"},
				{"ip_is_private": true, "outbound": "direct"},
			},
			AutoDetectInterface: true,
		},
		Experimental: &singboxExperimental{
			CacheFile: &singboxCacheFile{Enabled: true},
			ClashAPI: &singboxClashAPI{
				ExternalController: "127.0.0.1:9090",
				DefaultMode:        "Rule",
			},
		},
	}
}

// buildEmptySingboxConfig 用户未绑定分组时返回的最小空骨架
func buildEmptySingboxConfig() singboxConfig {
	cfg := buildSingboxBaseConfig()
	cfg.Outbounds = []map[string]any{
		{"type": "direct", "tag": "direct"},
		{"type": "block", "tag": "block"},
		{"type": "dns", "tag": "dns-out"},
	}
	cfg.Route.Final = "direct"
	return cfg
}

// marshalSingbox 序列化为带缩进的 JSON 字符串；序列化失败时回退为 "{}"
func (s *SubscribeService) marshalSingbox(cfg singboxConfig) string {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}
