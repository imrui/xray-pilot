package xray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

const (
	maxDNSServers = 16
	maxDNSHosts   = 200
	maxBalancers  = 16
)

var allowedDNSURIPrefixes = []string{
	"https://",
	"https+local://",
	"tcp://",
	"tcp+local://",
	"quic+local://",
}

// DNSServerObject 表示分流模式下的单条上游 DNS 服务器对象。
type DNSServerObject struct {
	Address       string                     `json:"address"`
	Port          int                        `json:"port,omitempty"`
	Domains       []string                   `json:"domains,omitempty"`
	ExpectIPs     []string                   `json:"expectIPs,omitempty"`
	SkipFallback  bool                       `json:"skipFallback,omitempty"`
	QueryStrategy string                     `json:"queryStrategy,omitempty"`
	Extra         map[string]json.RawMessage `json:"-"`
}

type dnsServerAlias DNSServerObject

var dnsServerKnownKeys = map[string]string{
	"address":       "address",
	"port":          "port",
	"domains":       "domains",
	"expectips":     "expectIPs",
	"skipfallback":  "skipFallback",
	"querystrategy": "queryStrategy",
}

func (s *DNSServerObject) UnmarshalJSON(data []byte) error {
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	if rawMap == nil {
		return fmt.Errorf("DNS 服务器条目必须是字符串或 JSON 对象")
	}
	var alias dnsServerAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	var extra map[string]json.RawMessage
	for k, v := range rawMap {
		lower := strings.ToLower(k)
		if canonical, ok := dnsServerKnownKeys[lower]; ok {
			if k != canonical {
				return fmt.Errorf("DNS 服务器字段 %q 请使用标准命名 %q", k, canonical)
			}
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[k] = v
	}
	*s = DNSServerObject(alias)
	s.Extra = extra
	return nil
}

func (s DNSServerObject) MarshalJSON() ([]byte, error) {
	base, err := marshalNoEscape(dnsServerAlias(s))
	if err != nil || len(s.Extra) == 0 {
		return base, err
	}
	merged := make(map[string]json.RawMessage, len(s.Extra)+6)
	for k, v := range s.Extra {
		if _, isKnown := dnsServerKnownKeys[strings.ToLower(k)]; !isKnown {
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

func validateBareHostname(host string) error {
	if host == "localhost" {
		return nil
	}
	if host == "" || len(host) > 253 || strings.ContainsAny(host, " /\\\n\r\t@?#") || net.ParseIP(host) != nil {
		return fmt.Errorf("无效主机名: %q", host)
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("无效主机名: %q", host)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("域名请使用 ASCII/punycode: %q", host)
			}
		}
	}
	return nil
}

func validateDNSQueryStrategy(qs string) error {
	if qs != "" && qs != "UseIP" && qs != "UseIPv4" && qs != "UseIPv6" {
		return fmt.Errorf("DNS queryStrategy 仅支持 UseIP、UseIPv4 或 UseIPv6")
	}
	return nil
}

// parseDNSAddressAndLoopbackPort 校验 DNS address 并在命中本机回环地址时返回其目标端口（用于自环防护）。
func parseDNSAddressAndLoopbackPort(address string, explicitPort int) (loopbackPort int, isURI bool, isLocalhost bool, err error) {
	if address == "" || strings.TrimSpace(address) != address || strings.ContainsAny(address, " \n\r\t") {
		return 0, false, false, fmt.Errorf("DNS 服务器 address 不能为空或含空白字符")
	}
	if address == "localhost" {
		if explicitPort != 0 {
			return 0, false, true, fmt.Errorf("localhost 模式不支持单独指定 port")
		}
		return 0, false, true, nil
	}
	for _, prefix := range allowedDNSURIPrefixes {
		if strings.HasPrefix(address, prefix) {
			isURI = true
			// 将 +local 替换为标准 scheme 以便 url.Parse 校验主机与端口
			normalizedURI := strings.Replace(address, "+local://", "://", 1)
			u, parseErr := url.Parse(normalizedURI)
			if parseErr != nil || u.Hostname() == "" || u.User != nil {
				return 0, true, false, fmt.Errorf("无效 DNS URI: %q", address)
			}
			isHTTPS := strings.HasPrefix(address, "https://") || strings.HasPrefix(address, "https+local://")
			if isHTTPS && explicitPort != 0 {
				return 0, true, false, fmt.Errorf("DoH (%s) 请直接在 URI 中指定端口，不要同时设置 port 字段", prefix)
			}
			host := u.Hostname()
			port := explicitPort
			if u.Port() != "" {
				pNum, pErr := strconv.Atoi(u.Port())
				if pErr != nil || pNum < 1 || pNum > 65535 {
					return 0, true, false, fmt.Errorf("DNS URI 端口无效: %q", address)
				}
				port = pNum
			}
			if port == 0 {
				if isHTTPS {
					port = 443
				} else if strings.HasPrefix(address, "quic+local://") {
					port = 853
				} else {
					port = 53
				}
			}
			if ip := net.ParseIP(host); (ip != nil && ip.IsLoopback()) || strings.EqualFold(host, "localhost") {
				return port, true, false, nil
			}
			return 0, true, false, nil
		}
	}
	if strings.Contains(address, "://") {
		return 0, true, false, fmt.Errorf("不支持的 DNS URI 协议前缀: %q", address)
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return 0, false, false, fmt.Errorf("DNS address 须为合法 IP、localhost 或受支持的加密/TCP DNS URI: %q", address)
	}
	port := explicitPort
	if port == 0 {
		port = 53
	}
	if ip.IsLoopback() {
		return port, false, false, nil
	}
	return 0, false, false, nil
}

func validateExpectIP(v string) error {
	if v == "" || len(v) > 256 || strings.TrimSpace(v) != v {
		return fmt.Errorf("expectIPs 条目无效: %q", v)
	}
	if strings.HasPrefix(v, "geoip:") || strings.HasPrefix(v, "!geoip:") {
		tag := strings.TrimPrefix(strings.TrimPrefix(v, "!"), "geoip:")
		if !geoipTagRegex.MatchString(tag) {
			return fmt.Errorf("无效 geoip 标签: %q", v)
		}
		return nil
	}
	if net.ParseIP(v) == nil {
		if _, _, err := net.ParseCIDR(v); err != nil {
			return fmt.Errorf("expectIPs 必须是 IP、CIDR 或 geoip: 标签: %q", v)
		}
	}
	return nil
}

func validateDNSConfig(dns *DNSConfig) error {
	if dns == nil {
		return nil
	}
	if err := validateDNSQueryStrategy(dns.QueryStrategy); err != nil {
		return err
	}
	if len(dns.Hosts) > maxDNSHosts {
		return fmt.Errorf("DNS hosts 映射最多 %d 条", maxDNSHosts)
	}
	hostKeys := make([]string, 0, len(dns.Hosts))
	for k := range dns.Hosts {
		hostKeys = append(hostKeys, k)
	}
	sort.Strings(hostKeys)
	for _, k := range hostKeys {
		if strings.Contains(k, ":") {
			if !strings.HasPrefix(k, "domain:") && !strings.HasPrefix(k, "full:") && !strings.HasPrefix(k, "geosite:") {
				return fmt.Errorf("DNS hosts 键 %q 仅支持域名、domain:、full: 或 geosite:", k)
			}
			if err := validateDomain(k); err != nil {
				return fmt.Errorf("DNS hosts[%s]: %w", k, err)
			}
		} else if err := validateBareHostname(k); err != nil {
			return fmt.Errorf("DNS hosts[%s]: %w", k, err)
		}
		rawVal := bytes.TrimSpace(dns.Hosts[k])
		if len(rawVal) == 0 {
			return fmt.Errorf("DNS hosts[%s]: 值不能为空", k)
		}
		if rawVal[0] == '"' {
			var single string
			if err := json.Unmarshal(rawVal, &single); err != nil || net.ParseIP(single) == nil {
				return fmt.Errorf("DNS hosts[%s]: 值必须是合法 IPv4/IPv6 字面量", k)
			}
		} else if rawVal[0] == '[' {
			var arr []string
			if err := json.Unmarshal(rawVal, &arr); err != nil || len(arr) == 0 {
				return fmt.Errorf("DNS hosts[%s]: IP 数组不能为空", k)
			}
			for _, ipStr := range arr {
				if net.ParseIP(ipStr) == nil {
					return fmt.Errorf("DNS hosts[%s]: %q 不是合法 IPv4/IPv6 字面量", k, ipStr)
				}
			}
		} else {
			return fmt.Errorf("DNS hosts[%s]: 值必须是单个 IP 字符串或 IP 字符串数组", k)
		}
	}

	if dns.Enabled && len(dns.Servers) == 0 {
		return fmt.Errorf("启用自定义 DNS 时至少需要配置 1 个上游 DNS 服务器")
	}
	if len(dns.Servers) > maxDNSServers {
		return fmt.Errorf("DNS servers 最多 %d 项", maxDNSServers)
	}
	for i, srvRaw := range dns.Servers {
		trimmed := bytes.TrimSpace(srvRaw)
		if len(trimmed) == 0 {
			return fmt.Errorf("dns.servers[%d]: 不能为空", i)
		}
		if trimmed[0] == '"' {
			var s string
			if err := json.Unmarshal(trimmed, &s); err != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, err)
			}
			loopPort, _, _, addrErr := parseDNSAddressAndLoopbackPort(s, 0)
			if addrErr != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, addrErr)
			}
			if loopPort == 10085 {
				return fmt.Errorf("dns.servers[%d]: 禁止指向面板本地管理端口 10085", i)
			}
		} else if trimmed[0] == '{' {
			var srvObj DNSServerObject
			if err := json.Unmarshal(trimmed, &srvObj); err != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, err)
			}
			if srvObj.Port != 0 {
				if err := validatePortRange(srvObj.Port); err != nil {
					return fmt.Errorf("dns.servers[%d]: %w", i, err)
				}
			}
			loopPort, _, _, addrErr := parseDNSAddressAndLoopbackPort(srvObj.Address, srvObj.Port)
			if addrErr != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, addrErr)
			}
			if loopPort == 10085 {
				return fmt.Errorf("dns.servers[%d]: 禁止指向面板本地管理端口 10085", i)
			}
			if err := validateDNSQueryStrategy(srvObj.QueryStrategy); err != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, err)
			}
			for _, d := range srvObj.Domains {
				if err := validateDomain(d); err != nil {
					return fmt.Errorf("dns.servers[%d].domains: %w", i, err)
				}
			}
			for _, eip := range srvObj.ExpectIPs {
				if err := validateExpectIP(eip); err != nil {
					return fmt.Errorf("dns.servers[%d].expectIPs: %w", i, err)
				}
			}
		} else {
			return fmt.Errorf("dns.servers[%d]: 必须是字符串或服务器配置对象", i)
		}
	}
	return nil
}

