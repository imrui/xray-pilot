package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTransactionStopsBeforeReplace(t *testing.T) {
	bash := os.Getenv("PILOT_TEST_BASH")
	if bash == "" {
		if runtime.GOOS == "windows" {
			t.Skip("set PILOT_TEST_BASH to Git Bash")
		} else {
			var err error
			bash, err = exec.LookPath("bash")
			if err != nil {
				t.Skip("bash unavailable")
			}
		}
	}
	python, err := exec.LookPath("python")
	if err != nil {
		python, err = exec.LookPath("python3")
	}
	if err != nil {
		t.Skip("python unavailable")
	}
	msys := func(p string) string {
		p = filepath.ToSlash(p)
		if runtime.GOOS == "windows" && len(p) > 2 && p[1] == ':' {
			return "/" + strings.ToLower(p[:1]) + p[2:]
		}
		return p
	}
	for _, tc := range []struct {
		name                                           string
		fail, validate, drift, abort                   bool
		restart, healthFail, rollbackFail, restartFail bool
		customIP, initial                              bool
	}{
		{name: "preflight-failure", fail: true}, {name: "preview-only", validate: true},
		{name: "drift-conflict", drift: true}, {name: "stale-before-commit", abort: true}, {name: "atomic-persist"},
		{name: "restart-preflight-rejected", restart: true, fail: true},
		{name: "restart-health-restores", restart: true, healthFail: true},
		{name: "restart-failure-restores", restart: true, restartFail: true},
		{name: "restart-rollback-fails", restart: true, healthFail: true, rollbackFail: true},
		{name: "custom-ip-restores", restart: true, healthFail: true, customIP: true},
		{name: "installer-sample-first-apply", restart: true, initial: true},
		{name: "api-only-first-apply", restart: true, initial: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			os.Mkdir(bin, 0700)
			write := func(name, body string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			write("xray", `#!/bin/sh
if [ "$1" = version ]; then echo 'Xray 26.3.27'; exit 0; fi
# Newer Xray requires a supported suffix in addition to an explicit format.
if [ "$1" = run ]; then
 case " $* " in *" -format json "*) ;; *) exit 1 ;; esac
 while [ "$#" -gt 0 ]; do
  if [ "$1" = -config ]; then
   shift
   case "$1" in *.json) ;; *) echo 'unsupported config file extension'; exit 1 ;; esac
  fi
  shift
 done
fi
[ "$PILOT_FAIL" != 1 ]
`)
			write("systemctl", `#!/bin/sh
case "$*" in
 *ExecStart*) printf 'xray run -config %s\n' "$PILOT_CONFIG" ;;
 *MainPID*) echo 0 ;;
 *User*|*Group*) echo xray ;;
 restart*) echo restart >> "$PILOT_CONFIG.calls"; if [ "$PILOT_RESTART_FAIL" = 1 ] && grep -q new "$PILOT_CONFIG"; then exit 1; fi; cp "$PILOT_CONFIG" "$PILOT_CONFIG.running"; if [ "$PILOT_ROLLBACK_FAIL" = 1 ] && grep -q old "$PILOT_CONFIG"; then exit 1; fi ;;
 stop*) echo stop >> "$PILOT_CONFIG.calls" ;;
 *) exit 0 ;;
esac
`)
			write("sleep", "#!/bin/sh\nexit 0\n")
			write("curl", "#!/bin/sh\nexit 0\n")
			// External system tools are isolated; this test exercises script control flow and file replacement.
			write("flock", "#!/bin/sh\nexit 0\n")
			write("chown", "#!/bin/sh\nexit 0\n")
			write("sync", "#!/bin/sh\nexit 0\n")
			write("python3", "#!/bin/sh\nexec '"+msys(python)+"' \"$@\"\n")
			// Ubuntu 18.04 ships Python 3.6, which has no time.time_ns.
			write("sitecustomize.py", "import time\nif hasattr(time, 'time_ns'): del time.time_ns\n")
			write("socket.py", "from _socket import *\ndef getaddrinfo(*a,**kw): return [(AF_INET,SOCK_STREAM,6,'',('93.184.216.34',443))]\n")
			path := filepath.Join(dir, "config.json")
			old := `{"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"old"}]}`
			next := `{"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"new"}]}`
			if tc.initial {
				old = `{"inbounds":[{"protocol":"socks","port":1080}],"outbounds":[{"protocol":"freedom","tag":"old"}]}`
				if tc.name == "api-only-first-apply" {
					next = strings.Replace(next, `"inbounds":[]`, `"inbounds":[{"protocol":"dokodemo-door","port":10085,"listen":"127.0.0.1","settings":{"address":"127.0.0.1"}}]`, 1)
				}
			}
			if tc.customIP {
				next = strings.TrimSuffix(next, "}") + `,"routing":{"rules":[{"type":"field","ip":["203.0.113.0/24"],"outboundTag":"new"}]}}`
			}
			os.WriteFile(path, []byte(old), 0600)
			hash := sha256.Sum256([]byte(old))
			expected := hex.EncodeToString(hash[:])
			if tc.initial {
				if err := acceptConfigBaseline(old, next, TransactionOptions{AllowInitial: true, InitialBaselineHash: expected}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.drift {
				expected = strings.Repeat("0", 64)
			}
			envelope, err := probeEnvelope(next, DefaultProbeURL)
			if err != nil {
				t.Fatal(err)
			}
			decision := "COMMIT"
			if tc.abort {
				decision = "ABORT"
			}
			var payload map[string]any
			json.Unmarshal([]byte(envelope), &payload)
			rollback, err := rollbackCandidate(old, next, next)
			if err != nil {
				t.Fatal(err)
			}
			payload["rollback"] = rollback
			rawEnvelope, _ := json.Marshal(payload)
			envelope = string(rawEnvelope)
			script := "PATH=\"$PILOT_BIN:$PATH\"\n" + xrayTransactionScript(msys(path), expected, tc.restart, tc.validate)
			if tc.healthFail {
				script = strings.Replace(script, proxyHealthPython, "raise SystemExit(1)", 1)
			}
			scriptPath := filepath.Join(dir, "apply.sh")
			os.WriteFile(scriptPath, []byte(script), 0600)
			cmd := exec.Command(bash, msys(scriptPath))
			cmd.Stdin = strings.NewReader(envelope + "\n" + decision + "\nCOMMIT\n")
			fail := "0"
			if tc.fail {
				fail = "1"
			}
			cmd.Env = append(os.Environ(), "PILOT_BIN="+msys(bin), "PILOT_CONFIG="+filepath.ToSlash(path), "PILOT_FAIL="+fail, "PYTHONPATH="+bin, fmt.Sprintf("PILOT_ROLLBACK_FAIL=%d", map[bool]int{true: 1}[tc.rollbackFail]), fmt.Sprintf("PILOT_RESTART_FAIL=%d", map[bool]int{true: 1}[tc.restartFail]))
			out, runErr := cmd.CombinedOutput()
			shouldFail := tc.fail || tc.drift || tc.abort || tc.healthFail || tc.restartFail
			if (runErr != nil) != shouldFail {
				t.Fatalf("exit mismatch: %v\n%s", runErr, out)
			}
			actual, _ := os.ReadFile(path)
			if tc.fail || tc.validate || tc.drift || tc.abort || tc.healthFail || tc.restartFail {
				if string(actual) != old {
					t.Fatal("live file changed before commit")
				}
			} else {
				var payload map[string]any
				if json.Unmarshal(actual, &payload) != nil || !strings.Contains(string(actual), "new") {
					t.Fatal("candidate not persisted")
				}
			}
			if tc.restart {
				calls, _ := os.ReadFile(path + ".calls")
				if tc.fail && len(calls) > 0 {
					t.Fatal("preflight touched service")
				}
				if tc.healthFail || tc.restartFail {
					running, _ := os.ReadFile(path + ".running")
					if string(running) != old || strings.Count(string(calls), "restart") != 2 {
						t.Fatalf("rollback not bounded: %s", calls)
					}
					marker := "ROLLED_BACK"
					if tc.rollbackFail {
						marker = "ROLLBACK_FAILED"
					} else if strings.Contains(string(calls), "stop") {
						t.Fatal("healthy old service stopped")
					}
					if !strings.Contains(string(out), marker) {
						t.Fatalf("missing %s: %s", marker, out)
					}
					if !tc.rollbackFail {
						raw, _ := os.ReadFile(filepath.Join(dir, ".pilot-config-state"))
						var receipt struct{ Phase, Hash, Process string }
						if json.Unmarshal(raw, &receipt) != nil || receipt.Phase != "restored" || receipt.Hash != hex.EncodeToString(hash[:]) || receipt.Process == "" {
							t.Fatalf("missing recovery receipt: %s", raw)
						}
					}
				}
			}

			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".pilot-config.") && e.Name() != ".pilot-config.lock" {
					t.Fatalf("temporary file leaked %s", e.Name())
				}
			}
		})
	}
}
func TestProbePayloadAndInputValidation(t *testing.T) {
	for _, url := range []string{"http://example.com", "https://user:pass@example.com", "https://127.0.0.1/", "https://example.com:22", "https://example.com/?token=x"} {
		if ValidateProbeURL(url) == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	raw := `{"inbounds":[{"protocol":"vless","port":443,"settings":{"clients":[{"id":"11111111-1111-4111-8111-111111111111","flow":"xtls-rprx-vision"}]},"streamSettings":{"network":"tcp","security":"reality","realitySettings":{"privateKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","serverNames":["example.com"],"shortIds":["abcd"]}}}],"outbounds":[]}`
	payload, err := probeEnvelope(raw, DefaultProbeURL)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Config any
		Probes []map[string]any
	}
	if json.Unmarshal([]byte(payload), &v) != nil || len(v.Probes) != 1 {
		t.Fatal("missing protocol probe")
	}
	probe, _ := json.Marshal(v.Probes[0])
	if strings.Contains(string(probe), "privateKey") || !strings.Contains(string(probe), "publicKey") {
		t.Fatal("probe key derivation incorrect")
	}
}
func TestPhaseOutputAcrossChunks(t *testing.T) {
	var phases []string
	out := limitedOutput{onLine: func(v string) { phases = append(phases, v) }}
	out.Write([]byte("PILOT:PHA"))
	out.Write([]byte("SE:staged\nPILOT:READY\n"))
	if len(phases) != 2 || phases[0] != "PILOT:PHASE:staged" || phases[1] != "PILOT:READY" {
		t.Fatal(phases)
	}
	out.Write([]byte(strings.Repeat("x", 30000)))
	if len(out.text) > 8192 {
		t.Fatal("output unbounded")
	}
}

