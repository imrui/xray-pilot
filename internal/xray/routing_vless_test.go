package xray

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/config"
)

const realityTestKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE"
const vlessTestID = "11111111-1111-4111-8111-111111111111"
const vlessTestEncryption = "mlkem768x25519plus.native.0rtt.wc9t2XT5n50jdSPnzLVpuzSZ9yH35sEUwiAfHD5ko00"

func realityTestOutbound(r, network, flow string, flat bool) string {
	account := `"id":"` + vlessTestID + `","encryption":"none","flow":"` + flow + `"`
	server := `"address":"192.0.2.1","port":443`
	settings := `{` + server + `,` + account + `}`
	if !flat {
		settings = `{"vnext":[{` + server + `,"users":[{` + account + `}]}]}`
	}
	return `{"tag":"egress","protocol":"vless","settings":` + settings + `,"streamSettings":{"network":"` + network + `","security":"reality","realitySettings":` + r + `}}`
}

func TestRealityOutboundValidation(t *testing.T) {
	key := `"password":"` + realityTestKey + `"`
	for _, tc := range []struct {
		name, fields, network, flow string
		valid                       bool
	}{
		{"defaults", key, "tcp", "", true},
		{"legacy alias", `"publicKey":"` + realityTestKey + `"`, "raw", "xtls-rprx-vision-udp443", true},
		{"grpc", key + `,"spiderX":"/x?a=b","shortId":"aa00","fingerprint":"firefox"`, "grpc", "", true},
		{"xhttp", key + `,"shortId":""`, "xhttp", "", true},
		{"missing key", "", "tcp", "", false},
		{"bad length", `"publicKey":"abc"`, "tcp", "", false},
		{"valid password wins", key + `,"publicKey":"unused"`, "tcp", "xtls-rprx-vision", true},
		{"invalid password wins", `"password":"bad","publicKey":"` + realityTestKey + `"`, "tcp", "", false},
		{"odd shortId", key + `,"shortId":"abc"`, "tcp", "", false},
		{"long shortId", key + `,"shortId":"112233445566778899"`, "tcp", "", false},
		{"invalid shortId", key + `,"shortId":"zz"`, "tcp", "", false},
		{"path", key + `,"spiderX":"relative"`, "tcp", "", false},
		{"percent", key + `,"spiderX":"/bad%g"`, "tcp", "", false},
		{"unsupported fingerprint", key + `,"fingerprint":"unknown"`, "tcp", "", false},
		{"unknown named fingerprint", key + `,"fingerprint":"hellofoo"`, "tcp", "", false},
		{"valid named fingerprint", key + `,"fingerprint":"HelloChrome_120"`, "tcp", "", true},
		{"inbound names", key + `,"serverNames":["a"]`, "tcp", "", false},
		{"TLS12 only", key + `,"fingerprint":"android"`, "tcp", "", false},
		{"ws", key, "ws", "", false},
		{"vision grpc", key, "grpc", "xtls-rprx-vision", false},
		{"unknown flow", key, "tcp", "vision", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, flat := range []bool{true, false} {
				out := realityTestOutbound("{"+tc.fields+"}", tc.network, tc.flow, flat)
				_, err := DecodeRoutingPolicy(securityPolicy(out))
				if (err == nil) != tc.valid {
					t.Errorf("flat=%t template validation=%v", flat, err)
				}
				err = ValidateRoutingOverrides(`{"outbound_overrides":{"egress":{"mode":"replace","outbound":` + out + `}}}`)
				if (err == nil) != tc.valid {
					t.Errorf("flat=%t override validation=%v", flat, err)
				}
			}
		})
	}
}

func TestVLESSOutboundRequiresEncryption(t *testing.T) {
	for _, format := range []string{"vnext", "shorthand", "shorthand-empty-vnext"} {
		out := realityTestOutbound(`{"password":"`+realityTestKey+`"}`, "tcp", "", format != "vnext")
		if format == "vnext" {
			// 首名用户合法，仍须校验后续用户。
			out = strings.Replace(out, `"users":[`, `"users":[{"id":"22222222-2222-4222-8222-222222222222","encryption":"none"},`, 1)
		} else if format == "shorthand-empty-vnext" {
			out = strings.Replace(out, `"settings":{`, `"settings":{"vnext":[],`, 1)
		}
		for _, tc := range []struct{ name, field string }{
			{"missing", ""}, {"empty", `"encryption":"",`}, {"whitespace", `"encryption":"  ",`},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				invalid := strings.Replace(out, `"encryption":"none","flow"`, tc.field+`"flow"`, 1)
				_, err := DecodeRoutingPolicy(securityPolicy(invalid))
				if err == nil || !strings.Contains(err.Error(), "encryption") {
					t.Errorf("template: expected encryption error, got %v", err)
				}
				err = ValidateRoutingOverrides(`{"outbound_overrides":{"egress":{"mode":"replace","outbound":` + invalid + `}}}`)
				if err == nil || !strings.Contains(err.Error(), "encryption") {
					t.Errorf("override: expected encryption error, got %v", err)
				}
			})
		}
	}
}

