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
	var alias dnsServerAlias
	extra, err := decodeRoutingExtensions(data, &alias, dnsServerKnownKeys, "DNS 服务器")
	if err != nil {
		return err
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

type dnsEndpoint struct {
	Address string
	Host    string
	Port    int
	URI     bool
	Local   bool
	System  bool
}

func isLocalOrPrivateAddr(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func rawHostValueHasLocalIP(rawVal json.RawMessage) bool {
	trimmed := bytes.TrimSpace(rawVal)
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] == '"' {
		var single string
		return json.Unmarshal(trimmed, &single) == nil && isLocalOrPrivateAddr(net.ParseIP(single))
	}
	if trimmed[0] == '[' {
		var arr []string
		if json.Unmarshal(trimmed, &arr) == nil {
			for _, s := range arr {
				if isLocalOrPrivateAddr(net.ParseIP(s)) {
					return true
				}
			}
		}
	}
	return false
}

// dnsHostsStaticLocalMatch 判断 Xray 内置 dns.hosts 是否将目标域名静态映射至本机/私网地址。
// 返回 matched=true 表示已静态确认命中本地地址；indeterminate=true 表示含指向本地地址的 geosite 规则但当前未加载对应分类数据（不混同为已匹配）。
func dnsHostsStaticLocalMatch(host string, hosts map[string]json.RawMessage, geosite *GeositeDB) (matched bool, indeterminate bool) {
	if len(hosts) == 0 {
		return false, false
	}
	keys := make([]string, 0, len(hosts))
	for k := range hosts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !rawHostValueHasLocalIP(hosts[k]) {
			continue
		}
		lowerKey := strings.ToLower(k)
		switch {
		case lowerKey == host || lowerKey == "full:"+host:
			return true, false
		case strings.HasPrefix(lowerKey, "domain:"):
			suffix := strings.TrimPrefix(lowerKey, "domain:")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true, false
			}
		case strings.HasPrefix(lowerKey, "geosite:"):
			spec := k[len("geosite:"):]
			hit, known := geosite.Match(spec, host)
			if !known {
				indeterminate = true
				continue
			}
			if hit {
				return true, false
			}
		}
	}
	return false, indeterminate
}

// mayReachLocal 在面板静态可见范围内判断上游 DNS 是否指向本机/私网地址。
// 普通域名返回 false 仅代表静态配置未将其指向本机，不代表排除运行时外部 DNS 解析变化；管理端口 10085 始终由调用方无条件独立拦截。
func (e dnsEndpoint) mayReachLocal(hosts map[string]json.RawMessage, geosite *GeositeDB) bool {
	if e.System {
		return false
	}
	if ip := net.ParseIP(e.Host); ip != nil {
		return isLocalOrPrivateAddr(ip)
	}
	lower := strings.ToLower(e.Host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".local") {
		return true
	}
	// Xray 的 +local 上游（https+local / tcp+local / quic+local）通过系统拨号器 internet.DialSystem 解析，不使用内置 dns.hosts。
	if e.Local {
		return false
	}
	matched, _ := dnsHostsStaticLocalMatch(lower, hosts, geosite)
	return matched
}

