package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
)

type RoutingPolicy struct {
	SchemaVersion     uint          `json:"schema_version"`
	EnableIPv6        bool          `json:"enable_ipv6_outbound,omitempty"` // Schema 1 only; retained for historical versions and rollback.
	IPv4Domains       []string      `json:"ipv4_force_domains"`
	IPv6Domains       []string      `json:"ipv6_force_domains"`
	IPv6FailurePolicy string        `json:"ipv6_failure_policy"`
	CustomRules       []RoutingRule `json:"custom_rules"`
}

func DecodeRoutingPolicy(raw string) (RoutingPolicy, error) {
	var p RoutingPolicy
	if len(raw) > 256*1024 {
		return p, fmt.Errorf("路由策略超过 256 KiB")
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	if d.Decode(new(any)) != io.EOF {
		return p, fmt.Errorf("只允许一个 JSON 对象")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	if fields == nil {
		return p, fmt.Errorf("路由策略必须是 JSON 对象")
	}
	if p.SchemaVersion == 0 {
		// Old API callers could omit the boolean switch. Unversioned drafts,
		// revisions and snapshots must all keep their original interpretation.
		p.SchemaVersion = 1
	}
	if p.SchemaVersion == 2 && fields["enable_ipv6_outbound"] != nil {
		return p, fmt.Errorf("新版策略不再使用 IPv6 开关，请分别填写强制 IPv4、IPv6 域名")
	}
	p.IPv4Domains = normalizeDomains(p.IPv4Domains)
	p.IPv6Domains = normalizeDomains(p.IPv6Domains)
	return p, p.Validate()
}
func normalizeDomains(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			out = append(out, v)
			seen[v] = true
		}
	}
	return out
}
func (p RoutingPolicy) Validate() error {
	if p.SchemaVersion > 2 {
		return fmt.Errorf("不支持的路由策略版本")
	}
	if p.IPv6FailurePolicy != "strict" {
		return fmt.Errorf("当前仅支持 strict；prefer_ipv6 尚未通过目标版本兼容验证")
	}
	for _, list := range [][]string{p.IPv4Domains, p.IPv6Domains} {
		if len(list) > 1000 {
			return fmt.Errorf("域名列表最多 1000 项")
		}
		for _, v := range list {
			if err := validateDomain(v); err != nil {
				return err
			}
		}
	}
	if len(p.CustomRules) > 200 {
		return fmt.Errorf("自定义规则最多 200 条")
	}
	for i, r := range p.CustomRules {
		fail := func(msg string) error { return fmt.Errorf("custom_rules[%d]: %s", i, msg) }
		if r.Type != "field" {
			return fail("type 必须为 field")
		}
		if len(r.InboundTag) > 0 || len(r.Source) > 0 {
			return fail("不允许修改系统入站或来源保护规则")
		}
		if r.OutboundTag != "direct-v4" && r.OutboundTag != "block" && !(r.OutboundTag == "direct-v6" && (p.SchemaVersion == 2 || p.EnableIPv6)) {
			return fail("引用了未知或未启用的出站")
		}
		if len(r.Domain) == 0 && len(r.IP) == 0 && r.Port == "" && len(r.Protocol) == 0 {
			return fail("必须限定目标，不能添加兜底规则")
		}
		for _, v := range r.Domain {
			if err := validateDomain(v); err != nil {
				return fail(err.Error())
			}
		}
		for _, v := range r.IP {
			if net.ParseIP(v) == nil {
				if _, _, err := net.ParseCIDR(v); err != nil {
					return fail("IP 必须是地址或 CIDR")
				}
			}
		}
		if r.Network != "" && r.Network != "tcp" && r.Network != "udp" && r.Network != "tcp,udp" {
			return fail("network 无效")
		}
		for _, v := range r.Protocol {
			if v != "http" && v != "tls" && v != "bittorrent" {
				return fail("protocol 无效")
			}
		}
		if r.Port != "" {
			for _, part := range strings.Split(r.Port, ",") {
				bounds := strings.Split(part, "-")
				if len(bounds) > 2 {
					return fail("port 无效")
				}
				last := 0
				for _, b := range bounds {
					n, e := strconv.Atoi(b)
					if e != nil || n < 1 || n > 65535 || n < last {
						return fail("port 无效")
					}
					last = n
				}
			}
		}
	}
	return nil
}
func validateDomain(v string) error {
	if len(v) > 512 || strings.TrimSpace(v) != v {
		return fmt.Errorf("无效域名匹配: %q", v)
	}
	if strings.HasPrefix(v, "regexp:") {
		if len(v) == 7 {
			return fmt.Errorf("正则表达式不能为空")
		}
		_, err := regexp.Compile(strings.TrimPrefix(v, "regexp:"))
		return err
	}
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 || (parts[0] != "domain" && parts[0] != "full") {
		return fmt.Errorf("域名须使用 domain:、full: 或 regexp: 前缀")
	}
	if parts[1] == "" || strings.ContainsAny(parts[1], " /\\\n\r\t@?#") || net.ParseIP(parts[1]) != nil {
		return fmt.Errorf("无效域名: %q", v)
	}
	for _, label := range strings.Split(parts[1], ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("无效域名: %q", v)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("域名请使用 ASCII/punycode")
			}
		}
	}
	return nil
}

