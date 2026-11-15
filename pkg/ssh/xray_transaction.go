package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"strings"
	"sync"
	"time"
)

// ApplyXrayConfig validates before replacing, serializes remote writers and refuses
// rollback across an authorization change. validateOnly never mutates live config.
type TransactionOptions struct {
	BaselineHash string
	AllowInitial bool
	BeforeApply  func() error
	ProbeURL     string
	OnPhase      func(string)
}

func (c *Client) ApplyXrayConfig(content, path string, restart, validateOnly bool, opts ...TransactionOptions) error {
	old, err := c.ReadRemoteFile(path)
	if err != nil {
		return fmt.Errorf("无法读取当前配置，拒绝覆盖")
	}
	expected := sha256.Sum256([]byte(old))
	var opt TransactionOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if !validateOnly {
		baseline := opt.BaselineHash
		if baseline == "" && !opt.AllowInitial {
			baseline = crypto.HashConfig(content)
		}
		if baseline != "" && crypto.HashConfig(old) != baseline && crypto.HashConfig(old) != crypto.HashConfig(content) {
			return fmt.Errorf("DRIFT_CONFLICT：当前文件与纳管基线不一致，请先核对")
		}
		if opt.BeforeApply != nil {
			if err := opt.BeforeApply(); err != nil {
				return err
			}
		}
	}
	envelope, err := probeEnvelope(content, opt.ProbeURL)
	if err != nil {
		return err
	}
	script := xrayTransactionScript(path, hex.EncodeToString(expected[:]), restart, validateOnly)
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	var output limitedOutput
	checkpointErr := make(chan error, 1)
	output.onLine = func(line string) {
		if line == "PILOT:READY" {
			if opt.BeforeApply != nil {
				if err := opt.BeforeApply(); err != nil {
					checkpointErr <- err
					_, _ = stdin.Write([]byte("ABORT\n"))
					_ = stdin.Close()
					return
				}
			}
			_, _ = stdin.Write([]byte("COMMIT\n"))
			_ = stdin.Close()
		} else if strings.HasPrefix(line, "PILOT:PHASE:") && opt.OnPhase != nil {
			opt.OnPhase(strings.TrimPrefix(line, "PILOT:PHASE:"))
		}
	}
	sess.Stdout = &output
	sess.Stderr = &output
	timer := time.AfterFunc(190*time.Second, func() { _ = sess.Close() })
	defer timer.Stop()
	if err = sess.Start("timeout 180 bash -c " + shellQuote(script)); err != nil {
		return err
	}
	if _, err = stdin.Write([]byte(envelope + "\n")); err != nil {
		return fmt.Errorf("节点配置上传中断")
	}
	if validateOnly {
		_ = stdin.Close()
	}
	err = sess.Wait()
	select {
	case checkErr := <-checkpointErr:
		return checkErr
	default:
	}
	if err != nil {
		// Only stable phase markers are surfaced: Xray errors may include credentials.
		for _, phase := range []string{"ROLLBACK_FAILED", "ROLLED_BACK", "AUTHORIZATION_STOPPED", "PREFLIGHT_FAILED", "DRIFT_CONFLICT", "PATH_MISMATCH", "VERSION_UNSUPPORTED", "IPV6_UNAVAILABLE", "HEALTH_PROBE_UNAVAILABLE", "HEALTH_FAILED"} {
			if strings.Contains(output.text, phase) {
				return fmt.Errorf("节点配置任务失败：%s", phase)
			}
		}
		return fmt.Errorf("节点配置任务未完成，请核对远端状态后重试")
	}
	return nil
}

type limitedOutput struct {
	text    string
	mu      sync.Mutex
	partial string
	onLine  func(string)
}

