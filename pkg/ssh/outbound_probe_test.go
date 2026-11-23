package ssh

import (
	"context"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProbeUsesXrayServiceIdentity(t *testing.T) {
	const checks = `
import io
from unittest import mock
status = "Uid:\t99\t900\t900\t900\nGid:\t99\t901\t901\t901\nGroups:\t901 904\n"
with mock.patch.object(os.path, "samefile", return_value=False):
    try:
        use_xray_identity("/proc/123")
    except ValueError:
        pass
    else:
        raise AssertionError("probe used a different network namespace")
ops = []
with mock.patch("builtins.open", return_value=io.StringIO(status)), mock.patch.object(os.path, "samefile", return_value=True):
    for field, value in (("geteuid", 0), ("getegid", 0), ("getgroups", [0])):
        setattr(os, field, lambda value=value: value)
    for field in ("setgroups", "setgid", "setuid"):
        setattr(os, field, lambda arg, field=field: ops.append((field, arg)))
    use_xray_identity("/proc/123")
assert ops == [("setgroups",[901,904]), ("setgid",901), ("setuid",900)]
`
	cmd := exec.Command(outboundProbePython(t), "-c", outboundProbeIdentityPython+checks)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("service identity: %v %s", err, out)
	}
}

func outboundProbePython(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"python", "python3"} {
		if binary, err := exec.LookPath(name); err == nil && exec.Command(binary, "--version").Run() == nil {
			return binary
		}
	}
	t.Skip("Python unavailable")
	return ""
}

