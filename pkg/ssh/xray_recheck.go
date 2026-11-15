package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const xrayPathCheckScript = `# PILOT:CHECK_PATH
execstart=$(systemctl show xray -p ExecStart --value)
if ! python3 - "$path" "$execstart" <<'PILOT_PATH'
import shlex,sys
parts=shlex.split(sys.argv[2].replace(";"," "))
paths=[parts[i+1] for i,x in enumerate(parts[:-1]) if x in ("-c","-config")]+[x.split("=",1)[1] for x in parts if x.startswith(("-c=","-config="))]
sys.exit(0 if paths==[sys.argv[1]] and not any(x.startswith("-confdir") for x in parts) else 1)
PILOT_PATH
then echo PATH_MISMATCH; exit 21; fi
`

func (c *Client) verifyXrayPath(path string) error {
	if _, err := c.RunBounded("path="+shellQuote(path)+"\n"+xrayPathCheckScript, 10*time.Second); err != nil {
		return fmt.Errorf("PATH_MISMATCH：服务配置路径未确认，拒绝应用")
	}
	return nil
}

// Recheck a restored or live-updated process without rewriting its config or
// restarting it. Disk equality alone never authorizes this path: callers must
// also supply the process instance that acknowledged this exact configuration.
func (c *Client) recheckXrayConfig(content, path, process, probeURL string) error {
	envelope, err := probeEnvelope(content, probeURL)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(envelope), &payload); err != nil {
		return err
	}
	delete(payload, "config")
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256([]byte(content))
	script := `set -eu
# PILOT:VERIFY_ONLY -- never replace config or restart the service.
umask 077
path=__PATH__
dir=$(dirname "$path")
test ! -L "$path"
test ! -L "$dir/.pilot-config.lock"
exec 9>"$dir/.pilot-config.lock"
flock -w 15 9
__CHECK_PATH__
check_current() {
 test "$(sha256sum "$path" | cut -d ' ' -f1)" = __HASH__ &&
 systemctl is-active --quiet xray &&
 test "$(systemctl show xray -p MainPID -p ExecMainStartTimestampMonotonic --value)" = __PROCESS__
}
check_current
pid=$(systemctl show xray -p MainPID --value)
test "$pid" != 0
binary=$(readlink -f "/proc/$pid/exe")
tmp=$(mktemp "$dir/.pilot-verify.XXXXXX")
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
timeout 5 "$binary" api statsquery --server=127.0.0.1:10085 >/dev/null 2>&1
timeout 30 python3 - "$tmp" "$binary" <<'PILOT_HEALTH'
__HEALTH__
PILOT_HEALTH
check_current
echo VERIFIED
`
	script = strings.ReplaceAll(script, "__PATH__", shellQuote(path))
	script = strings.ReplaceAll(script, "__CHECK_PATH__", xrayPathCheckScript)
	script = strings.ReplaceAll(script, "__HASH__", shellQuote(hex.EncodeToString(sum[:])))
	script = strings.ReplaceAll(script, "__PROCESS__", shellQuote(process))
	script = strings.ReplaceAll(script, "__HEALTH__", proxyHealthPython)
	sess, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdin = strings.NewReader(string(raw))
	var output limitedOutput
	sess.Stdout, sess.Stderr = &output, &output
	timer := time.AfterFunc(65*time.Second, func() { _ = sess.Close() })
	defer timer.Stop()
	if err := sess.Run("timeout 60 bash -c " + shellQuote(script)); err != nil || !strings.Contains(output.text, "VERIFIED") {
		return fmt.Errorf("运行配置复核失败")
	}
	return nil
}
