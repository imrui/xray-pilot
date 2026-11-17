package xray

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/pkg/crypto"
)

const (
	OutboundSecretPrefix = "enc:v1:"
	MaskedPlaceholder    = "<masked>"
	maxCustomOutbounds   = 32
)

var outboundTagRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

var reservedOutboundTags = map[string]bool{
	"direct":      true,
	"block":       true,
	"direct-v4":   true,
	"direct-v6":   true,
	"api":         true,
	"api-inbound": true,
	"dns-out":     true,
}

var reservedInboundPrefixes = []string{
	"vless-reality-",
	"vless-ws-",
	"trojan-",
	"http-proxy-",
	"socks-proxy-",
}

var allowedOutboundProtocols = map[string]bool{
	"freedom":     true,
	"blackhole":   true,
	"socks":       true,
	"http":        true,
	"vless":       true,
	"vmess":       true,
	"trojan":      true,
	"shadowsocks": true,
	"wireguard":   true,
	"loopback":    true,
}

// CustomOutbound 表示策略模板中的自定义出站配置，支持保留未知高级字段。
type CustomOutbound struct {
	Tag            string                     `json:"tag"`
	Protocol       string                     `json:"protocol"`
	Remark         string                     `json:"remark,omitempty"`
	SendThrough    string                     `json:"sendThrough,omitempty"`
	Settings       json.RawMessage            `json:"settings,omitempty"`
	StreamSettings json.RawMessage            `json:"streamSettings,omitempty"`
	Mux            json.RawMessage            `json:"mux,omitempty"`
	Extra          map[string]json.RawMessage `json:"-"`
}

// BalancerStrategy 预留给 Phase 3 负载均衡模块。
type BalancerStrategy struct {
	Type string `json:"type,omitempty"`
}

// Balancer 预留给 Phase 3 负载均衡模块。
type Balancer struct {
	Tag         string                     `json:"tag"`
	Remark      string                     `json:"remark,omitempty"`
	Selector    []string                   `json:"selector"`
	Strategy    *BalancerStrategy          `json:"strategy,omitempty"`
	FallbackTag string                     `json:"fallbackTag,omitempty"`
	Extra       map[string]json.RawMessage `json:"-"`
}

// Observatory 预留给 Phase 3 健康观测模块。
type Observatory struct {
	SubjectSelector   []string                   `json:"subjectSelector"`
	ProbeURL          string                     `json:"probeURL,omitempty"`
	ProbeInterval     string                     `json:"probeInterval,omitempty"`
	EnableConcurrency bool                       `json:"enableConcurrency,omitempty"`
	Extra             map[string]json.RawMessage `json:"-"`
}

// DNSConfig 预留给 Phase 3 服务端 DNS 分流模块。
type DNSConfig struct {
	Enabled                bool                       `json:"enabled"`
	QueryStrategy          string                     `json:"queryStrategy,omitempty"`
	DisableCache           bool                       `json:"disableCache,omitempty"`
	DisableFallback        bool                       `json:"disableFallback,omitempty"`
	DisableFallbackIfMatch bool                       `json:"disableFallbackIfMatch,omitempty"`
	Hosts                  map[string]json.RawMessage `json:"hosts,omitempty"`
	Servers                []json.RawMessage          `json:"servers,omitempty"`
	Extra                  map[string]json.RawMessage `json:"-"`
}

// OutboundOverride 表示节点级按出站 Tag 的参数替换或目标重映射。
type OutboundOverride struct {
	Mode      string          `json:"mode"`
	Outbound  *CustomOutbound `json:"outbound,omitempty"`
	TargetTag string          `json:"target_tag,omitempty"`
}

// ---- 无损 Extra 编解码 ----

type routingRuleAlias RoutingRule

var routingRuleKnownKeys = map[string]string{
	"domain":      "domain",
	"port":        "port",
	"network":     "network",
	"protocol":    "protocol",
	"type":        "type",
	"ip":          "ip",
	"source":      "source",
	"inboundtag":  "inboundTag",
	"outboundtag": "outboundTag",
	"balancertag": "balancerTag",
	"remark":      "remark",
	"disabled":    "disabled",
}

