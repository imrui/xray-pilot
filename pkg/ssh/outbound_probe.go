package ssh

import (
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

//go:embed vless_outbound_probe.py
var vlessOutboundProbePython string

// 探针沿用服务的用户、组和网络命名空间，避免 SSH 管理员的权限影响结果。
const outboundProbeIdentityPython = `import os,sys
def use_xray_identity(proc):
    if not os.path.samefile("/proc/self/ns/net", proc + "/ns/net"):
        raise ValueError("different network namespace")
    with open(proc + "/status") as f:
        ids = {key: value.split() for key, _, value in (line.partition(":") for line in f)
               if key in ("Uid", "Gid", "Groups")}
    uid, gid = int(ids["Uid"][1]), int(ids["Gid"][1])
    groups = set(map(int, ids["Groups"]))
    if set(os.getgroups()) - {gid} != groups - {gid}:
        os.setgroups(sorted(groups))
    if os.getegid() != gid:
        os.setgid(gid)
    if os.geteuid() != uid:
        os.setuid(uid)
    os.environ.pop("SSLKEYLOGFILE", None)
    os.umask(0o077)
`

// TestVLESSOutbound 在源节点用磁盘上的指定出站进行单次 HTTPS 测试。仅返回固定诊断，
// 运行配置、凭据和内核错误输出均留在节点内。耗时包括该线路的握手和 HTTPS 请求。
func (c *Client) TestVLESSOutbound(configPath, tag, probeURL string) (time.Duration, error) {
	if tag == "" || len(tag) > 128 || strings.ContainsAny(tag, "\x00\r\n") {
		return 0, errors.New("出站标签无效")
	}
	if probeURL == "" {
		probeURL = DefaultProbeURL
	}
	if err := ValidateProbeURL(probeURL); err != nil {
		return 0, err
	}
	script := `set -eu
path=$1
command -v python3 >/dev/null && command -v curl >/dev/null || { echo TOOLS; exit 0; }
systemctl is-active --quiet xray || { echo INACTIVE; exit 0; }
if ! ( ` + xrayPathCheckScript + ` ) >/dev/null 2>&1; then echo PATH; exit 0; fi
pid=$(systemctl show xray -p MainPID --value)
binary=$(readlink -f "/proc/$pid/exe") && test -x "$binary" && cd "/proc/$pid/cwd" || { echo INACTIVE; exit 0; }
exec python3 -I - "$path" "$2" "$3" "$binary" "$pid" 2>/dev/null <<'PILOT_TEST_OUTBOUND'
` + outboundProbeIdentityPython + `
try:
    use_xray_identity("/proc/" + str(int(sys.argv.pop())))
except (OSError, KeyError, IndexError, ValueError):
    print("IDENTITY")
    sys.exit(0)
` + publicProbeTargetPython + vlessOutboundProbePython + "\nPILOT_TEST_OUTBOUND"
	cmd := "timeout -k 3 30 bash -c " + shellQuote(script) + " -- " + shellQuote(configPath) + " " + shellQuote(tag) + " " + shellQuote(probeURL)
	output, err := c.RunBounded(cmd, 35*time.Second)
	if err != nil {
		return 0, errors.New("节点探测未完成，请检查 SSH 连接及超时")
	}
	status := strings.TrimSpace(output)
	if ms, ok := strings.CutPrefix(status, "OK "); ok {
		if elapsed, err := strconv.Atoi(ms); err == nil && elapsed >= 0 && elapsed <= 30000 {
			return time.Duration(elapsed) * time.Millisecond, nil
		}
	}
	if code, ok := strings.CutPrefix(status, "HTTP "); ok && len(code) == 3 {
		if n, err := strconv.Atoi(code); err == nil && n >= 100 && n <= 599 && n/100 != 2 {
			return 0, fmt.Errorf("探测网址返回 HTTP %d，未通过健康检查", n)
		}
	}
	switch status {
	case "INACTIVE":
		return 0, errors.New("节点的 Xray 服务未运行或无法访问")
	case "IDENTITY":
		return 0, errors.New("无法使用 Xray 服务的用户或网络命名空间进行测试")
	case "PATH", "CONFIG":
		return 0, errors.New("无法核实节点上已部署的 Xray 配置")
	case "MISSING":
		return 0, errors.New("节点已部署配置中未找到唯一匹配的 VLESS 出站")
	case "DEPENDENCY":
		return 0, errors.New("此出站依赖额外的代理或 DNS 路由，暂不支持独立测试")
	case "TARGET":
		return 0, errors.New("节点无法将探测网址解析为公网地址")
	case "START":
		return 0, errors.New("节点上的临时 Xray 探针未能启动")
	case "HTTPS":
		return 0, errors.New("经此出站的 HTTPS 请求失败，请检查出站参数、落地节点和探测网址")
	case "TOOLS":
		return 0, errors.New("节点缺少 python3 或 curl")
	default:
		return 0, errors.New("未收到有效的节点探测结果")
	}
}
