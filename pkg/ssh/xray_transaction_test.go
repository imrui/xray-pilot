package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
		name                         string
		fail, validate, drift, abort bool
	}{
		{"preflight-failure", true, false, false, false}, {"preview-only", false, true, false, false},
		{"drift-conflict", false, false, true, false}, {"stale-before-commit", false, false, false, true}, {"atomic-persist", false, false, false, false},
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
			write("xray", "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Xray 26.3.27'; exit 0; fi\n[ \"$PILOT_FAIL\" != 1 ]\n")
			write("systemctl", "#!/bin/sh\ncase \"$*\" in *ExecStart*) printf 'xray run -config %s\\n' \"$PILOT_CONFIG\" ;; *MainPID*) echo 0 ;; *User*) echo xray ;; *Group*) echo xray ;; *) exit 0 ;; esac\n")
			// External system tools are isolated; this test exercises script control flow and file replacement.
			write("flock", "#!/bin/sh\nexit 0\n")
			write("chown", "#!/bin/sh\nexit 0\n")
			write("sync", "#!/bin/sh\nexit 0\n")
			write("python3", "#!/bin/sh\nexec '"+msys(python)+"' \"$@\"\n")
			write("socket.py", "from _socket import *\ndef getaddrinfo(*a,**kw): return [(AF_INET,SOCK_STREAM,6,'',('93.184.216.34',443))]\n")
			path := filepath.Join(dir, "config.json")
			old := `{"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"old"}]}`
			next := `{"inbounds":[],"outbounds":[{"protocol":"freedom","tag":"new"}]}`
			os.WriteFile(path, []byte(old), 0600)
			hash := sha256.Sum256([]byte(old))
			expected := hex.EncodeToString(hash[:])
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
			script := "PATH=\"$PILOT_BIN:$PATH\"\n" + xrayTransactionScript(msys(path), expected, false, tc.validate)
			scriptPath := filepath.Join(dir, "apply.sh")
			os.WriteFile(scriptPath, []byte(script), 0600)
			cmd := exec.Command(bash, msys(scriptPath))
			cmd.Stdin = strings.NewReader(envelope + "\n" + decision + "\n")
			fail := "0"
			if tc.fail {
				fail = "1"
			}
			cmd.Env = append(os.Environ(), "PILOT_BIN="+msys(bin), "PILOT_CONFIG="+filepath.ToSlash(path), "PILOT_FAIL="+fail, "PYTHONPATH="+bin)
			out, runErr := cmd.CombinedOutput()
			shouldFail := tc.fail || tc.drift || tc.abort
			if (runErr != nil) != shouldFail {
				t.Fatalf("exit mismatch: %v\n%s", runErr, out)
			}
			actual, _ := os.ReadFile(path)
			if tc.fail || tc.validate || tc.drift || tc.abort {
				if string(actual) != old {
					t.Fatal("live file changed before commit")
				}
			} else {
				var payload map[string]any
				if json.Unmarshal(actual, &payload) != nil || !strings.Contains(string(actual), "new") {
					t.Fatal("candidate not persisted")
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