func (r *RoutingRule) UnmarshalJSON(data []byte) error {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	if rawMap == nil {
		return fmt.Errorf("路由规则必须是 JSON 对象")
	}
	var alias routingRuleAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	for k, v := range rawMap {
		lower := strings.ToLower(k)
		if canonical, ok := routingRuleKnownKeys[lower]; ok {
			if k != canonical {
				return fmt.Errorf("路由规则字段 %q 请使用标准命名 %q", k, canonical)
			}
			continue
		}
		if lower == "sourceip" {
			return fmt.Errorf("不允许修改系统入站或来源保护规则")
		}
		if lower == "targetip" {
			return fmt.Errorf("请使用 ip 字段配置目标地址匹配")
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[k] = v
	}
	*r = RoutingRule(alias)
	r.Extra = extra
	return nil
}

func (r RoutingRule) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(routingRuleAlias(r))
	if err != nil || len(r.Extra) == 0 {
		return base, err
	}
	merged := make(map[string]json.RawMessage, len(r.Extra)+8)
	for k, v := range r.Extra {
		if _, isKnown := routingRuleKnownKeys[strings.ToLower(k)]; !isKnown {
			merged[k] = v
		}
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(base, &known); err != nil {
		return nil, err
	}
	for k, v := range known {
		merged[k] = v
	}
	return json.Marshal(merged)
}

type customOutboundAlias CustomOutbound

var customOutboundKnownKeys = map[string]string{
	"tag":            "tag",
	"protocol":       "protocol",
	"remark":         "remark",
	"sendthrough":    "sendThrough",
	"settings":       "settings",
	"streamsettings": "streamSettings",
	"mux":            "mux",
}

func normalizeRawMessage(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return raw
}

func (c *CustomOutbound) UnmarshalJSON(data []byte) error {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	if rawMap == nil {
		return fmt.Errorf("自定义出站必须是 JSON 对象")
	}
	var alias customOutboundAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	for k, v := range rawMap {
		lower := strings.ToLower(k)
		if canonical, ok := customOutboundKnownKeys[lower]; ok {
			if k != canonical {
				return fmt.Errorf("自定义出站字段 %q 请使用标准命名 %q", k, canonical)
			}
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[k] = v
	}
	alias.Settings = normalizeRawMessage(alias.Settings)
	alias.StreamSettings = normalizeRawMessage(alias.StreamSettings)
	alias.Mux = normalizeRawMessage(alias.Mux)
	*c = CustomOutbound(alias)
	c.Extra = extra
	return nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (c CustomOutbound) MarshalJSON() ([]byte, error) {
	c.Settings = normalizeRawMessage(c.Settings)
	c.StreamSettings = normalizeRawMessage(c.StreamSettings)
	c.Mux = normalizeRawMessage(c.Mux)
	base, err := marshalNoEscape(customOutboundAlias(c))
	if err != nil || len(c.Extra) == 0 {
		return base, err
	}
	merged := make(map[string]json.RawMessage, len(c.Extra)+6)
	for k, v := range c.Extra {
		if _, isKnown := customOutboundKnownKeys[strings.ToLower(k)]; !isKnown {
			merged[k] = v
		}
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(base, &known); err != nil {
		return nil, err
	}
	for k, v := range known {
		merged[k] = v
	}
	return marshalNoEscape(merged)
}

type outboundAlias Outbound

var outboundKnownKeys = map[string]string{
	"settings":       "settings",
	"protocol":       "protocol",
	"tag":            "tag",
	"sendthrough":    "sendThrough",
	"streamsettings": "streamSettings",
	"mux":            "mux",
}

func (o *Outbound) UnmarshalJSON(data []byte) error {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	if rawMap == nil {
		return fmt.Errorf("outbound 必须是 JSON 对象")
	}
	var alias outboundAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	for k, v := range rawMap {
		if _, ok := outboundKnownKeys[strings.ToLower(k)]; ok {
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[k] = v
	}
	*o = Outbound(alias)
	o.Extra = extra
	return nil
}

func (o Outbound) MarshalJSON() ([]byte, error) {
	base, err := json.Marshal(outboundAlias(o))
	if err != nil || len(o.Extra) == 0 {
		return base, err
	}
	merged := make(map[string]json.RawMessage, len(o.Extra)+6)
	for k, v := range o.Extra {
		if _, isKnown := outboundKnownKeys[strings.ToLower(k)]; !isKnown && strings.ToLower(k) != "remark" {
			merged[k] = v
		}
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(base, &known); err != nil {
		return nil, err
	}
	for k, v := range known {
		merged[k] = v
	}
	return json.Marshal(merged)
}

func (c CustomOutbound) toRuntimeOutbound() Outbound {
	out := Outbound{
		Tag:         c.Tag,
		Protocol:    c.Protocol,
		SendThrough: c.SendThrough,
	}
	if len(c.Settings) > 0 {
		out.Settings = c.Settings
	}
	if len(c.StreamSettings) > 0 {
		out.StreamSettings = c.StreamSettings
	}
	if len(c.Mux) > 0 {
		out.Mux = c.Mux
	}
	if len(c.Extra) > 0 {
		out.Extra = make(map[string]json.RawMessage, len(c.Extra))
		for k, v := range c.Extra {
			if strings.ToLower(k) != "remark" {
				out.Extra[k] = v
			}
		}
		if len(out.Extra) == 0 {
			out.Extra = nil
		}
	}
	return out
}

// ---- 保留字与自定义出站校验 ----

// IsReservedOutboundTag 判断 tag 是否命中全局系统保留字或托管入站前缀。
func IsReservedOutboundTag(tag string) bool {
	if reservedOutboundTags[tag] {
		return true
	}
	for _, prefix := range reservedInboundPrefixes {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}

func validateOutboundTagFormat(tag string) error {
	if !outboundTagRegex.MatchString(tag) {
		return fmt.Errorf("出站 tag %q 格式无效（须匹配 ^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$）", tag)
	}
	if IsReservedOutboundTag(tag) {
		return fmt.Errorf("出站 tag %q 为系统保留名称，不允许自定义使用", tag)
	}
	return nil
}

func decodeJSONObject(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("必须是 JSON 对象")
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("只允许一个 JSON 对象")
	}
	return m, nil
}

func validateHostAddress(addr string) error {
	if addr == "" || strings.TrimSpace(addr) != addr || strings.ContainsAny(addr, " /\\\n\r\t@?#") {
		return fmt.Errorf("服务器地址不能为空或包含非法字符")
	}
	return nil
}

func validatePortRange(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("端口必须在 1~65535 之间")
	}
	return nil
}

func validateSecretCandidate(val string) error {
	if val == MaskedPlaceholder {
		return fmt.Errorf("敏感凭据不能为脱敏占位符 %s", MaskedPlaceholder)
	}
	if strings.HasPrefix(val, OutboundSecretPrefix) {
		cipherHex := strings.TrimPrefix(val, OutboundSecretPrefix)
		raw, err := hex.DecodeString(cipherHex)
		if err != nil || len(raw) <= 12 {
			return fmt.Errorf("出站敏感凭据密文格式无效")
		}
		if config.Global.Crypto.MasterKey != "" {
			if _, err := crypto.Decrypt(cipherHex); err != nil {
				return fmt.Errorf("出站敏感凭据密文无法解密")
			}
		}
	}
	return nil
}

func validateCustomOutbound(co CustomOutbound) error {
	if err := validateOutboundTagFormat(co.Tag); err != nil {
		return err
	}
	if !allowedOutboundProtocols[co.Protocol] {
		if co.Protocol == "dns" {
			return fmt.Errorf("不允许自定义 dns 出站，服务端 DNS 将通过顶层 dns 模块管控")
		}
		return fmt.Errorf("不支持的出站协议 %q", co.Protocol)
	}
	if co.SendThrough != "" {
		if strings.TrimSpace(co.SendThrough) != co.SendThrough || net.ParseIP(co.SendThrough) == nil {
			return fmt.Errorf("sendThrough 必须为合法 IPv4 或 IPv6 字面量")
		}
	}
	settingsMap, err := decodeJSONObject(co.Settings)
	if err != nil {
		return fmt.Errorf("settings 无效: %w", err)
	}
	if _, err := decodeJSONObject(co.StreamSettings); err != nil {
		return fmt.Errorf("streamSettings 无效: %w", err)
	}
	if _, err := decodeJSONObject(co.Mux); err != nil {
		return fmt.Errorf("mux 无效: %w", err)
	}
	if err := checkOutboundSecretsValid(co); err != nil {
		return err
	}

	switch co.Protocol {
	case "socks", "http":
		var cfg struct {
			Servers []struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Users   []struct {
					User string `json:"user"`
					Pass string `json:"pass"`
				} `json:"users"`
			} `json:"servers"`
		}
		if len(co.Settings) == 0 || json.Unmarshal(co.Settings, &cfg) != nil || len(cfg.Servers) == 0 {
			return fmt.Errorf("%s 出站必须配置非空 settings.servers", co.Protocol)
		}
		for i, srv := range cfg.Servers {
			if err := validateHostAddress(srv.Address); err != nil {
				return fmt.Errorf("servers[%d]: %w", i, err)
			}
			if err := validatePortRange(srv.Port); err != nil {
				return fmt.Errorf("servers[%d]: %w", i, err)
			}
		}
	case "vless":
		var cfg struct {
			Vnext []struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
				Users   []struct {
					ID string `json:"id"`
				} `json:"users"`
			} `json:"vnext"`
		}
		if len(co.Settings) == 0 || json.Unmarshal(co.Settings, &cfg) != nil || len(cfg.Vnext) == 0 {
			return fmt.Errorf("vless 出站必须配置非空 settings.vnext")
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
				if strings.TrimSpace(u.ID) == "" {
					return fmt.Errorf("vnext[%d].users[%d]: id 不能为空", i, j)
				}
			}
		}
	case "wireguard":
		var cfg struct {
			SecretKey string `json:"secretKey"`
			Peers     []struct {
				Endpoint     string `json:"endpoint"`
				PublicKey    string `json:"publicKey"`
				PreSharedKey string `json:"preSharedKey"`
			} `json:"peers"`
		}
		if len(co.Settings) == 0 || json.Unmarshal(co.Settings, &cfg) != nil {
			return fmt.Errorf("wireguard 出站 settings 格式无效")
		}
		if strings.TrimSpace(cfg.SecretKey) == "" {
			return fmt.Errorf("wireguard 出站 secretKey 不能为空")
		}
		if len(cfg.Peers) == 0 {
			return fmt.Errorf("wireguard 出站至少需要配置 1 个 peer")
		}
		for i, peer := range cfg.Peers {
			if strings.TrimSpace(peer.PublicKey) == "" {
				return fmt.Errorf("peers[%d]: publicKey 不能为空", i)
			}
			host, portStr, splitErr := net.SplitHostPort(peer.Endpoint)
			if splitErr != nil || validateHostAddress(host) != nil {
				return fmt.Errorf("peers[%d]: endpoint 必须为 host:port 格式", i)
			}
			port, portErr := strconv.Atoi(portStr)
			if portErr != nil || validatePortRange(port) != nil {
				return fmt.Errorf("peers[%d]: endpoint 端口必须在 1~65535 之间", i)
			}
		}
	case "freedom":
		if settingsMap != nil {
			if rawSend, exists := settingsMap["sendThrough"]; exists && rawSend != nil {
				return fmt.Errorf("freedom 的 sendThrough 应配置在出站顶层字段而非 settings 内")
			}
			if rawDS, exists := settingsMap["domainStrategy"]; exists && rawDS != nil {
				ds, ok := rawDS.(string)
				if !ok || strings.TrimSpace(ds) == "" {
					return fmt.Errorf("freedom settings.domainStrategy 无效")
				}
			}
		}
	}
	return nil
}

func extractDialerProxy(streamRaw json.RawMessage) (string, error) {
	if len(streamRaw) == 0 {
		return "", nil
	}
	m, err := decodeJSONObject(streamRaw)
	if err != nil || m == nil {
		return "", err
	}
	rawSockopt, exists := m["sockopt"]
	if !exists || rawSockopt == nil {
		return "", nil
	}
	sockopt, ok := rawSockopt.(map[string]any)
	if !ok {
		return "", fmt.Errorf("streamSettings.sockopt 必须是对象")
	}
	rawDP, exists := sockopt["dialerProxy"]
	if !exists || rawDP == nil {
		return "", nil
	}
	dp, ok := rawDP.(string)
	if !ok {
		return "", fmt.Errorf("streamSettings.sockopt.dialerProxy 必须是字符串")
	}
	return dp, nil
}

func rewriteDialerProxy(streamRaw json.RawMessage, targetTag string) (json.RawMessage, error) {
	m, err := decodeJSONObject(streamRaw)
	if err != nil || m == nil {
		return streamRaw, err
	}
	sockopt, ok := m["sockopt"].(map[string]any)
	if !ok {
		return streamRaw, nil
	}
	sockopt["dialerProxy"] = targetTag
	m["sockopt"] = sockopt
	out, err := json.Marshal(m)
	return out, err
}

func validateCustomOutbounds(outbounds []CustomOutbound) error {
	if len(outbounds) > maxCustomOutbounds {
		return fmt.Errorf("自定义出站最多 %d 个", maxCustomOutbounds)
	}
	seen := make(map[string]bool, len(outbounds))
	for i, co := range outbounds {
		if err := validateCustomOutbound(co); err != nil {
			return fmt.Errorf("custom_outbounds[%d]: %w", i, err)
		}
		if seen[co.Tag] {
			return fmt.Errorf("custom_outbounds[%d]: 出站 tag %q 重复", i, co.Tag)
		}
		seen[co.Tag] = true
	}

	edges := make(map[string]string, len(outbounds))
	for i, co := range outbounds {
		dp, err := extractDialerProxy(co.StreamSettings)
		if err != nil {
			return fmt.Errorf("custom_outbounds[%d]: %w", i, err)
		}
		if dp == "" {
			continue
		}
		if dp == "block" || dp == "api" || dp == "api-inbound" || dp == "dns-out" {
			return fmt.Errorf("custom_outbounds[%d]: dialerProxy 不能指向 %q", i, dp)
		}
		if dp != "direct" && dp != "direct-v4" && dp != "direct-v6" && !seen[dp] {
			return fmt.Errorf("custom_outbounds[%d]: dialerProxy 引用了不存在的出站 %q", i, dp)
		}
		edges[co.Tag] = dp
	}

	// 有向图环路检测（0=未访问, 1=访问中, 2=已完成）
	state := make(map[string]int, len(outbounds))
	var visit func(tag string) error
	visit = func(tag string) error {
		state[tag] = 1
		if next, ok := edges[tag]; ok && seen[next] {
			if state[next] == 1 {
				return fmt.Errorf("dialerProxy 存在循环依赖: %s -> %s", tag, next)
			}
			if state[next] == 0 {
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		state[tag] = 2
		return nil
	}
	for _, co := range outbounds {
		if state[co.Tag] == 0 {
			if err := visit(co.Tag); err != nil {
				return err
			}
		}
	}
	return nil
}

// ---- 引用图与精细化 IPv6 判定 ----

func isIPv6Literal(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = addr[1 : len(addr)-1]
	} else if host, _, err := net.SplitHostPort(addr); err == nil {
		addr = host
	}
	ip := net.ParseIP(addr)
	return ip != nil && ip.To4() == nil
}

func outboundRequiresIPv6(co CustomOutbound) bool {
	if co.SendThrough != "" && isIPv6Literal(co.SendThrough) {
		return true
	}
	if len(co.Settings) == 0 {
		return false
	}
	switch co.Protocol {
	case "freedom":
		var cfg struct {
			DomainStrategy string `json:"domainStrategy"`
		}
		if json.Unmarshal(co.Settings, &cfg) == nil && cfg.DomainStrategy == "ForceIPv6" {
			return true
		}
	case "socks", "http", "trojan", "shadowsocks":
		var cfg struct {
			Servers []struct {
				Address string `json:"address"`
			} `json:"servers"`
		}
		if json.Unmarshal(co.Settings, &cfg) == nil {
			for _, srv := range cfg.Servers {
				if isIPv6Literal(srv.Address) {
					return true
				}
			}
		}
	case "vless", "vmess":
		var cfg struct {
			Vnext []struct {
				Address string `json:"address"`
			} `json:"vnext"`
		}
		if json.Unmarshal(co.Settings, &cfg) == nil {
			for _, vn := range cfg.Vnext {
				if isIPv6Literal(vn.Address) {
					return true
				}
			}
		}
	case "wireguard":
		var cfg struct {
			Peers []struct {
				Endpoint string `json:"endpoint"`
			} `json:"peers"`
		}
		if json.Unmarshal(co.Settings, &cfg) == nil {
			for _, peer := range cfg.Peers {
				if host, _, err := net.SplitHostPort(peer.Endpoint); err == nil && isIPv6Literal(host) {
					return true
				}
			}
		}
	}
	return false
}

// referencedOutboundTags 返回策略中所有被实际启用规则及 dialerProxy 传递引用的出站 Tag 集合。
func (p RoutingPolicy) referencedOutboundTags() map[string]bool {
	refs := make(map[string]bool)
	queue := make([]string, 0, len(p.CustomRules)+2)
	enqueue := func(tag string) {
		if tag != "" && !refs[tag] {
			refs[tag] = true
			queue = append(queue, tag)
		}
	}

	if p.SchemaVersion < 2 || len(p.IPv4Domains) > 0 {
		enqueue("direct-v4")
	}
	if len(p.IPv6Domains) > 0 {
		if p.SchemaVersion >= 2 || p.EnableIPv6 {
			enqueue("direct-v6")
		} else {
			enqueue("direct-v4")
		}
	}
	for _, r := range p.CustomRules {
		if r.Disabled {
			continue
		}
		enqueue(r.OutboundTag)
	}

	byTag := make(map[string]CustomOutbound, len(p.CustomOutbounds))
	for _, co := range p.CustomOutbounds {
		byTag[co.Tag] = co
	}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		if co, ok := byTag[curr]; ok {
			if dp, err := extractDialerProxy(co.StreamSettings); err == nil && dp != "" {
				enqueue(dp)
			}
		}
	}
	return refs
}

// ---- 出站敏感字段加解密、幂等复用与脱敏 ----

type secretTransformFunc func(path, value string) (string, error)

func transformOutboundSecrets(co CustomOutbound, fn secretTransformFunc) (CustomOutbound, error) {
	out := co
	if len(co.Settings) > 0 {
		m, err := decodeJSONObject(co.Settings)
		if err != nil {
			return out, err
		}
		changed := false
		applyMapStr := func(obj map[string]any, key, fieldPath string) error {
			rawVal, exists := obj[key]
			if !exists || rawVal == nil {
				return nil
			}
			s, ok := rawVal.(string)
			if !ok || s == "" {
				return nil
			}
			next, err := fn(fieldPath, s)
			if err != nil {
				return err
			}
			if next != s {
				obj[key] = next
				changed = true
			}
			return nil
		}

		switch co.Protocol {
		case "socks", "http":
			if servers, ok := m["servers"].([]any); ok {
				for i, srvAny := range servers {
					srv, ok := srvAny.(map[string]any)
					if !ok {
						continue
					}
					if users, ok := srv["users"].([]any); ok {
						for j, uAny := range users {
							if u, ok := uAny.(map[string]any); ok {
								if err := applyMapStr(u, "pass", fmt.Sprintf("settings.servers[%d].users[%d].pass", i, j)); err != nil {
									return out, err
								}
							}
						}
					}
				}
			}
		case "vless", "vmess":
			if vnexts, ok := m["vnext"].([]any); ok {
				for i, vnAny := range vnexts {
					vn, ok := vnAny.(map[string]any)
					if !ok {
						continue
					}
					if users, ok := vn["users"].([]any); ok {
						for j, uAny := range users {
							if u, ok := uAny.(map[string]any); ok {
								if err := applyMapStr(u, "id", fmt.Sprintf("settings.vnext[%d].users[%d].id", i, j)); err != nil {
									return out, err
								}
							}
						}
					}
				}
			}
		case "trojan", "shadowsocks":
			if servers, ok := m["servers"].([]any); ok {
				for i, srvAny := range servers {
					if srv, ok := srvAny.(map[string]any); ok {
						if err := applyMapStr(srv, "password", fmt.Sprintf("settings.servers[%d].password", i)); err != nil {
							return out, err
						}
					}
				}
			}
		case "wireguard":
			if err := applyMapStr(m, "secretKey", "settings.secretKey"); err != nil {
				return out, err
			}
			if peers, ok := m["peers"].([]any); ok {
				for i, pAny := range peers {
					if peer, ok := pAny.(map[string]any); ok {
						if err := applyMapStr(peer, "preSharedKey", fmt.Sprintf("settings.peers[%d].preSharedKey", i)); err != nil {
							return out, err
						}
					}
				}
			}
		}

		if changed {
			raw, err := marshalNoEscape(m)
			if err != nil {
				return out, err
			}
			out.Settings = raw
		}
	}

	if len(co.StreamSettings) > 0 {
		m, err := decodeJSONObject(co.StreamSettings)
		if err != nil {
			return out, err
		}
		if rs, ok := m["realitySettings"].(map[string]any); ok {
			if s, ok := rs["privateKey"].(string); ok && s != "" {
				next, err := fn("streamSettings.realitySettings.privateKey", s)
				if err != nil {
					return out, err
				}
				if next != s {
					rs["privateKey"] = next
					m["realitySettings"] = rs
					raw, err := marshalNoEscape(m)
					if err != nil {
						return out, err
					}
					out.StreamSettings = raw
				}
			}
		}
	}

	return out, nil
}

func checkOutboundSecretsValid(co CustomOutbound) error {
	_, err := transformOutboundSecrets(co, func(_, val string) (string, error) {
		return val, validateSecretCandidate(val)
	})
	return err
}

type previousSecretBank struct {
	byTagPath map[string]string // key: tag + "\x00" + path -> enc:v1:...
	byPlain   map[string]string // key: plaintext -> enc:v1:...
}

func newSecretBank() *previousSecretBank {
	return &previousSecretBank{
		byTagPath: make(map[string]string),
		byPlain:   make(map[string]string),
	}
}

func (b *previousSecretBank) collectOutbound(co CustomOutbound) {
	_, _ = transformOutboundSecrets(co, func(path, val string) (string, error) {
		if strings.HasPrefix(val, OutboundSecretPrefix) {
			b.byTagPath[co.Tag+"\x00"+path] = val
			if plain, err := crypto.Decrypt(strings.TrimPrefix(val, OutboundSecretPrefix)); err == nil && plain != "" {
				if _, exists := b.byPlain[plain]; !exists {
					b.byPlain[plain] = val
				}
			}
		}
		return val, nil
	})
}

func (b *previousSecretBank) encryptOutbound(co CustomOutbound) (CustomOutbound, error) {
	return transformOutboundSecrets(co, func(path, val string) (string, error) {
		if err := validateSecretCandidate(val); err != nil {
			return "", err
		}
		if strings.HasPrefix(val, OutboundSecretPrefix) {
			cipherHex := strings.TrimPrefix(val, OutboundSecretPrefix)
			if _, err := crypto.Decrypt(cipherHex); err != nil {
				return "", fmt.Errorf("出站敏感凭据密文无效: %w", err)
			}
			return val, nil
		}
		if prevCipher, ok := b.byTagPath[co.Tag+"\x00"+path]; ok {
			if plain, err := crypto.Decrypt(strings.TrimPrefix(prevCipher, OutboundSecretPrefix)); err == nil && plain == val {
				return prevCipher, nil
			}
		}
		if prevCipher, ok := b.byPlain[val]; ok {
			return prevCipher, nil
		}
		enc, err := crypto.Encrypt(val)
		if err != nil {
			return "", fmt.Errorf("加密出站敏感凭据失败: %w", err)
		}
		cipherVal := OutboundSecretPrefix + enc
		b.byPlain[val] = cipherVal
		return cipherVal, nil
	})
}

// EncryptPolicySecrets 对策略中的自定义出站敏感字段执行 AES-GCM 加密；
// 若明文与 prev 中对应原密文解密结果一致，则复用原密文防止随机 Nonce 引发误漂移。
func EncryptPolicySecrets(next RoutingPolicy, prev *RoutingPolicy) (RoutingPolicy, error) {
	if len(next.CustomOutbounds) == 0 {
		return next, nil
	}
	bank := newSecretBank()
	if prev != nil {
		for _, co := range prev.CustomOutbounds {
			bank.collectOutbound(co)
		}
	}
	out := next
	out.CustomOutbounds = make([]CustomOutbound, len(next.CustomOutbounds))
	for i, co := range next.CustomOutbounds {
		enc, err := bank.encryptOutbound(co)
		if err != nil {
			return next, fmt.Errorf("custom_outbounds[%d]: %w", i, err)
		}
		out.CustomOutbounds[i] = enc
	}
	return out, nil
}

func decryptSingleOutbound(co CustomOutbound) (CustomOutbound, error) {
	return transformOutboundSecrets(co, func(_, val string) (string, error) {
		if !strings.HasPrefix(val, OutboundSecretPrefix) {
			return val, nil
		}
		plain, err := crypto.Decrypt(strings.TrimPrefix(val, OutboundSecretPrefix))
		if err != nil {
			return "", fmt.Errorf("解密出站敏感凭据失败: %w", err)
		}
		return plain, nil
	})
}

// DecryptPolicySecrets 解密策略中所有 enc:v1: 凭据，用于配置渲染及计算稳定语义摘要。
func DecryptPolicySecrets(p RoutingPolicy) (RoutingPolicy, error) {
	if len(p.CustomOutbounds) == 0 {
		return p, nil
	}
	out := p
	out.CustomOutbounds = make([]CustomOutbound, len(p.CustomOutbounds))
	for i, co := range p.CustomOutbounds {
		dec, err := decryptSingleOutbound(co)
		if err != nil {
			return p, fmt.Errorf("custom_outbounds[%d]: %w", i, err)
		}
		out.CustomOutbounds[i] = dec
	}
	return out, nil
}

func maskExtraRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj any
	if dec.Decode(&obj) != nil {
		return raw
	}
	changed := false
	var walk func(any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if s, ok := val.(string); ok && s != "" {
					switch k {
					case "privateKey", "password", "pass", "secretKey", "preSharedKey":
						if s != MaskedPlaceholder {
							x[k] = MaskedPlaceholder
							changed = true
						}
						continue
					}
					if strings.HasPrefix(s, OutboundSecretPrefix) {
						x[k] = MaskedPlaceholder
						changed = true
						continue
					}
				}
				x[k] = walk(val)
			}
			return x
		case []any:
			for i, elem := range x {
				x[i] = walk(elem)
			}
			return x
		case string:
			if strings.HasPrefix(x, OutboundSecretPrefix) {
				changed = true
				return MaskedPlaceholder
			}
			return x
		default:
			return v
		}
	}
	obj = walk(obj)
	if !changed {
		return raw
	}
	out, err := marshalNoEscape(obj)
	if err != nil {
		return raw
	}
	return out
}