func checkDNSInboundPortConflict(dns *DNSConfig, inbounds []Inbound) error {
	if dns == nil || !dns.Enabled {
		return nil
	}
	inboundPorts := make(map[int]string, len(inbounds))
	for _, in := range inbounds {
		if in.Port > 0 {
			inboundPorts[in.Port] = in.Tag
		}
	}
	inboundPorts[10085] = "api-inbound"

	for i, srvRaw := range dns.Servers {
		trimmed := bytes.TrimSpace(srvRaw)
		var loopPort int
		if len(trimmed) > 0 && trimmed[0] == '"' {
			var s string
			if json.Unmarshal(trimmed, &s) == nil {
				loopPort, _, _, _ = parseDNSAddressAndLoopbackPort(s, 0)
			}
		} else if len(trimmed) > 0 && trimmed[0] == '{' {
			var obj DNSServerObject
			if json.Unmarshal(trimmed, &obj) == nil {
				loopPort, _, _, _ = parseDNSAddressAndLoopbackPort(obj.Address, obj.Port)
			}
		}
		if loopPort > 0 {
			if ownerTag, conflict := inboundPorts[loopPort]; conflict {
				return fmt.Errorf("dns.servers[%d]: 本机回环端口 %d 与入站 %q 冲突，已拦截自环解析", i, loopPort, ownerTag)
			}
		}
	}
	return nil
}