func TestRollbackCannotRestoreRevokedIdentity(t *testing.T) {
	old := `{"inbounds":[{"settings":{"clients":[{"id":"revoked"}]}}]}`
	desired := `{"inbounds":[{"settings":{"clients":[]}}]}`
	if _, err := rollbackCandidate(old, desired, old); err == nil {
		t.Fatal("old identity restored")
	}
	raw, err := rollbackCandidate(old, desired, desired)
	if err != nil || strings.Contains(raw, "revoked") {
		t.Fatal("current authorization not preserved")
	}
}

func TestPreflightDropsPrivilegesToServiceUser(t *testing.T) {
	for _, validateOnly := range []bool{false, true} {
		script := xrayTransactionScript("/usr/local/etc/xray/config.json", strings.Repeat("a", 64), !validateOnly, validateOnly)
		if !strings.Contains(script, `runuser -u "$service_user" -g "$service_group" -- "$binary" run -test -format json -config "$1"`) {
			t.Fatalf("validateOnly=%v: preflight script missing runuser privilege drop", validateOnly)
		}
		if !strings.Contains(script, `run_xray_test "$tmp"`) {
			t.Fatalf("validateOnly=%v: preflight script did not invoke run_xray_test for candidate config", validateOnly)
		}
		if !validateOnly && !strings.Contains(script, `run_xray_test "$tmp.rollback.json"`) {
			t.Fatal("restart preflight did not invoke run_xray_test for rollback config")
		}
	}
}