// parseDNSEndpoint 统一校验、保护端口判断和最终渲染使用的有效地址。
// Xray 的 TCP/QUIC DNS 读取 URI 端口，忽略外层 port；TCP 对象 port 必须写入 URI。
func parseDNSEndpoint(address string, explicitPort int) (dnsEndpoint, error) {
	e := dnsEndpoint{Address: address}
	if address == "" || strings.TrimSpace(address) != address || strings.ContainsAny(address, " \n\r\t") {
		return e, fmt.Errorf("DNS 服务器 address 不能为空或含空白字符")
	}
	if explicitPort != 0 {
		if err := validatePortRange(explicitPort); err != nil {
			return e, err
		}
	}
	if address == "localhost" {
		if explicitPort != 0 {
			return e, fmt.Errorf("localhost 模式不支持单独指定 port")
		}
		e.System = true
		return e, nil
	}
	for _, prefix := range allowedDNSURIPrefixes {
		if strings.HasPrefix(address, prefix) {
			e.URI = true
			e.Local = strings.Contains(prefix, "+local://")
			u, parseErr := url.Parse(address)
			if parseErr != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
				return e, fmt.Errorf("无效 DNS URI: %q", address)
			}
			e.Host = strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
			if net.ParseIP(e.Host) == nil {
				if err := validateBareHostname(e.Host); err != nil {
					return e, fmt.Errorf("无效 DNS URI 主机: %w", err)
				}
			}
			isHTTPS := u.Scheme == "https" || u.Scheme == "https+local"
			isTCP := u.Scheme == "tcp" || u.Scheme == "tcp+local"
			if explicitPort != 0 && !isTCP {
				return e, fmt.Errorf("DNS URI (%s) 请直接在 URI 中指定端口，port 字段仅支持 IP/TCP 模式", prefix)
			}
			e.Port = explicitPort
			if u.Port() != "" {
				pNum, pErr := strconv.Atoi(u.Port())
				if pErr != nil || pNum < 1 || pNum > 65535 {
					return e, fmt.Errorf("DNS URI 端口无效: %q", address)
				}
				if explicitPort != 0 && explicitPort != pNum {
					return e, fmt.Errorf("DNS URI 端口与 port 字段不一致")
				}
				e.Port = pNum
			} else if explicitPort != 0 {
				u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(explicitPort))
			}
			if e.Port == 0 {
				if isHTTPS {
					e.Port = 443
				} else if u.Scheme == "quic+local" {
					e.Port = 853
				} else {
					e.Port = 53
				}
			}
			e.Address = u.String()
			return e, nil
		}
	}
	if strings.Contains(address, "://") {
		return e, fmt.Errorf("不支持的 DNS URI 协议前缀: %q", address)
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return e, fmt.Errorf("DNS address 须为合法 IP、localhost 或受支持的加密/TCP DNS URI: %q", address)
	}
	e.Host, e.Port = ip.String(), explicitPort
	if e.Port == 0 {
		e.Port = 53
	}
	return e, nil
}

// parseDNSAddressAndLoopbackPort 保持校验调用的统一入口。
func parseDNSAddressAndLoopbackPort(address string, explicitPort int) (int, bool, bool, error) {
	e, err := parseDNSEndpoint(address, explicitPort)
	if err != nil {
		return 0, e.URI, e.System, err
	}
	if e.mayReachLocal(nil, nil) {
		return e.Port, e.URI, e.System, nil
	}
	return 0, e.URI, e.System, nil
}

func validateExpectIP(v string) error {
	if v == "" || len(v) > 256 || strings.TrimSpace(v) != v {
		return fmt.Errorf("expectIPs 条目无效: %q", v)
	}
	if strings.HasPrefix(v, "geoip:") || strings.HasPrefix(v, "!geoip:") {
		tag := strings.TrimPrefix(strings.TrimPrefix(normalizeGeoIPRule(v), "geoip:"), "!")
		if !geoipTagRegex.MatchString(tag) {
			return fmt.Errorf("无效 geoip 标签: %q", v)
		}
		return nil
	}
	if err := parseXrayIPOrCIDR(v); err != nil {
		return fmt.Errorf("expectIPs 必须是合法 IP、CIDR 或 geoip: 标签: %w", err)
	}
	return nil
}