func renderRuntimeDNS(dns *DNSConfig) map[string]any {
	if dns == nil || !dns.Enabled {
		return nil
	}
	out := make(map[string]any)
	for k, v := range dns.Extra {
		if strings.ToLower(k) != "enabled" {
			out[k] = v
		}
	}
	qs := dns.QueryStrategy
	if qs == "" {
		qs = "UseIP"
	}
	out["queryStrategy"] = qs
	if dns.DisableCache {
		out["disableCache"] = true
	}
	if dns.DisableFallback {
		out["disableFallback"] = true
	}
	if dns.DisableFallbackIfMatch {
		out["disableFallbackIfMatch"] = true
	}
	if len(dns.Hosts) > 0 {
		out["hosts"] = dns.Hosts
	}
	servers := make([]any, 0, len(dns.Servers))
	for _, raw := range dns.Servers {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			var obj DNSServerObject
			if json.Unmarshal(trimmed, &obj) == nil {
				// 省略空的 domains / expectIPs 切片，保持与 Xray 原生行为一致
				if len(obj.Domains) == 0 {
					obj.Domains = nil
				}
				if len(obj.ExpectIPs) == 0 {
					obj.ExpectIPs = nil
				}
				servers = append(servers, obj)
				continue
			}
		}
		servers = append(servers, raw)
	}
	out["servers"] = servers
	return out
}