func maskSingleOutbound(co CustomOutbound) CustomOutbound {
	masked, err := transformOutboundSecrets(co, func(_, _ string) (string, error) {
		return MaskedPlaceholder, nil
	})
	if err != nil {
		masked = co
	}
	masked.Settings = maskExtraRawJSON(masked.Settings)
	masked.StreamSettings = maskExtraRawJSON(masked.StreamSettings)
	if len(masked.Extra) > 0 {
		nextExtra := make(map[string]json.RawMessage, len(masked.Extra))
		for k, v := range masked.Extra {
			nextExtra[k] = maskExtraRawJSON(v)
		}
		masked.Extra = nextExtra
	}
	return masked
}

// MaskPolicySecrets 将策略中所有自定义出站敏感字段替换为 "<masked>"（面向普通 admin）。
func MaskPolicySecrets(p RoutingPolicy) RoutingPolicy {
	if len(p.CustomOutbounds) == 0 {
		return p
	}
	out := p
	out.CustomOutbounds = make([]CustomOutbound, len(p.CustomOutbounds))
	for i, co := range p.CustomOutbounds {
		out.CustomOutbounds[i] = maskSingleOutbound(co)
	}
	return out
}

// MaskPolicyJSON 对策略 JSON 字符串中的自定义出站敏感凭据进行脱敏。
func MaskPolicyJSON(raw string) string {
	if raw == "" || (!strings.Contains(raw, "custom_outbounds") && !strings.Contains(raw, OutboundSecretPrefix)) {
		return raw
	}
	p, err := DecodeRoutingPolicy(raw)
	if err != nil {
		return string(maskExtraRawJSON(json.RawMessage(raw)))
	}
	if len(p.CustomOutbounds) == 0 {
		return raw
	}
	masked := MaskPolicySecrets(p)
	data, err := marshalNoEscape(masked)
	if err != nil {
		return raw
	}
	return string(data)
}

