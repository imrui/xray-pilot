package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	geositeTagRegex = regexp.MustCompile(`^[A-Za-z0-9_@-]+$`)
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
	if p.SchemaVersion < 3 && (fields["custom_outbounds"] != nil || fields["domain_strategy"] != nil) {
		return p, fmt.Errorf("自定义出站与域名解析策略需使用 schema_version 3")
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
	if p.DNS != nil || len(p.Balancers) > 0 || p.Observatory != nil {
		return fmt.Errorf("DNS 与负载均衡将在后续阶段启用")
	}
	for _, r := range p.CustomRules {
		if r.BalancerTag != "" {
			return fmt.Errorf("DNS 与负载均衡将在后续阶段启用")
		}
	}
	if p.SchemaVersion < 3 && (p.DomainStrategy != "" || len(p.CustomOutbounds) > 0) {
		return fmt.Errorf("自定义出站与域名解析策略需使用 schema_version 3")
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
	if err := validateCustomOutbounds(p.CustomOutbounds); err != nil {
		return err
	}
	customTags := make(map[string]bool, len(p.CustomOutbounds))
	for _, co := range p.CustomOutbounds {
		customTags[co.Tag] = true
	}
	if len(p.CustomRules) > 200 {
		return fmt.Errorf("自定义规则最多 200 条")
	}
	allowDirectV6 := p.SchemaVersion >= 2 || p.EnableIPv6
	for i, r := range p.CustomRules {
		if err := validateRoutingRuleFields(r, customTags, false); err != nil {
			return fmt.Errorf("custom_rules[%d]: %s", i, err.Error())
		}
		if r.OutboundTag == "direct-v6" && !allowDirectV6 {
			return fmt.Errorf("custom_rules[%d]: 引用了未知或未启用的出站", i)
		}
	}
	return nil
}

func validateRoutingRuleFields(r RoutingRule, customTags map[string]bool, skipOutboundLookup bool) error {
	if r.BalancerTag != "" {
		return fmt.Errorf("DNS 与负载均衡将在后续阶段启用")
	}
	if r.Type != "field" {
		return fmt.Errorf("type 必须为 field")
	}
	if len(r.InboundTag) > 0 || len(r.Source) > 0 {
		return fmt.Errorf("不允许修改系统入站或来源保护规则")
	}
	if r.OutboundTag == "" {
		return fmt.Errorf("引用了未知或未启用的出站")
	}
	if !skipOutboundLookup {
		isBuiltin := r.OutboundTag == "direct" || r.OutboundTag == "block" || r.OutboundTag == "direct-v4" || r.OutboundTag == "direct-v6"
		if !isBuiltin && !customTags[r.OutboundTag] {
			return fmt.Errorf("引用了未知或未启用的出站")
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

func validateIPRule(v string) error {
	if v == "" || len(v) > 256 || strings.TrimSpace(v) != v {
		return fmt.Errorf("IP 匹配项无效: %q", v)
	}
	if strings.HasPrefix(v, "geoip:") || strings.HasPrefix(v, "!geoip:") {
		tag := strings.TrimPrefix(strings.TrimPrefix(v, "!"), "geoip:")
		if !geoipTagRegex.MatchString(tag) {
			return fmt.Errorf("无效 geoip 标签: %q", v)
		}
		if v == "geoip:private" {
			return fmt.Errorf("系统已内置私网地址(geoip:private)保护规则，无需在自定义规则中重复配置")
		}
		return nil
	}
	if net.ParseIP(v) == nil {
		if _, _, err := net.ParseCIDR(v); err != nil {
			return fmt.Errorf("IP 必须是地址、CIDR 或 geoip: 标签")
		}
	}
	return nil
}

// MergeRoutingPolicy 合并基础模板策略与节点级覆盖（含按出站 Tag 的 replace / remap）。
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
				return RoutingPolicy{}, fmt.Errorf("DNS 与负载均衡将在后续阶段启用")
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
	if len(rawOutboundOverrides) == 0 {
		return p, nil
	}

	ovMap, err := decodeOutboundOverridesMap(rawOutboundOverrides)
	if err != nil {
		return RoutingPolicy{}, err
	}
	if len(ovMap) == 0 {
		return p, nil
	}

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
	}

	if err := p.Validate(); err != nil {
		return RoutingPolicy{}, err
	}
	return p, nil
}

// RequiresIPv6 判定该路由策略是否需要节点本机具备可用 IPv6 默认出口路由。
func (p RoutingPolicy) RequiresIPv6() bool {
	if p.SchemaVersion < 2 {
		return p.EnableIPv6
	}
	refs := p.referencedOutboundTags()
	if refs["direct-v6"] {
		return true
	}
	for _, co := range p.CustomOutbounds {
		if refs[co.Tag] && outboundRequiresIPv6(co) {
			return true
		}
	}
	return false
}

func ApplyRoutingPolicy(content string, p RoutingPolicy) (string, error) {
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
	if p.SchemaVersion >= 3 {
		ds := p.DomainStrategy
		if ds == "" {
			ds = "IPIfNonMatch"
		}
		cfg.Routing.DomainStrategy = ds
	}
	// Retain API and existing proxy allowlists, including explicit allow_private.
	for _, r := range p.CustomRules {
		if r.Disabled {
			continue
		}
		r.Remark = ""
		r.Disabled = false
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
		// No catch-all in schema >= 2: unmatched traffic uses the first (normal direct) outbound.
		refs := p.referencedOutboundTags()
		cfg.Outbounds = []Outbound{
			{Protocol: "freedom", Tag: "direct"},
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
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	return string(data), err
}