func TestProbeDeployedOutboundSelection(t *testing.T) {
	const tests = `
import tempfile
def select(config):
    with tempfile.NamedTemporaryFile(mode="w", delete=False) as f:
        json.dump(config,f)
    try:
        return select_outbound(f.name, "wanted")
    finally:
        os.unlink(f.name)
def rejected(config, reason):
    try:
        select(config)
    except ProbeError as e:
        assert str(e) == reason
    else:
        raise AssertionError("unexpectedly accepted")
one={"protocol":"vless","tag":"wanted","streamSettings":{"sockopt":{"mark":7}}}
other={"protocol":"freedom","tag":"direct"}
assert select({"outbounds":[other,one]}) == one
headers={"TLSSettings":{"MaStErKeYlOg":"/keep-header"}}
transport={"tLsSettings":{"MaStErKeYlOg":"/keys","serverName":"test"},"xhttpSettings":{"headers":headers,
           "extra":{"downloadSettings":{"RealitySettings":{"MasterKeyLog":"/more-keys","fingerprint":"chrome"}}}}}
stream=select({"outbounds":[dict(one,streamSettings=transport)]})["streamSettings"]
assert stream["tLsSettings"] == {"serverName":"test"} and stream["xhttpSettings"]["headers"] == headers
assert stream["xhttpSettings"]["extra"]["downloadSettings"]["RealitySettings"] == {"fingerprint":"chrome"}
rejected({"outbounds":[other]}, "MISSING")
rejected({"outbounds":[one,one]}, "MISSING")
rejected({"outbounds":[dict(other,tag="wanted")]}, "MISSING")
rejected({"outbounds":[dict(one,proxySettings={"tag":"other"})]}, "DEPENDENCY")
rejected({"outbounds":[dict(one,streamSettings={"sockopt":{"dialerProxy":"direct"}})]}, "DEPENDENCY")
rejected({"dns":{"hosts":{"relay.test":"1.1.1.1"}},"outbounds":[dict(one,targetStrategy="UseIP")]}, "DEPENDENCY")
ip="2606:4700:4700::1111"
socket.getaddrinfo=lambda *args,**kwargs: [(socket.AF_INET6,socket.SOCK_STREAM,6,"",(ip,443,0,0))]
assert public_probe_target("https://["+ip+"]/test") == ("["+ip+"]","["+ip+"]")
for ip in ("127.0.0.1","224.0.0.1","ff02::1"):
    try:
        public_probe_target("https://example.com")
    except ValueError:
        continue
    raise AssertionError("non-public probe target accepted")
`
	cmd := exec.Command(outboundProbePython(t), "-c", "import sys\nexec(sys.stdin.read(), {'__name__':'fixture'})")
	cmd.Stdin = strings.NewReader(publicProbeTargetPython + vlessOutboundProbePython + tests)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestActualVLESSOutboundProbe(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to run the isolated VLESS probe")
	}
	python := outboundProbePython(t)
	var requests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/denied" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	dir := t.TempDir()
	cert := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	selector, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := selector.Addr().(*net.TCPAddr).Port
	selector.Close()
	const id = "11111111-1111-4111-8111-111111111111"
	// 两个真实内核进程分别作为 VLESS 客户端和落地机。
	server := exec.Command(binary, "run", "-format", "json", "-config", "stdin:")
	server.Stdin = strings.NewReader(fmt.Sprintf(`{"inbounds":[{"listen":"127.0.0.1","port":%d,"protocol":"vless","settings":{"decryption":"none","clients":[{"id":%q}]}}],"outbounds":[{"protocol":"freedom","settings":{"finalRules":[{"action":"allow","ip":["127.0.0.1"],"port":%d},{"action":"block"}]}}]}`, port, id, target.Listener.Addr().(*net.TCPAddr).Port))
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Process.Kill(); _ = server.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", selector.Addr().String(), 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("VLESS fixture did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// 验证公网 IP 已固定，然后把目标替换为本地 HTTPS 测试服务。全程仅使用回环网络。
	const fixture = `import atexit,os,subprocess
original_popen=subprocess.Popen
children=[]
def track_process(*args,**kwargs):
    child=original_popen(*args,**kwargs)
    children.append(child)
    return child
subprocess.Popen=track_process
@atexit.register
def check_cleanup():
    for child in children:
        if child.poll() is None:
            child.kill()
            child.wait()
            os._exit(90)
original_lookup=socket.getaddrinfo
def resolve(host,*args,**kwargs):
    if host == "example.com": return [(socket.AF_INET,socket.SOCK_STREAM,6,"",("1.1.1.1",443))]
    return original_lookup(host,*args,**kwargs)
socket.getaddrinfo=resolve
if os.environ["PROBE_TEST_BUSY"] == "true":
    # 保留选中的端口，让 Xray 绑定失败。其他进程的监听不能作为启动回执。
    class BusySocket(socket.socket):
        def __exit__(self, *args):
            self.listen()
    socket.socket=BusySocket
original_run=subprocess.run
def trusted_curl(args,**kwargs):
    assert args[:2] == ["curl","-q"] and args[args.index("--noproxy")+1] == ""
    route=args.index("--connect-to")+1
    assert args[route] == "example.com:443:1.1.1.1:443"
    args[route]="example.com:443:"+os.environ["PROBE_TEST_TARGET"]
    return original_run(args[:2]+["--cacert",os.environ["PROBE_TEST_CA"]]+args[2:],**kwargs)
subprocess.run=trusted_curl
`
	for _, tc := range []struct{ name, uuid, path, want string }{
		{"through-selected-relay", id, "/test", "OK "},
		{"http-error", id, "/denied", "HTTP 403"},
		{"wrong-account", "22222222-2222-4222-8222-222222222222", "/test", "HTTPS"},
		{"occupied-probe-port", id, "/test", "START"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantRequests := requests.Load()
			if tc.want == "HTTP 403" || tc.want == "OK " {
				wantRequests++
			}
			path := filepath.Join(dir, "deployed.json")
			raw := fmt.Sprintf(`{"outbounds":[{"tag":"default","protocol":"freedom"},{"tag":"los2","protocol":"vless","settings":{"address":"127.0.0.1","port":%d,"id":%q,"encryption":"none"}}]}`, port, tc.uuid)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, python, "-c", publicProbeTargetPython+fixture+vlessOutboundProbePython, path, "los2", "https://example.com"+tc.path, binary)
			cmd.Env = append(os.Environ(), "PROBE_TEST_CA="+cert, "PROBE_TEST_TARGET="+target.Listener.Addr().String(),
				fmt.Sprintf("PROBE_TEST_BUSY=%t", tc.name == "occupied-probe-port"), "NO_PROXY=*")
			output, err := cmd.CombinedOutput()
			if err != nil || !strings.HasPrefix(string(output), tc.want) {
				t.Fatalf("probe result %q %v", output, err)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != raw || requests.Load() != wantRequests {
				t.Fatalf("disk changed or unexpected HTTPS delivery count %d: %v", requests.Load(), err)
			}
		})
	}
}