func decodeOutboundOverridesMap(raw json.RawMessage) (map[string]OutboundOverride, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		return nil, fmt.Errorf("outbound_overrides 必须是 JSON 对象")
	}
	out := make(map[string]OutboundOverride, len(entries))
	for tag, entryRaw := range entries {
		dec := json.NewDecoder(bytes.NewReader(entryRaw))
		dec.DisallowUnknownFields()
		var ov OutboundOverride
		if err := dec.Decode(&ov); err != nil {
			return nil, fmt.Errorf("outbound_overrides[%s]: %w", tag, err)
		}
		if dec.Decode(new(any)) != io.EOF {
			return nil, fmt.Errorf("outbound_overrides[%s]: 只允许一个 JSON 对象", tag)
		}
		if bytes.Equal(bytes.TrimSpace(entryRaw), []byte("null")) {
			return nil, fmt.Errorf("outbound_overrides[%s]: 不能为空", tag)
		}
		out[tag] = ov
	}
	return out, nil
}

func validateSingleOutboundOverrideShape(tag string, ov OutboundOverride) error {
	if err := validateOutboundTagFormat(tag); err != nil {
		return fmt.Errorf("outbound_overrides[%s]: %w", tag, err)
	}
	switch ov.Mode {
	case "replace":
		if ov.Outbound == nil || ov.TargetTag != "" {
			return fmt.Errorf("outbound_overrides[%s]: replace 模式必须提供 outbound 且不能包含 target_tag", tag)
		}
		if ov.Outbound.Tag != tag {
			return fmt.Errorf("outbound_overrides[%s]: outbound.tag (%q) 必须与覆盖键 %q 一致", tag, ov.Outbound.Tag, tag)
		}
		if err := validateCustomOutbound(*ov.Outbound); err != nil {
			return fmt.Errorf("outbound_overrides[%s]: %w", tag, err)
		}
	case "remap":
		if ov.TargetTag == "" || ov.Outbound != nil {
			return fmt.Errorf("outbound_overrides[%s]: remap 模式必须提供 target_tag 且不能包含 outbound", tag)
		}
		if ov.TargetTag == tag {
			return fmt.Errorf("outbound_overrides[%s]: target_tag 不能指向自身", tag)
		}
		if ov.TargetTag == "api" || ov.TargetTag == "api-inbound" || ov.TargetTag == "dns-out" {
			return fmt.Errorf("outbound_overrides[%s]: target_tag 不能指向系统保留出站 %q", tag, ov.TargetTag)
		}
		if ov.TargetTag != "direct" && ov.TargetTag != "block" && ov.TargetTag != "direct-v4" && ov.TargetTag != "direct-v6" {
			if err := validateOutboundTagFormat(ov.TargetTag); err != nil {
				return fmt.Errorf("outbound_overrides[%s]: %w", tag, err)
			}
		}
	default:
		return fmt.Errorf("outbound_overrides[%s]: mode 必须为 replace 或 remap", tag)
	}
	return nil
}