func validateDNSConfig(dns *DNSConfig) error {
	if dns == nil {
		return nil
	}
	if err := validateRoutingExtensions(dns.Extra, "dns"); err != nil {
		return err
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
			ep, addrErr := parseDNSEndpoint(s, 0)
			if addrErr != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, addrErr)
			}
			if !ep.System && ep.Port == 10085 {
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
			ep, addrErr := parseDNSEndpoint(srvObj.Address, srvObj.Port)
			if addrErr != nil {
				return fmt.Errorf("dns.servers[%d]: %w", i, addrErr)
			}
			if !ep.System && ep.Port == 10085 {
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

// checkDNSInboundPortConflict 执行不依赖任何未经节点核验面板本地 geodata 的确定性自环检查。
func checkDNSInboundPortConflict(dns *DNSConfig, inbounds []Inbound) error {
	return checkDNSInboundPortConflictWithGeosite(dns, inbounds, nil)
}

// DiagnoseDNSHostsSelfLoop 针对面板未持有目标节点已核验 geosite.dat 时的不确定边界生成非阻断诊断提示。
// localDiagnosticGeosite 仅作为面板本机参考诊断，绝不代替远端节点事实触发硬拦截。
func DiagnoseDNSHostsSelfLoop(dns *DNSConfig, inbounds []Inbound, verifiedNodeGeosite, localDiagnosticGeosite *GeositeDB) []string {
	if dns == nil || !dns.Enabled || verifiedNodeGeosite != nil {
		return nil
	}
	inboundPorts := make(map[int]string, len(inbounds))
	for _, in := range inbounds {
		if in.Port > 0 {
			inboundPorts[in.Port] = in.Tag
		}
	}
	var warnings []string
	for i, srvRaw := range dns.Servers {
		trimmed := bytes.TrimSpace(srvRaw)
		var (
			ep  dnsEndpoint
			err error
			ok  bool
		)
		if len(trimmed) > 0 && trimmed[0] == '"' {
			var s string
			if json.Unmarshal(trimmed, &s) == nil {
				ep, err = parseDNSEndpoint(s, 0)
				ok = err == nil
			}
		} else if len(trimmed) > 0 && trimmed[0] == '{' {
			var obj DNSServerObject
			if json.Unmarshal(trimmed, &obj) == nil {
				ep, err = parseDNSEndpoint(obj.Address, obj.Port)
				ok = err == nil
			}
		}
		if !ok || ep.System || ep.Local || ep.Port == 0 || net.ParseIP(ep.Host) != nil {
			continue
		}
		ownerTag, portConflict := inboundPorts[ep.Port]
		if !portConflict {
			continue
		}
		matched, indeterminate := dnsHostsStaticLocalMatch(strings.ToLower(ep.Host), dns.Hosts, nil)
		if matched || !indeterminate {
			continue
		}
		msg := fmt.Sprintf("dns.servers[%d] (%s:%d): dns.hosts 含指向本地/私网地址的 geosite 规则，面板未持有目标节点已核验的 geosite.dat，未对入站 %q 端口做分类级自环硬拦截；请以目标节点实际 geodata 为准", i, ep.Host, ep.Port, ownerTag)
		if localDiagnosticGeosite != nil {
			if localHit, _ := dnsHostsStaticLocalMatch(strings.ToLower(ep.Host), dns.Hosts, localDiagnosticGeosite); localHit {
				msg += "（参考提示：面板本机未核验的参考 geosite.dat 中该域名命中了对应分类）"
			}
		}
		warnings = append(warnings, msg)
	}
	return warnings
}

// DiagnoseRenderedDNSSelfLoop 从已合成的基础配置提取入站端口并返回未核验 geosite 自环诊断提示。
func DiagnoseRenderedDNSSelfLoop(baseConfig string, p RoutingPolicy, opts RenderOptions) []string {
	if p.DNS == nil || !p.DNS.Enabled || opts.VerifiedNodeGeosite() != nil {
		return nil
	}
	var cfg Config
	if json.Unmarshal([]byte(baseConfig), &cfg) != nil {
		return nil
	}
	return DiagnoseDNSHostsSelfLoop(p.DNS, cfg.Inbounds, nil, ResolveDiagnosticLocalGeositeDB(""))
}

func checkDNSInboundPortConflictWithGeosite(dns *DNSConfig, inbounds []Inbound, geosite *GeositeDB) error {
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
		var (
			ep  dnsEndpoint
			err error
			ok  bool
		)
		if len(trimmed) > 0 && trimmed[0] == '"' {
			var s string
			if json.Unmarshal(trimmed, &s) == nil {
				ep, err = parseDNSEndpoint(s, 0)
				ok = err == nil
			}
		} else if len(trimmed) > 0 && trimmed[0] == '{' {
			var obj DNSServerObject
			if json.Unmarshal(trimmed, &obj) == nil {
				ep, err = parseDNSEndpoint(obj.Address, obj.Port)
				ok = err == nil
			}
		}
		if !ok || ep.System {
			continue
		}
		if ep.Port == 10085 {
			return fmt.Errorf("dns.servers[%d]: 禁止指向面板本地管理端口 10085", i)
		}
		if ep.Port > 0 && ep.mayReachLocal(dns.Hosts, geosite) {
			if ownerTag, conflict := inboundPorts[ep.Port]; conflict {
				return fmt.Errorf("dns.servers[%d]: 本机/内网解析地址端口 %d 与入站 %q 冲突，已拦截自环解析", i, ep.Port, ownerTag)
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
		if _, known := dnsConfigKnownKeys[strings.ToLower(k)]; !known {
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
				if endpoint, err := parseDNSEndpoint(obj.Address, obj.Port); err == nil && endpoint.URI {
					obj.Address = endpoint.Address
					obj.Port = 0
				}
				obj.ExpectIPs = normalizeIPRules(obj.ExpectIPs)
				// expectedIPs 是 Xray 兼容别名，也必须使用支持旧版内核的语法。
				for _, key := range []string{"expectedIPs", "unexpectedIPs"} {
					if extra := obj.Extra[key]; len(extra) > 0 {
						var rules []string
						if json.Unmarshal(extra, &rules) == nil {
							obj.Extra[key], _ = json.Marshal(normalizeIPRules(rules))
						}
					}
				}
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
		if sel == "" || strings.TrimSpace(sel) != sel {
			return nil, fmt.Errorf("选择器条目不能为空或含首尾空白")
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
		if err := validateRoutingExtensions(b.Extra, "balancer"); err != nil {
			return err
		}
		if b.Strategy != nil {
			if err := validateRoutingExtensions(b.Strategy.Extra, "balancer.strategy"); err != nil {
				return err
			}
		}
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
		if err := validateRoutingExtensions(obs.Extra, "observatory"); err != nil {
			return err
		}
		if obs.ProbeURL != "" {
			if err := ValidateObservatoryProbeURL(obs.ProbeURL); err != nil {
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
			// Xray selector 始终按前缀匹配；即使等于已移除的 tag，仍可能选中其他出站。
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
		keptBalancers = append(keptBalancers, b)
	}
	p.Balancers = keptBalancers

	if p.Observatory != nil {
		p.Observatory.SubjectSelector = filterSelectors(p.Observatory.SubjectSelector)
		if len(p.Observatory.SubjectSelector) == 0 {
			// 没有剩余观测对象时由有效负载均衡器按需补齐。
			p.Observatory = nil
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
		if rb.Strategy == nil {
			rb.Strategy = &BalancerStrategy{Type: "random"}
		} else if rb.Strategy.Type == "" {
			strategy := *rb.Strategy
			strategy.Type = "random"
			rb.Strategy = &strategy
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
	if probeTarget == "" || ValidateObservatoryProbeURL(probeTarget) != nil {
		probeTarget = DefaultObservatoryProbeURL
	}

	var runtimeObs map[string]any
	if p.Observatory != nil {
		// 显式观测器自身就是出站引用根，包含仅用于独立探测的出站。
		runtimeObs = make(map[string]any)
		for k, v := range p.Observatory.Extra {
			if _, known := observatoryKnownKeys[strings.ToLower(k)]; !known {
				runtimeObs[k] = v
			}
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
