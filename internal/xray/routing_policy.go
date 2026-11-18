package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	geositeTagRegex = regexp.MustCompile(`^[A-Za-z0-9_-]+(@[A-Za-z0-9_-]+)*$`)
	geoipTagRegex   = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

type RoutingPolicy struct {
	SchemaVersion     uint             `json:"schema_version"`
	DomainStrategy    string           `json:"domain_strategy,omitempty"`
	EnableIPv6        bool             `json:"enable_ipv6_outbound,omitempty"` // Schema 1 only; retained for historical versions and rollback.
	IPv4Domains       []string         `json:"ipv4_force_domains"`
	IPv6Domains       []string         `json:"ipv6_force_domains"`
	IPv6FailurePolicy string           `json:"ipv6_failure_policy"`
	CustomOutbounds   []CustomOutbound `json:"custom_outbounds,omitempty"`
	Balancers         []Balancer       `json:"balancers,omitempty"`
	Observatory       *Observatory     `json:"observatory,omitempty"`
	DNS               *DNSConfig       `json:"dns,omitempty"`
	CustomRules       []RoutingRule    `json:"custom_rules"`
}

type RenderOptions struct {
	DefaultProbeURL   string
	NodeGeosite       *GeositeDB
	NodeGeositeSHA256 string
}

// VerifiedNodeGeosite 仅在调用方显式提供目标节点 geodata 且 SHA-256 摘要校验一致时返回，绝不隐式读取面板本机文件。
func (o RenderOptions) VerifiedNodeGeosite() *GeositeDB {
	if o.NodeGeosite == nil || strings.TrimSpace(o.NodeGeositeSHA256) == "" {
		return nil
	}
	if !strings.EqualFold(o.NodeGeosite.SHA256(), strings.TrimSpace(o.NodeGeositeSHA256)) {
		return nil
	}
	return o.NodeGeosite
}

type routingPolicyLegacyWire struct {
	SchemaVersion     uint          `json:"schema_version"`
	EnableIPv6        bool          `json:"enable_ipv6_outbound,omitempty"`
	IPv4Domains       []string      `json:"ipv4_force_domains"`
	IPv6Domains       []string      `json:"ipv6_force_domains"`
	IPv6FailurePolicy string        `json:"ipv6_failure_policy"`
	CustomRules       []RoutingRule `json:"custom_rules"`
}

type routingPolicyV3Wire struct {
	SchemaVersion     uint             `json:"schema_version"`
	DomainStrategy    string           `json:"domain_strategy"`
	IPv4Domains       []string         `json:"ipv4_force_domains"`
	IPv6Domains       []string         `json:"ipv6_force_domains"`
	IPv6FailurePolicy string           `json:"ipv6_failure_policy"`
	CustomOutbounds   []CustomOutbound `json:"custom_outbounds"`
	Balancers         []Balancer       `json:"balancers,omitempty"`
	Observatory       *Observatory     `json:"observatory,omitempty"`
	DNS               *DNSConfig       `json:"dns,omitempty"`
	CustomRules       []RoutingRule    `json:"custom_rules"`
}