// ValidateRoutingOverrides 独立校验节点 Overrides JSON 的语法结构与安全边界。
func ValidateRoutingOverrides(overrides string) error {
	if overrides == "" || overrides == "{}" {
		return nil
	}
	if len(overrides) > 256*1024 {
		return fmt.Errorf("覆盖超过 256 KiB")
	}
	var o map[string]json.RawMessage
	dec := json.NewDecoder(strings.NewReader(overrides))
	if err := dec.Decode(&o); err != nil {
		return err
	}
	if dec.Decode(new(any)) != io.EOF {
		return fmt.Errorf("只允许一个 JSON 对象")
	}
	if o == nil {
		return fmt.Errorf("覆盖必须是 JSON 对象")
	}
	for k, v := range o {
		trimmed := bytes.TrimSpace(v)
		switch k {
		case "enable_ipv6_outbound":
			if !bytes.Equal(trimmed, []byte("null")) {
				var b bool
				if err := json.Unmarshal(trimmed, &b); err != nil {
					return err
				}
			}
		case "ipv4_force_domains", "ipv6_force_domains":
			if !bytes.Equal(trimmed, []byte("null")) {
				var list []string
				if err := json.Unmarshal(trimmed, &list); err != nil {
					return err
				}
				list = normalizeDomains(list)
				if len(list) > 1000 {
					return fmt.Errorf("域名列表最多 1000 项")
				}
				for _, d := range list {
					if err := validateDomain(d); err != nil {
						return err
					}
				}
			}
		case "ipv6_failure_policy":
			if !bytes.Equal(trimmed, []byte("null")) {
				var s string
				if err := json.Unmarshal(trimmed, &s); err != nil {
					return err
				}
				if s != "strict" {
					return fmt.Errorf("当前仅支持 strict；prefer_ipv6 尚未通过目标版本兼容验证")
				}
			}
		case "custom_rules":
			if !bytes.Equal(trimmed, []byte("null")) {
				var rules []RoutingRule
				if err := json.Unmarshal(trimmed, &rules); err != nil {
					return err
				}
				if len(rules) > 200 {
					return fmt.Errorf("自定义规则最多 200 条")
				}
				for i, r := range rules {
					if err := validateRoutingRuleFields(r, nil, true); err != nil {
						return fmt.Errorf("custom_rules[%d]: %w", i, err)
					}
					if r.OutboundTag == "api" || r.OutboundTag == "api-inbound" || r.OutboundTag == "dns-out" {
						return fmt.Errorf("custom_rules[%d]: 引用了未知或未启用的出站", i)
					}
				}
			}
		case "outbound_overrides":
			if !bytes.Equal(trimmed, []byte("null")) {
				ovMap, err := decodeOutboundOverridesMap(trimmed)
				if err != nil {
					return err
				}
				keys := make([]string, 0, len(ovMap))
				for tag := range ovMap {
					keys = append(keys, tag)
				}
				sort.Strings(keys)
				for _, tag := range keys {
					if err := validateSingleOutboundOverrideShape(tag, ovMap[tag]); err != nil {
						return err
					}
				}
			}
		case "dns_override":
			if !bytes.Equal(trimmed, []byte("null")) {
				return fmt.Errorf("DNS 与负载均衡将在后续阶段启用")
			}
		default:
			return fmt.Errorf("未知覆盖字段 %s", k)
		}
	}
	return nil
}

