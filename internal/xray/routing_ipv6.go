package xray

import (
	"bytes"
	"encoding/json"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// IPv6 预检只判断确定需要本机 IPv6 的路径。经代理承载的目标地址属于远端拨号，
// 不能据此要求本节点具备 IPv6；WireGuard 的隧道地址同样不属于本机外网依赖。
func routingPolicyRequiresIPv6(p RoutingPolicy, opts RenderOptions) bool {
	a := ipv6Analyzer{policy: p, outbounds: make(map[string]CustomOutbound), visiting: make(map[string]bool)}
	for _, co := range p.CustomOutbounds {
		a.outbounds[co.Tag] = co
	}
	// direct 是默认兜底及系统运维代理的出站，始终具有运行时可达性。
	if a.outbound("direct", "") {
		return true
	}
	if len(p.IPv6Domains) > 0 && a.outbound("direct-v6", "") {
		return true
	}
	for _, r := range p.CustomRules {
		if !r.Disabled && a.rule(r, "") {
			return true
		}
	}
	active := make(map[string]bool)
	for _, r := range p.CustomRules {
		if !r.Disabled && r.BalancerTag != "" {
			active[r.BalancerTag] = true
		}
	}
	// 直接复用渲染结果，覆盖显式与自动 observatory、系统探针设置及安全默认值。
	_, obs := renderRuntimeBalancersAndObservatory(p, active, opts.DefaultProbeURL)
	if obs != nil {
		probeURL, _ := obs["probeURL"].(string)
		selectors, _ := obs["subjectSelector"].([]string)
		probeHost := ""
		if parsed, err := url.Parse(probeURL); err == nil {
			probeHost = parsed.Hostname()
		}
		tags, _ := matchSelectorCandidates(selectors, selectableOutboundTags(p.CustomOutbounds, true))
		for _, tag := range tags {
			if a.outbound(tag, probeHost) {
				return true
			}
		}
	}
	return a.dns()
}

type ipv6Analyzer struct {
	policy    RoutingPolicy
	outbounds map[string]CustomOutbound
	visiting  map[string]bool
}

func strategyNeedsIPv6(strategy string) bool {
	switch strings.ToLower(strategy) {
	case "forceipv6", "useipv6":
		return true
	}
	return false
}

func localDialNeedsIPv6(address, sendThrough, strategy string) bool {
	if isIPv6Literal(sendThrough) || isIPv6Literal(address) {
		return true
	}
	// IP 字面量不经过域名策略解析。
	return net.ParseIP(address) == nil && strategyNeedsIPv6(strategy)
}

func (a *ipv6Analyzer) outbound(tag, target string) bool {
	key := tag + "\x00" + target
	if a.visiting[key] {
		return false // Validate 已拒绝环路，防止未校验的内部调用递归不终止。
	}
	a.visiting[key] = true
	defer delete(a.visiting, key)
	switch tag {
	case "block":
		return false
	case "direct-v4":
		return localDialNeedsIPv6(target, "", "ForceIPv4")
	case "direct-v6":
		return localDialNeedsIPv6(target, "", "ForceIPv6")
	case "direct":
		strategy := "AsIs"
		if dns := a.policy.DNS; dns != nil && dns.Enabled {
			strategy = dns.QueryStrategy
		}
		return localDialNeedsIPv6(target, "", strategy)
	}
	co, exists := a.outbounds[tag]
	if !exists || co.Protocol == "blackhole" {
		return false
	}
	var settings struct {
		Address        string `json:"address"`
		TargetStrategy string `json:"targetStrategy"`
		DomainStrategy string `json:"domainStrategy"`
		Servers        []struct {
			Address string `json:"address"`
		} `json:"servers"`
		Vnext []struct {
			Address string `json:"address"`
		} `json:"vnext"`
		Peers []struct {
			Endpoint string `json:"endpoint"`
		} `json:"peers"`
	}
	_ = json.Unmarshal(co.Settings, &settings)
	var stream struct {
		Sockopt struct {
			DomainStrategy string `json:"domainStrategy"`
		} `json:"sockopt"`
	}
	_ = json.Unmarshal(co.StreamSettings, &stream)
	strategy := stream.Sockopt.DomainStrategy
	var outerStrategy string
	_ = json.Unmarshal(co.Extra["targetStrategy"], &outerStrategy)
	if co.Protocol == "freedom" {
		freedomStrategy := settings.TargetStrategy
		if freedomStrategy == "" {
			freedomStrategy = settings.DomainStrategy
		}
		if freedomStrategy != "" && !strings.EqualFold(freedomStrategy, "AsIs") {
			strategy = freedomStrategy
		}
	}
	if outerStrategy != "" && !strings.EqualFold(outerStrategy, "AsIs") {
		strategy = outerStrategy
	}
	dp, _ := extractDialerProxy(co.StreamSettings)
	dial := func(address string) bool {
		if dp != "" {
			// 当前出站的 socket 由下一跳提供；sendThrough 不参与本机拨号。
			if net.ParseIP(address) == nil && strategyNeedsIPv6(strategy) {
				address = "2001:db8::1" // 用地址族占位，不进行 DNS 或网络访问。
			}
			return a.outbound(dp, address)
		}
		return localDialNeedsIPv6(address, co.SendThrough, strategy)
	}
	switch co.Protocol {
	case "freedom":
		return dial(target)
	case "socks", "http", "trojan", "shadowsocks":
		if settings.Address != "" && dial(settings.Address) {
			return true
		}
		for _, server := range settings.Servers {
			if dial(server.Address) {
				return true
			}
		}
	case "vless", "vmess":
		if settings.Address != "" && dial(settings.Address) {
			return true
		}
		for _, server := range settings.Vnext {
			if dial(server.Address) {
				return true
			}
		}
	case "wireguard":
		for _, peer := range settings.Peers {
			if host, _, err := net.SplitHostPort(peer.Endpoint); err == nil && dial(host) {
				return true
			}
		}
	}
	return false
}

func (a *ipv6Analyzer) rule(r RoutingRule, target string) bool {
	if r.OutboundTag != "" {
		return a.outbound(r.OutboundTag, target)
	}
	for _, b := range a.policy.Balancers {
		if b.Tag != r.BalancerTag {
			continue
		}
		tags, _ := matchSelectorCandidates(b.Selector, selectableOutboundTags(a.policy.CustomOutbounds, true))
		for _, tag := range tags {
			if a.outbound(tag, target) {
				return true
			}
		}
		return b.FallbackTag != "" && a.outbound(b.FallbackTag, target)
	}
	return false
}

func (a *ipv6Analyzer) dns() bool {
	dns := a.policy.DNS
	if dns == nil || !dns.Enabled {
		return false
	}
	for _, raw := range dns.Servers {
		var server DNSServerObject
		if data := bytes.TrimSpace(raw); len(data) > 0 && data[0] == '"' {
			_ = json.Unmarshal(data, &server.Address)
		} else {
			_ = json.Unmarshal(raw, &server)
		}
		ep, err := parseDNSEndpoint(server.Address, server.Port)
		if err != nil || ep.System || !isIPv6Literal(ep.Host) {
			continue
		}
		if ip := net.ParseIP(ep.Host); ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue // 本地/链路地址不需要默认 IPv6 出口。
		}
		if ep.Local {
			return true
		}
		matched := false
		for _, rule := range a.policy.CustomRules {
			if rule.Disabled || !dnsRuleMatches(rule, ep) {
				continue
			}
			matched = true
			if a.rule(rule, ep.Host) {
				return true
			}
			break
		}
		if !matched && a.outbound("direct", ep.Host) {
			return true
		}
	}
	return false
}

