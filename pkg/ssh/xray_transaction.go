package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"strings"
	"sync"
	"time"
)

// ApplyXrayConfig validates before replacing, serializes remote writers and refuses
// rollback across an authorization change. validateOnly never mutates live config.
type TransactionOptions struct {
	BaselineHash        string
	AllowInitial        bool
	InitialBaselineHash string
	OnBaseline          func(string) error
	BeforeApply         func() error
	ProbeURL            string
	OnPhase             func(string)
	RollbackContent     string
	// ConfirmedProcess is supplied only for this exact content's acknowledged runtime.
	ConfirmedProcess string
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
	if restart && !validateOnly && crypto.HashConfig(old) == crypto.HashConfig(content) {
		if err := c.verifyXrayPath(path); err != nil {
			return err
		}
		if _, err := c.VerifyConfigReceipt(old, path); err == nil {
			return nil
		}
		process, err := c.VerifyRecoveryReceipt(old, path)
		if err != nil {
			process = opt.ConfirmedProcess
		}
		process = strings.TrimSpace(process)
		if process == "" {
			return fmt.Errorf("节点配置任务未完成：目标文件已存在，缺少运行回执，不重复重启")
		}
		if opt.BeforeApply != nil {
			if err := opt.BeforeApply(); err != nil {
				return err
			}
		}
		if err := c.recheckXrayConfig(old, path, process, opt.ProbeURL); err != nil {
			return fmt.Errorf("节点配置任务未完成：目标文件已存在，运行健康尚未确认，不重复重启")
		}
		if opt.BeforeApply != nil {
			return opt.BeforeApply()
		}
		return nil
	}
	if !validateOnly {
		if err := acceptConfigBaseline(old, content, opt); err != nil {
			return err
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
	var payload map[string]any
	_ = json.Unmarshal([]byte(envelope), &payload)
	rollback, err := rollbackCandidate(old, content, opt.RollbackContent)
	if err != nil && restart && !validateOnly {
		return err
	}
	payload["rollback"] = rollback
	encoded, _ := json.Marshal(payload)
	envelope = string(encoded)
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
		if line == "PILOT:READY" || line == "PILOT:ROLLBACK_READY" {
			if opt.BeforeApply != nil {
				if err := opt.BeforeApply(); err != nil {
					checkpointErr <- err
					_, _ = stdin.Write([]byte("ABORT\n"))
					_ = stdin.Close()
					return
				}
			}
			_, _ = stdin.Write([]byte("COMMIT\n"))
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
		if strings.Contains(output.text, "AUTHORIZATION_STOPPED") {
			return fmt.Errorf("AUTHORIZATION_STOPPED：%w", checkErr)
		}
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

func acceptConfigBaseline(old, desired string, opt TransactionOptions) error {
	if crypto.HashConfig(old) == crypto.HashConfig(desired) {
		return nil
	}
	if opt.BaselineHash != "" && crypto.HashConfig(old) == opt.BaselineHash {
		return nil
	}
	raw := sha256.Sum256([]byte(old))
	if opt.BaselineHash == "" && opt.AllowInitial && opt.InitialBaselineHash == hex.EncodeToString(raw[:]) {
		// Establish the attested baseline before preflight. A preflight failure
		// can then be retried without granting permission for a different file.
		if opt.OnBaseline != nil {
			return opt.OnBaseline(crypto.HashConfig(old))
		}
		return nil
	}
	return fmt.Errorf("DRIFT_CONFLICT：当前文件与纳管基线不一致，请先核对")
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
__CHECK_PATH__
pid=$(systemctl show xray -p MainPID --value)
if test "$pid" != 0 && test -e "/proc/$pid/exe"; then binary=$(readlink -f "/proc/$pid/exe"); else binary=$(command -v xray); fi
tmp=$(mktemp "$dir/.pilot-config.XXXXXX")
trap 'rm -f "$tmp" "$tmp.probes" "$tmp.rollback"' EXIT
IFS= read -r envelope
printf '%s' "$envelope" > "$tmp"
unset envelope
echo PILOT:PHASE:validating
python3 - "$tmp" <<'PILOT_EXTRACT'
import json,sys,os
p=sys.argv[1]
data=json.load(open(p))
with open(p,"w") as f: json.dump(data.pop("config"),f,ensure_ascii=False,indent=2)
with open(p+".rollback","x") as f: f.write(data.pop("rollback", ""))
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
chown "$service_user:$service_group" "$tmp.rollback"
chmod 600 "$tmp.rollback"
# Strict policy is enabled only on the baseline whose configuration was verified.
if grep -q 'ForceIPv' "$tmp"; then
 "$binary" version | head -1 | grep -Eq 'Xray 26\.3\.27([[:space:]]|$)' || { echo VERSION_UNSUPPORTED; exit 22; }
fi
if grep -q '"direct-v6"' "$tmp"; then
 ip -6 route show default | grep -q default || { echo IPV6_UNAVAILABLE; exit 23; }
 probe_url=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["probe_url"])' "$tmp.probes")
 timeout 10 curl -6 -fsS --max-time 8 "$probe_url" >/dev/null || { echo IPV6_UNAVAILABLE; exit 23; }
fi
if ! timeout 30 "$binary" run -test -format json -config "$tmp" >/dev/null 2>&1; then echo PREFLIGHT_FAILED; exit 24; fi
__ROLLBACK_PREFLIGHT__
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
	s = strings.ReplaceAll(s, "__CHECK_PATH__", xrayPathCheckScript)
	s = strings.ReplaceAll(s, "__HASH__", shellQuote(expected))
	pre := ""
	if validateOnly {
		pre = "echo VALIDATED; exit 0"
	}
	s = strings.ReplaceAll(s, "__VALIDATE__", pre)
	rollbackPreflight := ""
	if restart {
		rollbackPreflight = `command -v curl >/dev/null || { echo PREFLIGHT_FAILED; exit 24; }
 timeout 30 "$binary" run -test -format json -config "$tmp.rollback" >/dev/null 2>&1 || { echo PREFLIGHT_FAILED; exit 24; }`
	}
	s = strings.ReplaceAll(s, "__ROLLBACK_PREFLIGHT__", rollbackPreflight)
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
  process=$(systemctl show xray -p MainPID -p ExecMainStartTimestampMonotonic --value)
  python3 - "$path" "$dir" "$process" <<'PILOT_RECEIPT'
import hashlib,json,os,shutil,sys,time,tempfile,uuid
p,d,process=sys.argv[1:]
receipt={"phase":"verified","hash":hashlib.sha256(open(p,"rb").read()).hexdigest(),"process":process.strip()}
state=d+"/.pilot-config-state"
fd,receipt_tmp=tempfile.mkstemp(prefix=".pilot-receipt.",dir=d)
with os.fdopen(fd,"w") as f:
    json.dump(receipt,f);f.flush();os.fsync(f.fileno())
os.replace(receipt_tmp,state)
backups=d+"/.pilot-healthy"
if os.path.islink(backups): raise RuntimeError("unsafe backup directory")
os.makedirs(backups,mode=0o700,exist_ok=True);os.chmod(backups,0o700)
target=backups+"/config."+receipt["hash"]+"."+uuid.uuid4().hex+".json"
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
		rollback := `echo PILOT:ROLLBACK_READY
IFS= read -r decision || decision=ABORT
if test "$decision" = COMMIT; then
 rollback_hash=$(sha256sum "$tmp.rollback" | cut -d ' ' -f1)
 mv -f "$tmp.rollback" "$path"
 sync -f "$dir"
 if test "$(sha256sum "$path" | cut -d ' ' -f1)" = "$rollback_hash" && systemctl restart xray && systemctl is-active --quiet xray; then
  process=$(systemctl show xray -p MainPID -p ExecMainStartTimestampMonotonic --value)
  python3 - "$path" "$dir" "$process" <<'PILOT_RECOVERY_RECEIPT'
import hashlib,json,os,sys,tempfile
p,d,process=sys.argv[1:]
receipt={"phase":"restored","hash":hashlib.sha256(open(p,"rb").read()).hexdigest(),"process":process.strip()}
fd,tmp=tempfile.mkstemp(prefix=".pilot-receipt.",dir=d)
with os.fdopen(fd,"w") as f:
    json.dump(receipt,f);f.flush();os.fsync(f.fileno())
os.replace(tmp,d+"/.pilot-config-state")
PILOT_RECOVERY_RECEIPT
  echo ROLLED_BACK; exit 27
 fi
 echo ROLLBACK_FAILED
fi
systemctl stop xray || true
printf '%s\n' authorization_stopped > "$dir/.pilot-config-state"
echo AUTHORIZATION_STOPPED
exit 26`
		apply = strings.ReplaceAll(apply, "__ROLLBACK__", rollback)
		apply = strings.ReplaceAll(apply, "__PROXY_HEALTH__", proxyHealthPython)
	}
	return strings.ReplaceAll(s, "__RESTART__", apply)
}

// 恢复文件必须满足当前授权，不能回放旧 UUID、密钥或已删除协议。
func rollbackCandidate(old, desired, fallback string) (string, error) {
	want := AuthorizationHash(desired)
	if want == "" {
		return "", fmt.Errorf("恢复配置无效")
	}
	if AuthorizationHash(old) == want {
		return old, nil
	}
	if AuthorizationHash(fallback) != want {
		return "", fmt.Errorf("PREFLIGHT_FAILED：缺少符合当前授权的恢复配置")
	}
	return fallback, nil
}

// AuthorizationHash identifies current credentials and system protection rules,
// independently of administrator-controlled destination routing (including IPs).
// Invalid JSON has no usable authorization fingerprint.
func AuthorizationHash(content string) string {
	var cfg map[string]any
	if json.Unmarshal([]byte(content), &cfg) != nil || cfg == nil {
		return ""
	}
	var protected []any
	routing, _ := cfg["routing"].(map[string]any)
	rules, _ := routing["rules"].([]any)
	for _, rule := range rules {
		m, _ := rule.(map[string]any)
		guard := m["inboundTag"] != nil || m["source"] != nil
		ips, _ := m["ip"].([]any)
		for _, ip := range ips {
			if ip == "geoip:private" {
				guard = true
			}
		}
		if guard {
			protected = append(protected, rule)
		}
	}
	raw, _ := json.Marshal(map[string]any{"inbounds": cfg["inbounds"], "protected": protected})
	return crypto.HashConfig(string(raw))
}