// EncryptOverridesSecrets 对节点 Overrides 中的 outbound_overrides 替换出站凭据进行加密与原密文复用。
func EncryptOverridesSecrets(nextRaw, prevRaw string, basePolicy *RoutingPolicy) (string, error) {
	if nextRaw == "" || nextRaw == "{}" {
		return nextRaw, nil
	}
	var nextObj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(nextRaw), &nextObj); err != nil || nextObj == nil {
		return nextRaw, nil
	}
	ovRaw, exists := nextObj["outbound_overrides"]
	if !exists || bytes.Equal(bytes.TrimSpace(ovRaw), []byte("null")) {
		return nextRaw, nil
	}
	ovMap, err := decodeOutboundOverridesMap(ovRaw)
	if err != nil {
		return "", err
	}
	if len(ovMap) == 0 {
		return nextRaw, nil
	}

	bank := newSecretBank()
	if prevRaw != "" && prevRaw != "{}" {
		var prevObj map[string]json.RawMessage
		if json.Unmarshal([]byte(prevRaw), &prevObj) == nil && prevObj != nil {
			if prevOvRaw, ok := prevObj["outbound_overrides"]; ok && !bytes.Equal(bytes.TrimSpace(prevOvRaw), []byte("null")) {
				if prevMap, err := decodeOutboundOverridesMap(prevOvRaw); err == nil {
					for _, ov := range prevMap {
						if ov.Outbound != nil {
							bank.collectOutbound(*ov.Outbound)
						}
					}
				}
			}
		}
	}
	if basePolicy != nil {
		for _, co := range basePolicy.CustomOutbounds {
			bank.collectOutbound(co)
		}
	}

	keys := make([]string, 0, len(ovMap))
	for tag := range ovMap {
		keys = append(keys, tag)
	}
	sort.Strings(keys)
	for _, tag := range keys {
		ov := ovMap[tag]
		if ov.Mode == "replace" && ov.Outbound != nil {
			enc, err := bank.encryptOutbound(*ov.Outbound)
			if err != nil {
				return "", fmt.Errorf("outbound_overrides[%s]: %w", tag, err)
			}
			ov.Outbound = &enc
			ovMap[tag] = ov
		}
	}
	encodedOv, err := json.Marshal(ovMap)
	if err != nil {
		return "", err
	}
	nextObj["outbound_overrides"] = encodedOv
	out, err := json.Marshal(nextObj)
	return string(out), err
}