// DNS 上游 IP 字面量仅匹配能静态确定的路由，动态 geodata/扩展条件不假定命中。
func dnsRuleMatches(r RoutingRule, ep dnsEndpoint) bool {
	if len(r.Domain) > 0 || len(r.Protocol) > 0 || len(r.Source) > 0 || len(r.InboundTag) > 0 || len(r.Extra) > 0 {
		return false
	}
	network := "udp"
	if ep.URI {
		network = "tcp"
	}
	if r.Network != "" && r.Network != "tcp,udp" && r.Network != network {
		return false
	}
	if r.Port != "" {
		matched := false
		for _, part := range strings.Split(r.Port, ",") {
			bounds := strings.SplitN(part, "-", 2)
			low, _ := strconv.Atoi(bounds[0])
			high := low
			if len(bounds) == 2 {
				high, _ = strconv.Atoi(bounds[1])
			}
			matched = matched || (ep.Port >= low && ep.Port <= high)
		}
		if !matched {
			return false
		}
	}
	if len(r.IP) == 0 {
		return true
	}
	hostAddr, err := netip.ParseAddr(ep.Host)
	if err != nil {
		return false
	}
	hostAddr = hostAddr.Unmap()
	for _, target := range r.IP {
		if strings.Contains(target, "/") {
			if prefix, cErr := parseXrayCIDR(target); cErr == nil && prefix.Contains(hostAddr) {
				return true
			}
			continue
		}
		if targetAddr, tErr := netip.ParseAddr(target); tErr == nil && targetAddr.Unmap() == hostAddr {
			return true
		}
	}
	return false
}
