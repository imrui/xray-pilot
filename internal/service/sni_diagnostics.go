package service

import (
	"fmt"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"net"
	"regexp"
	"strings"
	"time"
)

var sniName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func (s *SyncService) DiagnoseSNI(id uint, host string) (string, error) {
	if !sniName.MatchString(host) || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "", fmt.Errorf("请输入合法的公开域名")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", fmt.Errorf("域名标签无效")
		}
	}
	n, err := s.nodeRepo.FindByID(id)
	if err != nil {
		return "", err
	}
	p := s.sshParams(n)
	c, err := xssh.Connect(xssh.Config{Host: p.Host, Port: p.Port, User: p.User, KeyPath: p.KeyPath, KnownHostsPath: p.KnownHostsPath})
	if err != nil {
		return "", fmt.Errorf("无法连接节点")
	}
	defer c.Close()
	// Resolve on the target, validate every address, then pin the connection to it.
	// No redirect following, no arbitrary shell interpolation, no private targets.
	script := `import socket, ssl, ipaddress, json, time
host = "__HOST__"
results = []
addresses = sorted(set(x[4][0] for x in socket.getaddrinfo(host,443,type=socket.SOCK_STREAM)))
for addr in addresses:
    ip = ipaddress.ip_address(addr)
    if not ip.is_global:
        raise SystemExit("目标解析包含非公网地址，拒绝探测")
for addr in addresses[:8]:
    start = time.monotonic()
    result = {"address":addr,"family":"IPv6" if ":" in addr else "IPv4"}
    try:
        ctx = ssl.create_default_context()
        ctx.set_alpn_protocols(["h2","http/1.1"])
        with socket.create_connection((addr,443),timeout=3) as raw:
            result["tcp_reachable"] = True
            with ctx.wrap_socket(raw,server_hostname=host) as tls:
                result["tls"] = tls.version()
                result["alpn"] = tls.selected_alpn_protocol()
                result["certificate_name_valid"] = True
        ctx.set_alpn_protocols(["http/1.1"])
        with socket.create_connection((addr,443),timeout=3) as raw:
            with ctx.wrap_socket(raw,server_hostname=host) as tls:
                tls.sendall(("HEAD / HTTP/1.1\r\nHost: "+host+"\r\nConnection: close\r\n\r\n").encode("ascii"))
                headers = tls.recv(2048).decode("latin1").split("\r\n")
                result["http"] = headers[0][:160]
                result["http_probe_alpn"] = "http/1.1"
                result["redirect"] = next((h[9:].strip()[:256] for h in headers if h.lower().startswith("location:")),None)
    except Exception as e:
        result["error"] = type(e).__name__
    result["elapsed_ms"] = round((time.monotonic()-start)*1000)
    results.append(result)
print(json.dumps({"host":host,"results":results,"note":"HTTP 状态仅供诊断；需另行验证 Reality 握手"},ensure_ascii=False))
`
	script = strings.ReplaceAll(script, "__HOST__", host)
	output, err := c.RunBounded("timeout 35 python3 - <<'PILOT_SNI'\n"+script+"\nPILOT_SNI", 40*time.Second)
	if err != nil {
		return "", fmt.Errorf("SNI 诊断失败：节点需安装 python3，且域名必须全部解析为公网地址")
	}
	return output, nil
}