func (p RoutingPolicy) MarshalJSON() ([]byte, error) {
	v4 := p.IPv4Domains
	if v4 == nil {
		v4 = []string{}
	}
	v6 := p.IPv6Domains
	if v6 == nil {
		v6 = []string{}
	}
	rules := p.CustomRules
	if rules == nil {
		rules = []RoutingRule{}
	}
	if p.SchemaVersion < 3 {
		return json.Marshal(routingPolicyLegacyWire{
			SchemaVersion:     p.SchemaVersion,
			EnableIPv6:        p.EnableIPv6,
			IPv4Domains:       v4,
			IPv6Domains:       v6,
			IPv6FailurePolicy: p.IPv6FailurePolicy,
			CustomRules:       rules,
		})
	}
	ds := p.DomainStrategy
	if ds == "" {
		ds = "IPIfNonMatch"
	}
	outbounds := p.CustomOutbounds
	if outbounds == nil {
		outbounds = []CustomOutbound{}
	}
	return marshalNoEscape(routingPolicyV3Wire{
		SchemaVersion:     p.SchemaVersion,
		DomainStrategy:    ds,
		IPv4Domains:       v4,
		IPv6Domains:       v6,
		IPv6FailurePolicy: p.IPv6FailurePolicy,
		CustomOutbounds:   outbounds,
		Balancers:         p.Balancers,
		Observatory:       p.Observatory,
		DNS:               p.DNS,
		CustomRules:       rules,
	})
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
	if p.SchemaVersion >= 2 && fields["enable_ipv6_outbound"] != nil {
		return p, fmt.Errorf("新版策略不再使用 IPv6 开关，请分别填写强制 IPv4、IPv6 域名")
	}
	if p.SchemaVersion < 3 && (fields["custom_outbounds"] != nil || fields["domain_strategy"] != nil || fields["dns"] != nil || fields["balancers"] != nil || fields["observatory"] != nil) {
		return p, fmt.Errorf("自定义出站、DNS 与负载均衡需使用 schema_version 3")
	}
	if p.SchemaVersion >= 3 {
		if p.DomainStrategy == "" {
			p.DomainStrategy = "IPIfNonMatch"
		}
		if p.CustomOutbounds == nil {
			p.CustomOutbounds = []CustomOutbound{}
		}
	}
	p.IPv4Domains = normalizeDomains(p.IPv4Domains)
	p.IPv6Domains = normalizeDomains(p.IPv6Domains)
	if p.CustomRules == nil {
		p.CustomRules = []RoutingRule{}
	}
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
	if p.SchemaVersion == 0 || p.SchemaVersion > 3 {
		return fmt.Errorf("不支持的路由策略版本")
	}
	if p.SchemaVersion < 3 && (p.DomainStrategy != "" || len(p.CustomOutbounds) > 0 || p.DNS != nil || len(p.Balancers) > 0 || p.Observatory != nil) {
		return fmt.Errorf("自定义出站、DNS 与负载均衡需使用 schema_version 3")
	}
	if p.SchemaVersion >= 3 {
		ds := p.DomainStrategy
		if ds == "" {
			ds = "IPIfNonMatch"
		}
		if ds != "AsIs" && ds != "IPIfNonMatch" && ds != "IPOnDemand" {
			return fmt.Errorf("domain_strategy 仅支持 AsIs、IPIfNonMatch 或 IPOnDemand")
		}
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
	if err := validateDNSConfig(p.DNS); err != nil {
		return err
	}
	if err := validateCustomOutbounds(p.CustomOutbounds); err != nil {
		return err
	}
	allowDirectV6 := p.SchemaVersion >= 2 || p.EnableIPv6
	if err := validateBalancersAndObservatory(p.Balancers, p.Observatory, p.CustomOutbounds, allowDirectV6); err != nil {
		return err
	}

	customTags := make(map[string]bool, len(p.CustomOutbounds))
	for _, co := range p.CustomOutbounds {
		customTags[co.Tag] = true
	}
	balancerTags := make(map[string]bool, len(p.Balancers))
	for _, bal := range p.Balancers {
		balancerTags[bal.Tag] = true
	}

	if len(p.CustomRules) > 200 {
		return fmt.Errorf("自定义规则最多 200 条")
	}
	for i, r := range p.CustomRules {
		if p.SchemaVersion < 3 && r.BalancerTag != "" {
			return fmt.Errorf("custom_rules[%d]: balancerTag 需使用 schema_version 3", i)
		}
		if err := validateRoutingRuleFields(r, customTags, balancerTags, false); err != nil {
			return fmt.Errorf("custom_rules[%d]: %s", i, err.Error())
		}
		if r.OutboundTag == "direct-v6" && !allowDirectV6 {
			return fmt.Errorf("custom_rules[%d]: 引用了未知或未启用的出站", i)
		}
	}
	return nil
}

func validateRoutingRuleFields(
	r RoutingRule,
	customTags map[string]bool,
	balancerTags map[string]bool,
	skipOutboundLookup bool,
) error {
	if r.Type != "field" {
		return fmt.Errorf("type 必须为 field")
	}
	if len(r.InboundTag) > 0 || len(r.Source) > 0 {
		return fmt.Errorf("不允许修改系统入站或来源保护规则")
	}
	hasOutbound := r.OutboundTag != ""
	hasBalancer := r.BalancerTag != ""
	if hasOutbound && hasBalancer {
		return fmt.Errorf("outboundTag 与 balancerTag 互斥，只能填写其中一个")
	}
	if !hasOutbound && !hasBalancer {
		return fmt.Errorf("引用了未知或未启用的出站")
	}
	if !skipOutboundLookup {
		if hasOutbound {
			isBuiltin := r.OutboundTag == "direct" || r.OutboundTag == "block" || r.OutboundTag == "direct-v4" || r.OutboundTag == "direct-v6"
			if !isBuiltin && !customTags[r.OutboundTag] {
				return fmt.Errorf("引用了未知或未启用的出站")
			}
		} else if hasBalancer && !balancerTags[r.BalancerTag] {
			return fmt.Errorf("引用了未知的负载均衡组 %q", r.BalancerTag)
		}
	} else if hasBalancer {
		if err := validateOutboundTagFormat(r.BalancerTag); err != nil {
			return err
		}
	}
	if len(r.Domain) == 0 && len(r.IP) == 0 && r.Port == "" && len(r.Protocol) == 0 {
		return fmt.Errorf("必须限定目标，不能添加兜底规则")
	}
	for _, v := range r.Domain {
		if err := validateDomain(v); err != nil {
			return err
		}
	}
	for _, v := range r.IP {
		if err := validateIPRule(v); err != nil {
			return err
		}
	}
	if r.Network != "" && r.Network != "tcp" && r.Network != "udp" && r.Network != "tcp,udp" {
		return fmt.Errorf("network 无效")
	}
	for _, v := range r.Protocol {
		if v != "http" && v != "tls" && v != "quic" && v != "bittorrent" {
			return fmt.Errorf("protocol 无效")
		}
	}
	if r.Port != "" {
		for _, part := range strings.Split(r.Port, ",") {
			bounds := strings.Split(part, "-")
			if len(bounds) > 2 {
				return fmt.Errorf("port 无效")
			}
			last := 0
			for _, b := range bounds {
				n, e := strconv.Atoi(b)
				if e != nil || n < 1 || n > 65535 || n < last {
					return fmt.Errorf("port 无效")
				}
				last = n
			}
		}
	}
	return nil
}

func validateDomain(v string) error {
	if v == "" || len(v) > 512 || strings.TrimSpace(v) != v {
		return fmt.Errorf("无效域名匹配: %q", v)
	}
	if strings.HasPrefix(v, "regexp:") {
		if len(v) == 7 {
			return fmt.Errorf("正则表达式不能为空")
		}
		_, err := regexp.Compile(strings.TrimPrefix(v, "regexp:"))
		return err
	}
	if strings.HasPrefix(v, "geosite:") {
		tag := strings.TrimPrefix(v, "geosite:")
		if !geositeTagRegex.MatchString(tag) {
			return fmt.Errorf("无效 geosite 标签: %q", v)
		}
		return nil
	}
	if strings.HasPrefix(v, "keyword:") {
		kw := strings.TrimPrefix(v, "keyword:")
		if kw == "" || strings.ContainsAny(kw, " \n\r\t") {
			return fmt.Errorf("无效 keyword 匹配: %q", v)
		}
		return nil
	}
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 || (parts[0] != "domain" && parts[0] != "full") {
		return fmt.Errorf("域名须使用 domain:、full:、geosite:、keyword: 或 regexp: 前缀")
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

// parseXrayCIDR 对齐 Xray 26.3.27 / 26.11.9 的 parseCIDR 语义：先将 IPv4-mapped IPv6 还原为 IPv4（前缀上限 32），再应用掩码。
func parseXrayCIDR(s string) (netip.Prefix, error) {
	ipStr, bitsStr, ok := strings.Cut(s, "/")
	if !ok || bitsStr == "" {
		return netip.Prefix{}, fmt.Errorf("无效 CIDR: %q", s)
	}
	for i := 0; i < len(bitsStr); i++ {
		if bitsStr[i] < '0' || bitsStr[i] > '9' {
			return netip.Prefix{}, fmt.Errorf("无效 CIDR 前缀长度: %q", s)
		}
	}
	addr, err := netip.ParseAddr(ipStr)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("无效 CIDR 地址: %q", s)
	}
	addr = addr.Unmap()
	bits, err := strconv.Atoi(bitsStr)
	if err != nil || bits < 0 || bits > addr.BitLen() {
		return netip.Prefix{}, fmt.Errorf("CIDR 前缀长度超出地址族上限: %q", s)
	}
	return netip.PrefixFrom(addr, bits), nil
}

func parseXrayIPOrCIDR(v string) error {
	if strings.Contains(v, "/") {
		_, err := parseXrayCIDR(v)
		return err
	}
	addr, err := netip.ParseAddr(v)
	if err != nil || addr.Zone() != "" {
		return fmt.Errorf("无效 IP 地址: %q", v)
	}
	return nil
}

func validateIPRule(v string) error {
	if v == "" || len(v) > 256 || strings.TrimSpace(v) != v {
		return fmt.Errorf("IP 匹配项无效: %q", v)
	}
	if strings.HasPrefix(v, "geoip:") || strings.HasPrefix(v, "!geoip:") {
		tag := strings.TrimPrefix(strings.TrimPrefix(normalizeGeoIPRule(v), "geoip:"), "!")
		if !geoipTagRegex.MatchString(tag) {
			return fmt.Errorf("无效 geoip 标签: %q", v)
		}
		if v == "geoip:private" {
			return fmt.Errorf("系统已内置私网地址(geoip:private)保护规则，无需在自定义规则中重复配置")
		}
		return nil
	}
	if err := parseXrayIPOrCIDR(v); err != nil {
		return fmt.Errorf("IP 必须是合法地址、CIDR（注意映射地址前缀上限）或 geoip: 标签: %w", err)
	}
	return nil
}

// normalizeGeoIPRule 使用新旧内核都支持的否定 GeoIP 语法。
func normalizeGeoIPRule(v string) string {
	if strings.HasPrefix(v, "!geoip:") {
		return "geoip:!" + strings.TrimPrefix(v, "!geoip:")
	}
	return v
}

func normalizeIPRules(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = normalizeGeoIPRule(v)
	}
	return out
}

// MergeRoutingPolicy 合并基础模板策略与节点级覆盖（含按出站 Tag 的 replace / remap 及节点级 DNS 覆盖）。
func MergeRoutingPolicy(base, overrides string) (RoutingPolicy, error) {
	if overrides == "" || overrides == "{}" {
		return DecodeRoutingPolicy(base)
	}
	if len(overrides) > 256*1024 {
		return RoutingPolicy{}, fmt.Errorf("覆盖超过 256 KiB")
	}
	basePolicy, err := DecodeRoutingPolicy(base)
	if err != nil {
		return RoutingPolicy{}, err
	}
	var b, o map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &b); err != nil {
		return RoutingPolicy{}, err
	}
	dec := json.NewDecoder(strings.NewReader(overrides))
	if err := dec.Decode(&o); err != nil {
		return RoutingPolicy{}, err
	}
	if dec.Decode(new(any)) != io.EOF {
		return RoutingPolicy{}, fmt.Errorf("只允许一个 JSON 对象")
	}
	if o == nil {
		return RoutingPolicy{}, fmt.Errorf("覆盖必须是 JSON 对象")
	}

	var rawOutboundOverrides json.RawMessage
	var rawDNSOverride json.RawMessage
	for k, v := range o {
		trimmed := bytes.TrimSpace(v)
		switch k {
		case "enable_ipv6_outbound", "ipv4_force_domains", "ipv6_force_domains", "ipv6_failure_policy", "custom_rules":
			if !bytes.Equal(trimmed, []byte("null")) {
				if k == "enable_ipv6_outbound" && basePolicy.SchemaVersion >= 2 {
					return RoutingPolicy{}, fmt.Errorf("新版策略不接受旧版 IPv6 开关覆盖，请移除该覆盖并配置强制域名列表")
				}
				b[k] = v
			}
		case "outbound_overrides":
			if !bytes.Equal(trimmed, []byte("null")) {
				rawOutboundOverrides = trimmed
			}
		case "dns_override":
			if !bytes.Equal(trimmed, []byte("null")) {
				if basePolicy.SchemaVersion < 3 {
					return RoutingPolicy{}, fmt.Errorf("DNS 覆盖需先将所绑定模板升级为 schema_version 3")
				}
				rawDNSOverride = trimmed
			}
		default:
			return RoutingPolicy{}, fmt.Errorf("未知覆盖字段 %s", k)
		}
	}

	mergedRaw, err := json.Marshal(b)
	if err != nil {
		return RoutingPolicy{}, err
	}
	p, err := DecodeRoutingPolicy(string(mergedRaw))
	if err != nil {
		return RoutingPolicy{}, err
	}

	if len(rawOutboundOverrides) > 0 {
		ovMap, ovErr := decodeOutboundOverridesMap(rawOutboundOverrides)
		if ovErr != nil {
			return RoutingPolicy{}, ovErr
		}
		if len(ovMap) > 0 {
			baseTagIdx := make(map[string]int, len(p.CustomOutbounds))
			for i, co := range p.CustomOutbounds {
				baseTagIdx[co.Tag] = i
			}

			keys := make([]string, 0, len(ovMap))
			for tag := range ovMap {
				keys = append(keys, tag)
			}
			sort.Strings(keys)

			allowDirectV6 := p.SchemaVersion >= 2 || p.EnableIPv6
			remaps := make(map[string]string)
			for _, tag := range keys {
				ov := ovMap[tag]
				if _, exists := baseTagIdx[tag]; !exists {
					return RoutingPolicy{}, fmt.Errorf("outbound_overrides[%s]: 当前绑定模板版本不存在该出站，请清理或更新节点覆盖", tag)
				}
				if err := validateSingleOutboundOverrideShape(tag, ov); err != nil {
					return RoutingPolicy{}, err
				}
				if ov.Mode == "remap" {
					target := ov.TargetTag
					if target == "direct-v6" && !allowDirectV6 {
						return RoutingPolicy{}, fmt.Errorf("outbound_overrides[%s]: target_tag 引用了未启用的 direct-v6", tag)
					}
					isBuiltin := target == "direct" || target == "block" || target == "direct-v4" || target == "direct-v6"
					if !isBuiltin {
						if _, exists := baseTagIdx[target]; !exists {
							return RoutingPolicy{}, fmt.Errorf("outbound_overrides[%s]: target_tag 引用了不存在的自定义出站 %q", tag, target)
						}
						if targetOv, hasOv := ovMap[target]; hasOv && targetOv.Mode == "remap" {
							return RoutingPolicy{}, fmt.Errorf("outbound_overrides[%s]: 禁止多跳链式 remap (%s -> %s)", tag, tag, target)
						}
					}
					remaps[tag] = target
				}
			}

			for _, tag := range keys {
				ov := ovMap[tag]
				if ov.Mode == "replace" && ov.Outbound != nil {
					p.CustomOutbounds[baseTagIdx[tag]] = *ov.Outbound
				}
			}

			if len(remaps) > 0 {
				for i := range p.CustomRules {
					if target, ok := remaps[p.CustomRules[i].OutboundTag]; ok {
						p.CustomRules[i].OutboundTag = target
					}
				}
				kept := make([]CustomOutbound, 0, len(p.CustomOutbounds))
				for _, co := range p.CustomOutbounds {
					if _, isRemapped := remaps[co.Tag]; isRemapped {
						continue
					}
					if dp, err := extractDialerProxy(co.StreamSettings); err == nil && dp != "" {
						if target, ok := remaps[dp]; ok {
							rewritten, rwErr := rewriteDialerProxy(co.StreamSettings, target)
							if rwErr != nil {
								return RoutingPolicy{}, rwErr
							}
							co.StreamSettings = rewritten
						}
					}
					kept = append(kept, co)
				}
				p.CustomOutbounds = kept
				if err := adjustBalancersAfterRemap(&p, remaps); err != nil {
					return RoutingPolicy{}, err
				}
			}
		}
	}

	if len(rawDNSOverride) > 0 {
		dnsOv, dnsErr := decodeDNSOverride(rawDNSOverride)
		if dnsErr != nil {
			return RoutingPolicy{}, dnsErr
		}
		p.DNS = dnsOv
	}

	if err := p.Validate(); err != nil {
		return RoutingPolicy{}, err
	}
	return p, nil
}

