package xray

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func wireGuardReservedPolicy(reserved, peerExtra string) string {
	field := ""
	if reserved != "" {
		field = `,"reserved":` + reserved
	}
	privateKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	publicKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	return securityPolicy(`{"tag":"egress","protocol":"wireguard","settings":{"secretKey":"` + privateKey + `","address":["172.16.0.2/32"],"noKernelTun":true` + field + `,"peers":[{"endpoint":"192.0.2.1:2408","publicKey":"` + publicKey + `"` + peerExtra + `}]}}`)
}

func TestWireGuardReservedValidation(t *testing.T) {
	for _, reserved := range []string{"", "[]", "null", "[0,1,255]"} {
		if _, err := DecodeRoutingPolicy(wireGuardReservedPolicy(reserved, "")); err != nil {
			t.Fatalf("valid reserved %s rejected: %v", reserved, err)
		}
	}
	for _, reserved := range []string{"[1]", "[1,2]", "[1,2,3,4]", "[-1,0,1]", "[0,1,256]", "[0,1,1.5]", `[0,1,"2"]`, `"AQID"`, "{}"} {
		if _, err := DecodeRoutingPolicy(wireGuardReservedPolicy(reserved, "")); err == nil || !strings.Contains(err.Error(), "settings.reserved") {
			t.Errorf("invalid reserved %s not rejected precisely: %v", reserved, err)
		}
	}
	for _, peerReserved := range []string{"[1,2,3]", "[]", "null"} {
		if _, err := DecodeRoutingPolicy(wireGuardReservedPolicy("", `,"reserved":`+peerReserved)); err == nil || !strings.Contains(err.Error(), "请移至 settings.reserved") {
			t.Errorf("peer reserved %s silently accepted: %v", peerReserved, err)
		}
	}
}

// 只执行真实内核配置解析；noKernelTun 兼容 Windows，不启动服务或发起握手。
func TestActualXrayWireGuardReserved(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to the Xray binary under test")
	}
	p, err := DecodeRoutingPolicy(wireGuardReservedPolicy("[1,2,3]", ""))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := ApplyRoutingPolicy(`{"log":{"loglevel":"none"},"inbounds":[],"outbounds":[],"routing":{"rules":[]}}`, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, valid := range []bool{true, false} {
		name := "valid-three-bytes"
		if !valid {
			name = "invalid-length"
		}
		t.Run(name, func(t *testing.T) {
			var cfg map[string]any
			if err := json.Unmarshal([]byte(rendered), &cfg); err != nil {
				t.Fatal(err)
			}
			if !valid {
				for _, raw := range cfg["outbounds"].([]any) {
					out := raw.(map[string]any)
					if out["protocol"] == "wireguard" {
						out["settings"].(map[string]any)["reserved"] = []int{1, 2}
					}
				}
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "wireguard-reserved.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", path)
			cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
			output, err := cmd.CombinedOutput()
			if valid && err != nil {
				t.Fatalf("valid reserved rejected by actual Xray: %v\n%s", err, output)
			}
			if !valid && (err == nil || !strings.Contains(strings.ToLower(strings.ReplaceAll(string(output), path, "")), "reserved")) {
				t.Fatalf("invalid reserved length was not rejected by actual Xray: %v\n%s", err, output)
			}
		})
	}
}