func (b *limitedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.onLine != nil {
		b.partial += string(p)
		for {
			idx := strings.IndexByte(b.partial, '\n')
			if idx < 0 {
				break
			}
			line := b.partial[:idx]
			b.partial = b.partial[idx+1:]
			b.onLine(strings.TrimSpace(line))
		}
		if len(b.partial) > 8192 {
			b.partial = ""
		}
	}
	if len(b.text) < 8192 {
		n := 8192 - len(b.text)
		if n > len(p) {
			n = len(p)
		}
		b.text += string(p[:n])
	}
	return len(p), nil
}
func xrayTransactionScript(path, expected string, restart, validateOnly bool) string {
	s := `set -eu
umask 077
path=__PATH__
dir=$(dirname "$path")
test -d "$dir"
test ! -L "$path"
test ! -L "$dir/.pilot-config.lock"
test ! -L "$dir/.pilot-config-state"
exec 9>"$dir/.pilot-config.lock"
flock -w 30 9
actual=$(sha256sum "$path" | cut -d ' ' -f1)
test "$actual" = __HASH__ || { echo DRIFT_CONFLICT; exit 20; }
execstart=$(systemctl show xray -p ExecStart --value)
if ! python3 - "$path" "$execstart" <<'PILOT_PATH'
import shlex,sys
parts=shlex.split(sys.argv[2].replace(";"," "))
paths=[parts[i+1] for i,x in enumerate(parts[:-1]) if x in ("-c","-config")]+[x.split("=",1)[1] for x in parts if x.startswith(("-c=","-config="))]
sys.exit(0 if paths==[sys.argv[1]] and not any(x.startswith("-confdir") for x in parts) else 1)
PILOT_PATH
then echo PATH_MISMATCH; exit 21; fi
pid=$(systemctl show xray -p MainPID --value)
if test "$pid" != 0 && test -e "/proc/$pid/exe"; then binary=$(readlink -f "/proc/$pid/exe"); else binary=$(command -v xray); fi
tmp=$(mktemp "$dir/.pilot-config.XXXXXX")
trap 'rm -f "$tmp" "$tmp.probes"' EXIT
IFS= read -r envelope
printf '%s' "$envelope" > "$tmp"
unset envelope
echo PILOT:PHASE:validating
python3 - "$tmp" <<'PILOT_EXTRACT'
import json,sys,os
p=sys.argv[1]
data=json.load(open(p))
with open(p,"w") as f: json.dump(data.pop("config"),f,ensure_ascii=False,indent=2)
with open(p+".probes","x") as f: json.dump(data,f)
import socket,ipaddress,urllib.parse
host=urllib.parse.urlparse(data["probe_url"]).hostname
addresses=socket.getaddrinfo(host,443,type=socket.SOCK_STREAM)
if not addresses or any(not ipaddress.ip_address(a[4][0]).is_global for a in addresses):
    print("PREFLIGHT_FAILED");sys.exit(24)
PILOT_EXTRACT
service_user=$(systemctl show xray -p User --value)
service_group=$(systemctl show xray -p Group --value)
test -n "$service_user" || service_user=root
test -n "$service_group" || service_group=$(id -gn "$service_user")
chown "$service_user:$service_group" "$tmp"
chmod 600 "$tmp"
# Strict policy is enabled only on the baseline whose configuration was verified.
if grep -q 'ForceIPv' "$tmp"; then
 "$binary" version | head -1 | grep -Eq 'Xray 26\.3\.27([[:space:]]|$)' || { echo VERSION_UNSUPPORTED; exit 22; }
fi
if grep -q '"direct-v6"' "$tmp"; then
 ip -6 route show default | grep -q default || { echo IPV6_UNAVAILABLE; exit 23; }
 probe_url=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["probe_url"])' "$tmp.probes")
 timeout 10 curl -6 -fsS --max-time 8 "$probe_url" >/dev/null || { echo IPV6_UNAVAILABLE; exit 23; }
fi
if ! timeout 30 "$binary" run -test -config "$tmp" >/dev/null 2>&1; then echo PREFLIGHT_FAILED; exit 24; fi
sync -f "$tmp"
echo PILOT:PHASE:staged
__VALIDATE__
echo PILOT:READY
IFS= read -r decision
test "$decision" = COMMIT || exit 29
backup="$dir/.pilot-previous.json"
test ! -L "$backup"
cp -p "$path" "$backup"
chmod 600 "$backup"
sync -f "$backup"
# Verify the original file again immediately before replacement.
actual=$(sha256sum "$path" | cut -d ' ' -f1)
test "$actual" = __HASH__ || { echo DRIFT_CONFLICT; exit 20; }
mv -f "$tmp" "$path"
sync -f "$dir"
printf '%s\n' persisted > "$dir/.pilot-config-state"
echo PILOT:PHASE:persisted
__RESTART__
`
	s = strings.ReplaceAll(s, "__PATH__", shellQuote(path))
	s = strings.ReplaceAll(s, "__HASH__", shellQuote(expected))
	pre := ""
	if validateOnly {
		pre = "echo VALIDATED; exit 0"
	}
	s = strings.ReplaceAll(s, "__VALIDATE__", pre)
	apply := "echo PERSISTED"
	if restart {
		apply = `healthy() {
 systemctl is-active --quiet xray &&
 timeout 5 "$binary" api statsquery --server=127.0.0.1:10085 >/dev/null 2>&1 &&
 timeout 30 python3 - "$tmp.probes" "$binary" <<'PILOT_HEALTH'
__PROXY_HEALTH__
PILOT_HEALTH
}
echo PILOT:PHASE:applying
if systemctl restart xray; then
 sleep 2
 echo PILOT:PHASE:verifying
 if healthy; then
  python3 - "$path" "$dir" <<'PILOT_RECEIPT'
import hashlib,json,os,shutil,sys,time,subprocess,tempfile
p,d=sys.argv[1:]
receipt={"phase":"verified","hash":hashlib.sha256(open(p,"rb").read()).hexdigest(),"process":subprocess.check_output(["systemctl","show","xray","-p","MainPID","-p","ExecMainStartTimestampMonotonic","--value"],text=True).strip()}
state=d+"/.pilot-config-state"
fd,receipt_tmp=tempfile.mkstemp(prefix=".pilot-receipt.",dir=d)
with os.fdopen(fd,"w") as f:
    json.dump(receipt,f);f.flush();os.fsync(f.fileno())
os.replace(receipt_tmp,state)
backups=d+"/.pilot-healthy"
if os.path.islink(backups): raise RuntimeError("unsafe backup directory")
os.makedirs(backups,mode=0o700,exist_ok=True);os.chmod(backups,0o700)
target=backups+"/config."+receipt["hash"]+"."+str(time.time_ns())+".json"
shutil.copyfile(p,target);os.chmod(target,0o600)
files=sorted((os.path.join(backups,x) for x in os.listdir(backups) if x.startswith("config.") and x.endswith(".json") and not os.path.islink(os.path.join(backups,x))),key=os.path.getmtime,reverse=True)
for i,f in enumerate(files):
    if i>=3 or time.time()-os.path.getmtime(f)>7*86400: os.unlink(f)
PILOT_RECEIPT
  echo VERIFIED; exit 0
 fi
fi
echo HEALTH_FAILED
__ROLLBACK__
`
		rollback := `systemctl stop xray || true
printf '%s\n' authorization_stopped > "$dir/.pilot-config-state"
echo AUTHORIZATION_STOPPED
exit 26`
		apply = strings.ReplaceAll(apply, "__ROLLBACK__", rollback)
		apply = strings.ReplaceAll(apply, "__PROXY_HEALTH__", proxyHealthPython)
	}
	return strings.ReplaceAll(s, "__RESTART__", apply)
}