// Missing/null inherits; arrays replace rather than merge.
func MergeRoutingPolicy(base, overrides string) (RoutingPolicy, error) {
	if overrides == "" {
		return DecodeRoutingPolicy(base)
	}
	var b, o map[string]json.RawMessage
	if len(overrides) > 256*1024 {
		return RoutingPolicy{}, fmt.Errorf("覆盖超过 256 KiB")
	}
	if err := json.Unmarshal([]byte(base), &b); err != nil {
		return RoutingPolicy{}, err
	}
	if err := json.Unmarshal([]byte(overrides), &o); err != nil {
		return RoutingPolicy{}, err
	}
	if o == nil {
		return RoutingPolicy{}, fmt.Errorf("覆盖必须是 JSON 对象")
	}
	for k, v := range o {
		switch k {
		case "enable_ipv6_outbound", "ipv4_force_domains", "ipv6_force_domains", "ipv6_failure_policy", "custom_rules":
		default:
			return RoutingPolicy{}, fmt.Errorf("未知覆盖字段 %s", k)
		}
		if !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			if k == "enable_ipv6_outbound" && string(b["schema_version"]) == "2" {
				return RoutingPolicy{}, fmt.Errorf("新版策略不接受旧版 IPv6 开关覆盖，请移除该覆盖并配置强制域名列表")
			}
			b[k] = v
		}
	}
	raw, _ := json.Marshal(b)
	return DecodeRoutingPolicy(string(raw))
}

// RequiresIPv6 reflects an actual IPv6 route, not a server-wide capability switch.
func (p RoutingPolicy) RequiresIPv6() bool {
	if p.SchemaVersion < 2 {
		return p.EnableIPv6
	}
	if len(p.IPv6Domains) > 0 {
		return true
	}
	for _, rule := range p.CustomRules {
		if rule.OutboundTag == "direct-v6" {
			return true
		}
	}
	return false
}
func ApplyRoutingPolicy(content string, p RoutingPolicy) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	var cfg Config
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return "", err
	}
	if cfg.Routing == nil {
		return "", fmt.Errorf("缺少系统路由")
	}
	// Retain API and existing proxy allowlists, including explicit allow_private.

	cfg.Routing.Rules = append(cfg.Routing.Rules, p.CustomRules...)
	if len(p.IPv4Domains) > 0 {
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Domain: p.IPv4Domains, OutboundTag: "direct-v4"})
	}
	if len(p.IPv6Domains) > 0 {
		tag := "direct-v4"
		if p.SchemaVersion == 2 || p.EnableIPv6 {
			tag = "direct-v6"
		}
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Domain: p.IPv6Domains, OutboundTag: tag})
	}
	if p.SchemaVersion < 2 {
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Network: "tcp,udp", OutboundTag: "direct-v4"})
	}
	// No catch-all in schema 2: unmatched traffic uses the first (normal direct)
	// outbound. Keeping IPIfNonMatch intact also preserves system IP checks.
	cfg.Outbounds = []Outbound{{Protocol: "freedom", Tag: "direct"}, {Protocol: "blackhole", Tag: "block"}}
	useV4 := p.SchemaVersion < 2 || len(p.IPv4Domains) > 0
	for _, rule := range p.CustomRules {
		if rule.OutboundTag == "direct-v4" {
			useV4 = true
		}
	}
	if useV4 {
		cfg.Outbounds = append(cfg.Outbounds, Outbound{Protocol: "freedom", Tag: "direct-v4", Settings: map[string]string{"domainStrategy": "ForceIPv4"}})
	}
	if p.SchemaVersion < 2 {
		// Historical snapshots must retain their exact rendering and hashes.
		cfg.Outbounds = []Outbound{{Protocol: "freedom", Tag: "direct", Settings: map[string]string{"domainStrategy": "AsIs"}}, {Protocol: "freedom", Tag: "direct-v4", Settings: map[string]string{"domainStrategy": "ForceIPv4"}}, {Protocol: "blackhole", Tag: "block"}}
	}
	if p.RequiresIPv6() {
		cfg.Outbounds = append(cfg.Outbounds, Outbound{Protocol: "freedom", Tag: "direct-v6", Settings: map[string]string{"domainStrategy": "ForceIPv6"}})
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	return string(data), err
}