// RequiresIPv6 判定该路由策略是否需要节点本机具备可用 IPv6 默认出口路由。
func (p RoutingPolicy) RequiresIPv6() bool {
	return p.RequiresIPv6WithOptions(RenderOptions{})
}

// RequiresIPv6WithOptions 与最终渲染共用探针设置，避免遗漏自动观测器的本机 IPv6 依赖。
func (p RoutingPolicy) RequiresIPv6WithOptions(opts RenderOptions) bool {
	if p.SchemaVersion < 2 {
		return p.EnableIPv6
	}
	return routingPolicyRequiresIPv6(p, opts)
}

func ApplyRoutingPolicy(content string, p RoutingPolicy) (string, error) {
	return ApplyRoutingPolicyWithOptions(content, p, RenderOptions{})
}

func ApplyRoutingPolicyWithOptions(content string, p RoutingPolicy, opts RenderOptions) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	decrypted, err := DecryptPolicySecrets(p)
	if err != nil {
		return "", err
	}
	p = decrypted

	var cfg Config
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		return "", err
	}
	if cfg.Routing == nil {
		return "", fmt.Errorf("缺少系统路由")
	}
	if err := checkDNSInboundPortConflictWithGeosite(p.DNS, cfg.Inbounds, opts.VerifiedNodeGeosite()); err != nil {
		return "", err
	}
	if p.SchemaVersion >= 3 {
		ds := p.DomainStrategy
		if ds == "" {
			ds = "IPIfNonMatch"
		}
		cfg.Routing.DomainStrategy = ds
	}

	activeBalancerTags := make(map[string]bool)
	// Retain API and existing proxy allowlists, including explicit allow_private.
	for _, r := range p.CustomRules {
		if r.Disabled {
			continue
		}
		if r.BalancerTag != "" {
			activeBalancerTags[r.BalancerTag] = true
		}
		r.Remark = ""
		r.Disabled = false
		r.IP = normalizeIPRules(r.IP)
		cfg.Routing.Rules = append(cfg.Routing.Rules, r)
	}
	if len(p.IPv4Domains) > 0 {
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Domain: p.IPv4Domains, OutboundTag: "direct-v4"})
	}
	if len(p.IPv6Domains) > 0 {
		tag := "direct-v4"
		if p.SchemaVersion >= 2 || p.EnableIPv6 {
			tag = "direct-v6"
		}
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Domain: p.IPv6Domains, OutboundTag: tag})
	}

	if p.SchemaVersion < 2 {
		cfg.Routing.Rules = append(cfg.Routing.Rules, RoutingRule{Type: "field", Network: "tcp,udp", OutboundTag: "direct-v4"})
		// Historical snapshots must retain their exact rendering and hashes.
		cfg.Outbounds = []Outbound{
			{Protocol: "freedom", Tag: "direct", Settings: map[string]string{"domainStrategy": "AsIs"}},
			{Protocol: "freedom", Tag: "direct-v4", Settings: map[string]string{"domainStrategy": "ForceIPv4"}},
			{Protocol: "blackhole", Tag: "block"},
		}
		if p.RequiresIPv6() {
			cfg.Outbounds = append(cfg.Outbounds, Outbound{Protocol: "freedom", Tag: "direct-v6", Settings: map[string]string{"domainStrategy": "ForceIPv6"}})
		}
	} else {
		refs := p.referencedOutboundTags()
		directOutbound := Outbound{Protocol: "freedom", Tag: "direct"}
		if p.DNS != nil && p.DNS.Enabled {
			cfg.DNS = renderRuntimeDNS(p.DNS)
			// 自动打通内置 freedom direct 解析链路，使服务端自定义 DNS 与 hosts 即时生效
			directStrategy := "UseIP"
			if p.DNS.QueryStrategy == "UseIPv4" || p.DNS.QueryStrategy == "UseIPv6" {
				directStrategy = p.DNS.QueryStrategy
			}
			directOutbound.Settings = map[string]string{"domainStrategy": directStrategy}
		} else {
			cfg.DNS = nil
		}

		cfg.Outbounds = []Outbound{
			directOutbound,
			{Protocol: "blackhole", Tag: "block"},
		}
		if refs["direct-v4"] {
			cfg.Outbounds = append(cfg.Outbounds, Outbound{Protocol: "freedom", Tag: "direct-v4", Settings: map[string]string{"domainStrategy": "ForceIPv4"}})
		}
		if refs["direct-v6"] {
			cfg.Outbounds = append(cfg.Outbounds, Outbound{Protocol: "freedom", Tag: "direct-v6", Settings: map[string]string{"domainStrategy": "ForceIPv6"}})
		}
		for _, co := range p.CustomOutbounds {
			if refs[co.Tag] {
				cfg.Outbounds = append(cfg.Outbounds, co.toRuntimeOutbound())
			}
		}

		balancers, obs := renderRuntimeBalancersAndObservatory(p, activeBalancerTags, opts.DefaultProbeURL)
		cfg.Routing.Balancers = balancers
		if obs != nil {
			cfg.Observatory = obs
		} else {
			cfg.Observatory = nil
		}
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	return string(data), err
}