// MaskOverridesJSON 将节点 Overrides JSON 中的出站敏感凭据替换为 "<masked>"。
func MaskOverridesJSON(raw string) string {
	if raw == "" || raw == "{}" || (!strings.Contains(raw, "outbound_overrides") && !strings.Contains(raw, OutboundSecretPrefix)) {
		return raw
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &obj) != nil || obj == nil {
		return raw
	}
	ovRaw, exists := obj["outbound_overrides"]
	if !exists || bytes.Equal(bytes.TrimSpace(ovRaw), []byte("null")) {
		return raw
	}
	ovMap, err := decodeOutboundOverridesMap(ovRaw)
	if err != nil {
		return string(maskExtraRawJSON(json.RawMessage(raw)))
	}
	for tag, ov := range ovMap {
		if ov.Outbound != nil {
			masked := maskSingleOutbound(*ov.Outbound)
			ov.Outbound = &masked
			ovMap[tag] = ov
		}
	}
	encodedOv, err := marshalNoEscape(ovMap)
	if err != nil {
		return raw
	}
	obj["outbound_overrides"] = encodedOv
	out, err := marshalNoEscape(obj)
	if err != nil {
		return raw
	}
	return string(out)
}

// PoliciesSemanticallyEqual 基于解密规范化后的策略结构判断两份路由策略是否等价。
func PoliciesSemanticallyEqual(aRaw, bRaw string) bool {
	if strings.TrimSpace(aRaw) == "" || strings.TrimSpace(bRaw) == "" {
		return strings.TrimSpace(aRaw) == strings.TrimSpace(bRaw)
	}
	pa, errA := DecodeRoutingPolicy(aRaw)
	pb, errB := DecodeRoutingPolicy(bRaw)
	if errA != nil || errB != nil {
		return crypto.HashConfig(aRaw) == crypto.HashConfig(bRaw)
	}
	if da, err := DecryptPolicySecrets(pa); err == nil {
		pa = da
	}
	if db, err := DecryptPolicySecrets(pb); err == nil {
		pb = db
	}
	ja, _ := json.Marshal(pa)
	jb, _ := json.Marshal(pb)
	return crypto.HashConfig(string(ja)) == crypto.HashConfig(string(jb))
}