func decodeDNSOverride(raw json.RawMessage) (*DNSConfig, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg DNSConfig
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("dns_override: %w", err)
	}
	if dec.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("dns_override: 只允许一个 JSON 对象")
	}
	if err := validateDNSConfig(&cfg); err != nil {
		return nil, fmt.Errorf("dns_override: %w", err)
	}
	return &cfg, nil
}

// ---- Balancers 与 Observatory 校验与自动补齐 ----

func matchSelectorCandidates(selectors []string, candidates []string) ([]string, error) {
	matchedSet := make(map[string]bool)
	for _, sel := range selectors {
		sel = strings.TrimSpace(sel)
		if sel == "" {
			return nil, fmt.Errorf("选择器条目不能为空")
		}
		if strings.HasPrefix("block", sel) || strings.HasPrefix("api", sel) || strings.HasPrefix("api-inbound", sel) {
			return nil, fmt.Errorf("选择器 %q 不能匹配系统保留出站 block 或 api", sel)
		}
		for _, cand := range candidates {
			if strings.HasPrefix(cand, sel) {
				matchedSet[cand] = true
			}
		}
	}
	out := make([]string, 0, len(matchedSet))
	for _, cand := range candidates {
		if matchedSet[cand] {
			out = append(out, cand)
		}
	}
	return out, nil
}

func selectableOutboundTags(outbounds []CustomOutbound, allowDirectV6 bool) []string {
	list := []string{"direct", "direct-v4"}
	if allowDirectV6 {
		list = append(list, "direct-v6")
	}
	for _, co := range outbounds {
		list = append(list, co.Tag)
	}
	return list
}

func balancerNeedsObservatory(b Balancer) bool {
	return (b.Strategy != nil && b.Strategy.Type == "leastPing") || b.FallbackTag != ""
}

