package ssh

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the actual probe client with the production temporary config path. Only the
// public HTTP request is replaced, so this also catches Xray CLI format errors.
func TestActualProbeClientBoundedRetry(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to the supported Xray binary")
	}
	python, err := exec.LookPath("python")
	if err != nil {
		python, err = exec.LookPath("python3")
	}
	if err != nil {
		t.Skip("Python unavailable")
	}
	for _, persistent := range []bool{false, true} {
		name := "transient"
		if persistent {
			name = "persistent"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			payload := filepath.Join(dir, "probe-envelope.json")
			data := map[string]any{"ports": []int{}, "probe_url": "https://probe.invalid/", "probes": []any{map[string]any{"log": map[string]string{"loglevel": "none"}, "outbounds": []any{map[string]string{"protocol": "freedom"}}}}}
			raw, _ := json.Marshal(data)
			if err := os.WriteFile(payload, raw, 0600); err != nil {
				t.Fatal(err)
			}
			prefix := `import socket,subprocess,sys,os
real_lookup=socket.getaddrinfo
def lookup(host,*args,**kwargs):
    if host=='probe.invalid': return [(socket.AF_INET,socket.SOCK_STREAM,6,'',('93.184.216.34',443))]
    return real_lookup(host,*args,**kwargs)
socket.getaddrinfo=lookup
attempts=0
def request(args,**kwargs):
    global attempts
    attempts+=1
    assert args[0]=='curl' and args[args.index('--noproxy')+1]==''
    endpoint=args[args.index('--socks5-hostname')+1].split(':')
    with socket.create_connection((endpoint[0],int(endpoint[1])),timeout=1): pass
    open(sys.argv[1]+'.attempts','w').write(str(attempts))
    if attempts==1 or os.environ.get('PILOT_PERSISTENT')=='1':
        raise subprocess.CalledProcessError(28,args)
    return subprocess.CompletedProcess(args,0)
subprocess.run=request
`
			cmd := exec.Command(python, "-c", prefix+proxyHealthPython, payload, binary)
			flag := "0"
			if persistent {
				flag = "1"
			}
			cmd.Env = append(os.Environ(), "PILOT_PERSISTENT="+flag)
			out, runErr := cmd.CombinedOutput()
			if (runErr != nil) != persistent {
				t.Fatalf("probe outcome: %v %s", runErr, out)
			}
			attempts, _ := os.ReadFile(payload + ".attempts")
			if string(attempts) != "2" {
				t.Fatalf("expected exactly two requests: %s %s", attempts, out)
			}
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".pilot-probe.") {
					t.Fatalf("probe credentials left behind: %s", entry.Name())
				}
			}
		})
	}
}