// BuildOverridesForTarget 根据基础模板版本与目标已验证策略构造合法的节点 Overrides JSON（供手动恢复策略使用）。
func BuildOverridesForTarget(baseRaw, targetRaw string) (string, error) {
	if PoliciesSemanticallyEqual(baseRaw, targetRaw) {
		return "{}", nil
	}
	base, err := DecodeRoutingPolicy(baseRaw)
	if err != nil {
		return "", err
	}
	target, err := DecodeRoutingPolicy(targetRaw)
	if err != nil {
		return "", err
	}
	ov := map[string]any{
		"ipv4_force_domains":  target.IPv4Domains,
		"ipv6_force_domains":  target.IPv6Domains,
		"ipv6_failure_policy": target.IPv6FailurePolicy,
		"custom_rules":        target.CustomRules,
	}
	if base.SchemaVersion < 2 {
		ov["enable_ipv6_outbound"] = target.EnableIPv6
	}
	if len(base.CustomOutbounds) > 0 {
		targetByTag := make(map[string]CustomOutbound, len(target.CustomOutbounds))
		for _, co := range target.CustomOutbounds {
			targetByTag[co.Tag] = co
		}
		outOverrides := make(map[string]OutboundOverride)
		for _, baseCo := range base.CustomOutbounds {
			if targetCo, exists := targetByTag[baseCo.Tag]; exists {
				jb, _ := json.Marshal(baseCo)
				jt, _ := json.Marshal(targetCo)
				if crypto.HashConfig(string(jb)) != crypto.HashConfig(string(jt)) {
					cp := targetCo
					outOverrides[baseCo.Tag] = OutboundOverride{Mode: "replace", Outbound: &cp}
				}
			} else {
				outOverrides[baseCo.Tag] = OutboundOverride{Mode: "remap", TargetTag: "direct"}
			}
		}
		if len(outOverrides) > 0 {
			ov["outbound_overrides"] = outOverrides
		}
	}
	data, err := json.Marshal(ov)
	return string(data), err
}