func validateBalancersAndObservatory(
	balancers []Balancer,
	obs *Observatory,
	outbounds []CustomOutbound,
	allowDirectV6 bool,
) error {
	if len(balancers) > maxBalancers {
		return fmt.Errorf("负载均衡器最多 %d 个", maxBalancers)
	}
	customTags := make(map[string]bool, len(outbounds))
	for _, co := range outbounds {
		customTags[co.Tag] = true
	}
	candidates := selectableOutboundTags(outbounds, allowDirectV6)

	seenBalancers := make(map[string]bool, len(balancers))
	requiredObservedTags := make(map[string]bool)

	for i, b := range balancers {
		if err := validateOutboundTagFormat(b.Tag); err != nil {
			return fmt.Errorf("balancers[%d]: %w", i, err)
		}
		if customTags[b.Tag] {
			return fmt.Errorf("balancers[%d]: tag %q 与自定义出站重名", i, b.Tag)
		}
		if seenBalancers[b.Tag] {
			return fmt.Errorf("balancers[%d]: tag %q 重复", i, b.Tag)
		}
		seenBalancers[b.Tag] = true

		if len(b.Selector) == 0 {
			return fmt.Errorf("balancers[%d](%s): selector 不能为空", i, b.Tag)
		}
		matched, err := matchSelectorCandidates(b.Selector, candidates)
		if err != nil {
			return fmt.Errorf("balancers[%d](%s): %w", i, b.Tag, err)
		}
		if len(matched) == 0 {
			return fmt.Errorf("balancers[%d](%s): selector 未匹配到任何有效出站", i, b.Tag)
		}

		stType := "random"
		if b.Strategy != nil && b.Strategy.Type != "" {
			stType = b.Strategy.Type
		}
		if stType != "random" && stType != "roundRobin" && stType != "leastPing" {
			return fmt.Errorf("balancers[%d](%s): strategy.type 仅支持 random、roundRobin 或 leastPing", i, b.Tag)
		}

		if b.FallbackTag != "" {
			if b.FallbackTag == "direct-v6" && !allowDirectV6 {
				return fmt.Errorf("balancers[%d](%s): fallbackTag 引用了未启用的 direct-v6", i, b.Tag)
			}
			isBuiltin := b.FallbackTag == "direct" || b.FallbackTag == "block" || b.FallbackTag == "direct-v4" || b.FallbackTag == "direct-v6"
			if !isBuiltin && !customTags[b.FallbackTag] {
				return fmt.Errorf("balancers[%d](%s): fallbackTag 引用了未知出站 %q", i, b.Tag, b.FallbackTag)
			}
		}

		if balancerNeedsObservatory(b) {
			for _, m := range matched {
				requiredObservedTags[m] = true
			}
		}
	}

	if obs != nil {
		if obs.ProbeURL != "" {
			if err := xssh.ValidateProbeURL(obs.ProbeURL); err != nil {
				return fmt.Errorf("observatory.probeURL: %w", err)
			}
		}
		if obs.ProbeInterval != "" {
			d, err := time.ParseDuration(obs.ProbeInterval)
			if err != nil {
				return fmt.Errorf("observatory.probeInterval 时间格式无效: %q", obs.ProbeInterval)
			}
			if d < 10*time.Second || d > time.Hour {
				return fmt.Errorf("observatory.probeInterval 须介于 10s 与 1h 之间")
			}
		}
		if len(obs.SubjectSelector) == 0 {
			return fmt.Errorf("observatory.subjectSelector 不能为空")
		}
		observedMatched, err := matchSelectorCandidates(obs.SubjectSelector, candidates)
		if err != nil {
			return fmt.Errorf("observatory.subjectSelector: %w", err)
		}
		if len(observedMatched) == 0 {
			return fmt.Errorf("observatory.subjectSelector 未匹配到任何有效出站")
		}
		observedSet := make(map[string]bool, len(observedMatched))
		for _, tag := range observedMatched {
			observedSet[tag] = true
		}
		for reqTag := range requiredObservedTags {
			if !observedSet[reqTag] {
				return fmt.Errorf("observatory.subjectSelector 未覆盖负载均衡依赖探活的出站 %q", reqTag)
			}
		}
	}
	return nil
}