func TestVLESSAccountFieldNamesAndEncryption(t *testing.T) {
	for _, flat := range []bool{false, true} {
		valid := realityTestOutbound(`{"password":"`+realityTestKey+`"}`, "tcp", "xtls-rprx-vision", flat)
		valid = strings.Replace(valid, `"encryption":"none"`, `"encryption":"`+vlessTestEncryption+`"`, 1)
		// 扩展传输字段中的 HTTP 头允许任意大小写。
		valid = strings.Replace(valid, `"network":"tcp"`, `"network":"tcp","wsSettings":{"headers":{"Flow":"custom","Encryption":"custom"}}`, 1)
		for _, tc := range []struct{ field, spelling string }{
			{"", ""}, {"encryption", "Encryption"}, {"flow", "Flow"},
		} {
			out := valid
			if tc.field != "" {
				out = strings.Replace(out, `"`+tc.field+`":`, `"`+tc.spelling+`":`, 1)
			}
			_, templateErr := DecodeRoutingPolicy(securityPolicy(out))
			overrideErr := ValidateRoutingOverrides(`{"outbound_overrides":{"egress":{"mode":"replace","outbound":` + out + `}}}`)
			for _, err := range []error{templateErr, overrideErr} {
				if tc.field == "" && err != nil {
					t.Errorf("flat=%t: rejected VLESS Encryption: %v", flat, err)
				}
				if tc.field != "" && (err == nil || !strings.Contains(err.Error(), tc.spelling)) {
					t.Errorf("flat=%t: expected noncanonical %s error, got %v", flat, tc.spelling, err)
				}
			}
		}
	}
}

func TestRealityLegacyKeyEncryptionAndMasking(t *testing.T) {
	old := config.Global.Crypto.MasterKey
	config.Global.Crypto.MasterKey = strings.Repeat("23", 32)
	t.Cleanup(func() { config.Global.Crypto.MasterKey = old })
	out := realityTestOutbound(`{"publicKey":"`+realityTestKey+`"}`, "tcp", "", false)
	raw := securityPolicy(out)
	ov := `{"outbound_overrides":{"egress":{"mode":"replace","outbound":` + out + `}}}`
	plain, err := DecodeRoutingPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncryptPolicySecrets(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(enc)
	if err != nil || strings.Contains(string(data), realityTestKey) {
		t.Fatal("plaintext persisted")
	}
	if _, err = DecodeRoutingPolicy(string(data)); err != nil {
		t.Fatal("encrypted policy rejected:", err)
	}
	same, err := EncryptPolicySecrets(plain, &enc)
	if err != nil || !reflect.DeepEqual(enc, same) {
		t.Fatal("resave changed ciphertext", err)
	}
	decrypted, err := DecryptPolicySecrets(enc)
	if err != nil || !strings.Contains(string(decrypted.CustomOutbounds[0].StreamSettings), realityTestKey) {
		t.Fatal("legacy key did not survive rendering")
	}
	encOv, err := EncryptOverridesSecrets(ov, "", &plain)
	if err != nil || strings.Contains(encOv, realityTestKey) {
		t.Fatal("override key not encrypted", err)
	}
	for _, text := range []string{MaskPolicyJSON(raw), MaskPolicyJSON(strings.Replace(raw, "publicKey", "PUBLICKEY", 1)), MaskOverridesJSON(ov), MaskOverridesJSON(encOv)} {
		if strings.Contains(text, realityTestKey) || !strings.Contains(text, "masked") {
			t.Fatal("legacy key not masked")
		}
	}
	wg := maskExtraRawJSON(json.RawMessage(`{"peers":[{"publicKey":"visible"}]}`))
	if !strings.Contains(string(wg), "visible") {
		t.Fatal("WireGuard public key was masked")
	}
}

func TestActualXrayVLESSReality(t *testing.T) {
	binary := os.Getenv("XRAY_TEST_BINARY")
	if binary == "" {
		t.Skip("set XRAY_TEST_BINARY to check the selected core")
	}
	runCore := func(t *testing.T, raw string) ([]byte, error) {
		t.Helper()
		path := filepath.Join(t.TempDir(), "reality.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "run", "-test", "-format", "json", "-config", path)
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+filepath.Dir(binary))
		return cmd.CombinedOutput()
	}
	for _, format := range []string{"vnext", "shorthand"} {
		flat := format == "shorthand"
		for _, key := range []string{"password", "publicKey"} {
			out := realityTestOutbound(`{"`+key+`":"`+realityTestKey+`","fingerprint":"chrome","shortId":"abcd","spiderX":"/"}`, "tcp", "xtls-rprx-vision", flat)
			p, err := DecodeRoutingPolicy(securityPolicy(out))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := ApplyRoutingPolicy(`{"log":{"loglevel":"none"},"inbounds":[],"outbounds":[],"routing":{"rules":[]}}`, p)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(format+"/"+key, func(t *testing.T) {
				if output, err := runCore(t, raw); err != nil {
					t.Fatalf("%v: %s", err, output)
				}
			})
		}
		t.Run(format+"/vless-encryption", func(t *testing.T) {
			out := realityTestOutbound(`{"password":"`+realityTestKey+`"}`, "tcp", "xtls-rprx-vision", flat)
			out = strings.Replace(out, `"encryption":"none"`, `"encryption":"`+vlessTestEncryption+`"`, 1)
			if output, err := runCore(t, `{"outbounds":[`+out+`]}`); err != nil {
				t.Fatalf("core rejected VLESS Encryption: %v %s", err, output)
			}
		})
		t.Run(format+"/missing-encryption", func(t *testing.T) {
			out := realityTestOutbound(`{"password":"`+realityTestKey+`"}`, "tcp", "", flat)
			out = strings.Replace(out, `"encryption":"none",`, "", 1)
			output, err := runCore(t, `{"outbounds":[`+out+`]}`)
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 || !strings.Contains(string(output), `please add/set "encryption":"none"`) {
				t.Fatalf("expected core to reject missing encryption with exit 23: %v %s", err, output)
			}
		})
	}
}
