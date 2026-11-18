package xray

import (
	"fmt"
	"net/netip"
	"net/url"

	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// Observatory 在 Xray 内直接拨号，没有同步健康探测的节点侧解析检查。
// 使用 IP 字面量避免 DNS 重绑定及自定义 DNS hosts 将探测目标改成内网地址。
const DefaultObservatoryProbeURL = "https://1.1.1.1/"

var nonPublicProbePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func ValidateObservatoryProbeURL(raw string) error {
	if err := xssh.ValidateProbeURL(raw); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	ip, err := netip.ParseAddr(u.Hostname())
	if err != nil || ip.Zone() != "" {
		return fmt.Errorf("健康观测须使用公网 IP 的 HTTPS URL（443 端口），不支持域名")
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return fmt.Errorf("健康观测目标必须是公网 IP")
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return fmt.Errorf("健康观测目标必须是已分配的 IPv6 全球单播地址")
	}
	for _, prefix := range nonPublicProbePrefixes {
		if prefix.Contains(ip) {
			return fmt.Errorf("健康观测目标不能是保留、转换或非公网地址")
		}
	}
	return nil
}