// adjustBalancersAfterRemap 在节点执行出站 remap 后同步调整或剔除失效选择器；
// 若被启用规则引用的负载均衡组候选集变为空，则返回明确错误拦截保存。
func adjustBalancersAfterRemap(p *RoutingPolicy, remaps map[string]string) error {
	if len(p.Balancers) == 0 && p.Observatory == nil {
		return nil
	}
	referencedBalancers := make(map[string]bool)
	for _, r := range p.CustomRules {
		if !r.Disabled && r.BalancerTag != "" {
			referencedBalancers[r.BalancerTag] = true
		}
	}

	candidates := selectableOutboundTags(p.CustomOutbounds, p.SchemaVersion >= 2 || p.EnableIPv6)

	filterSelectors := func(selectors []string) []string {
		kept := make([]string, 0, len(selectors))
		seen := make(map[string]bool)
		for _, sel := range selectors {
			if target, isExactRemap := remaps[sel]; isExactRemap {
				// 若原精确 Tag 被重映射至 block，直接从候选集中剔除；若重映射至其他有效出站则剔除失效原 Tag（或保留仍存在的前缀）
				if target == "block" {
					continue
				}
				continue
			}
			// 仅保留在合并后仍至少能匹配到 1 个候选出站的 selector
			matched, err := matchSelectorCandidates([]string{sel}, candidates)
			if err == nil && len(matched) > 0 && !seen[sel] {
				kept = append(kept, sel)
				seen[sel] = true
			}
		}
		return kept
	}

	keptBalancers := make([]Balancer, 0, len(p.Balancers))
	anyNeedsObs := false
	for _, b := range p.Balancers {
		if target, ok := remaps[b.FallbackTag]; ok {
			b.FallbackTag = target
		}
		b.Selector = filterSelectors(b.Selector)
		if len(b.Selector) == 0 {
			if referencedBalancers[b.Tag] {
				return fmt.Errorf("负载均衡组 %q 的所有候选出站在本节点均已被重映射移除，请调整该节点规则或出站覆盖", b.Tag)
			}
			continue
		}
		if balancerNeedsObservatory(b) {
			anyNeedsObs = true
		}
		keptBalancers = append(keptBalancers, b)
	}
	p.Balancers = keptBalancers

	if p.Observatory != nil {
		p.Observatory.SubjectSelector = filterSelectors(p.Observatory.SubjectSelector)
		if len(p.Observatory.SubjectSelector) == 0 {
			if anyNeedsObs {
				// 交由自动补齐或后续校验处理
				p.Observatory = nil
			} else {
				p.Observatory = nil
			}
		}
	}
	return nil
}

func renderRuntimeBalancersAndObservatory(
	p RoutingPolicy,
	activeBalancerTags map[string]bool,
	defaultProbeURL string,
) ([]Balancer, map[string]any) {
	if len(p.Balancers) == 0 && p.Observatory == nil {
		return nil, nil
	}

	var runtimeBalancers []Balancer
	autoSelectors := make([]string, 0)
	seenAutoSel := make(map[string]bool)
	needsObs := false

	for _, b := range p.Balancers {
		if !activeBalancerTags[b.Tag] {
			continue
		}
		rb := b
		rb.Remark = ""
		if rb.Strategy == nil || rb.Strategy.Type == "" {
			rb.Strategy = &BalancerStrategy{Type: "random"}
		}
		if balancerNeedsObservatory(rb) {
			needsObs = true
			for _, sel := range rb.Selector {
				if !seenAutoSel[sel] {
					seenAutoSel[sel] = true
					autoSelectors = append(autoSelectors, sel)
				}
			}
		}
		runtimeBalancers = append(runtimeBalancers, rb)
	}

	probeTarget := strings.TrimSpace(defaultProbeURL)
	if probeTarget == "" || xssh.ValidateProbeURL(probeTarget) != nil {
		probeTarget = xssh.DefaultProbeURL
	}

	var runtimeObs map[string]any
	if p.Observatory != nil && (needsObs || len(runtimeBalancers) > 0 || len(p.Balancers) == 0) {
		// 仅保留在当前节点最终生成的有效候选出站集合中有匹配的 selector
		runtimeObs = make(map[string]any)
		for k, v := range p.Observatory.Extra {
			runtimeObs[k] = v
		}
		runtimeObs["subjectSelector"] = p.Observatory.SubjectSelector
		pURL := p.Observatory.ProbeURL
		if pURL == "" {
			pURL = probeTarget
		}
		runtimeObs["probeURL"] = pURL
		pInt := p.Observatory.ProbeInterval
		if pInt == "" {
			pInt = "1m"
		}
		runtimeObs["probeInterval"] = pInt
		runtimeObs["enableConcurrency"] = p.Observatory.EnableConcurrency
	} else if needsObs {
		runtimeObs = map[string]any{
			"subjectSelector":   autoSelectors,
			"probeURL":          probeTarget,
			"probeInterval":     "1m",
			"enableConcurrency": true,
		}
	}

	return runtimeBalancers, runtimeObs
}
