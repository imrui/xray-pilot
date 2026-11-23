package xray

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/config"
)

func securityPolicy(outbound string) string {
	return `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[` + outbound + `],"custom_rules":[{"type":"field","domain":["domain:example.com"],"outboundTag":"egress"}]}`
}

func TestOutboundRejectsRoutingEscapeFields(t *testing.T) {
	for _, outbound := range []string{
		`{"tag":"egress","protocol":"loopback","settings":{"inboundTag":"api-inbound"}}`,
		`{"tag":"egress","protocol":"freedom","settings":{"redirect":"127.0.0.1:10085"}}`,
		`{"tag":"egress","protocol":"freedom","settings":{"Redirect":"127.0.0.1:10085"}}`,
		`{"tag":"egress","protocol":"freedom","proxySettings":{"tag":"api"}}`,
		`{"tag":"egress","protocol":"vmess","settings":{"reverse":{"tag":"api-inbound"}}}`,
	} {
		t.Run(outbound, func(t *testing.T) {
			if _, err := DecodeRoutingPolicy(securityPolicy(outbound)); err == nil {
				t.Fatal("accepted routing escape")
			}
			if err := ValidateRoutingOverrides(`{"outbound_overrides":{"egress":{"mode":"replace","outbound":` + outbound + `}}}`); err == nil {
				t.Fatal("node override accepted routing escape")
			}
		})
	}
}

func TestOutboundRejectsAmbiguousCredentialJSON(t *testing.T) {
	for _, settings := range []string{
		`{"Servers":[{"address":"203.0.113.1","port":1080}]}`,
		`{"servers":[{"address":"203.0.113.1","port":1080,"Users":[{"user":"ops","pass":"secret"}]}]}`,
		`{"servers":[{"address":"203.0.113.1","port":1080,"users":[{"user":"ops","Pass":"secret"}]}]}`,
		`{"servers":[{"address":"203.0.113.1","port":1080,"users":[{"pass":"first","pass":"second"}]}]}`,
		`{"servers":[{"address":"203.0.113.1","port":1080,"users":[{"pass":"first","PASS":"second"}]}]}`,
	} {
		if _, err := DecodeRoutingPolicy(securityPolicy(`{"tag":"egress","protocol":"socks","settings":` + settings + `}`)); err == nil {
			t.Errorf("accepted ambiguous settings: %s", settings)
		}
	}
	for _, outbound := range []string{
		`{"tag":"egress","protocol":"freedom","TargetStrategy":"ForceIPv6"}`,
		`{"tag":"egress","protocol":"freedom","settings":{"DomainStrategy":"ForceIPv6"}}`,
		`{"tag":"egress","protocol":"freedom","streamSettings":{"sockopt":{"DomainStrategy":"ForceIPv6"}}}`,
	} {
		if _, err := DecodeRoutingPolicy(securityPolicy(outbound)); err == nil {
			t.Fatal("accepted noncanonical dialing strategy")
		}
	}
}

func TestOutboundFlatAndExtensionCredentialsStayEncrypted(t *testing.T) {
	oldKey := config.Global.Crypto.MasterKey
	config.Global.Crypto.MasterKey = strings.Repeat("22", 32)
	t.Cleanup(func() { config.Global.Crypto.MasterKey = oldKey })
	for _, tc := range []struct{ protocol, key string }{
		{"socks", "pass"}, {"http", "pass"}, {"vless", "id"}, {"vmess", "id"}, {"trojan", "password"}, {"shadowsocks", "password"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			encryption := ""
			if tc.protocol == "vless" {
				encryption = `,"encryption":"none"`
			}
			raw := `{"tag":"egress","protocol":"` + tc.protocol + `","settings":{"address":"203.0.113.1","port":443,"` + tc.key + `":"flat-protocol-secret"` + encryption + `}}`
			p, err := DecodeRoutingPolicy(securityPolicy(raw))
			if err != nil {
				t.Fatal(err)
			}
			enc, err := EncryptPolicySecrets(p, nil)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(enc)
			if strings.Contains(string(data), "flat-protocol-secret") || !strings.Contains(string(data), OutboundSecretPrefix) {
				t.Fatal("flat protocol credential not encrypted")
			}
		})
	}
	plain, err := DecodeRoutingPolicy(securityPolicy(`{"tag":"egress","protocol":"trojan","settings":{"address":"203.0.113.1","port":443,"password":"flat-secret"},"streamSettings":{"realitySettings":{"password":"transport-secret"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := EncryptPolicySecrets(plain, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(enc)
	for _, secret := range []string{"flat-secret", "transport-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("plaintext credential persisted")
		}
	}
	resaved, err := EncryptPolicySecrets(plain, &enc)
	if err != nil {
		t.Fatal(err)
	}
	resavedJSON, _ := json.Marshal(resaved)
	if string(raw) != string(resavedJSON) {
		t.Fatal("same plaintext changed the ciphertext")
	}
	decoded, err := DecryptPolicySecrets(enc)
	if err != nil {
		t.Fatal(err)
	}
	decodedJSON, _ := json.Marshal(decoded)
	for _, secret := range []string{"flat-secret", "transport-secret"} {
		if !strings.Contains(string(decodedJSON), secret) {
			t.Fatal("credential did not survive decryption")
		}
	}
	masked := MaskPolicyJSON(strings.ReplaceAll(securityPolicy(`{"tag":"egress","protocol":"trojan","settings":{"Password":"old-secret"}}`), "egress", "legacy"))
	if strings.Contains(masked, "old-secret") {
		t.Fatal("invalid historical JSON leaked a noncanonical secret")
	}
}

func TestIndependentObservatoryRetainsOutboundChain(t *testing.T) {
	policy := `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[{"tag":"probe-egress","protocol":"socks","settings":{"servers":[{"address":"203.0.113.1","port":1080}]},"streamSettings":{"sockopt":{"dialerProxy":"probe-hop"}}},{"tag":"probe-hop","protocol":"freedom"},{"tag":"unused","protocol":"freedom"}],"observatory":{"subjectSelector":["probe-egress"],"probeURL":"https://1.1.1.1/","probeInterval":"1m"}}`
	p, err := DecodeRoutingPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := ApplyRoutingPolicy(`{"inbounds":[],"outbounds":[],"routing":{"rules":[]}}`, p)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := json.Unmarshal([]byte(rendered), &cfg); err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, out := range cfg.Outbounds {
		tags[out.Tag] = true
	}
	if !tags["probe-egress"] || !tags["probe-hop"] || tags["unused"] {
		t.Fatalf("incorrect observatory reference closure: %v", tags)
	}
	p.Balancers = []Balancer{{Tag: "idle-pool", Selector: []string{"unused"}}}
	rendered, err = ApplyRoutingPolicy(`{"inbounds":[],"outbounds":[],"routing":{"rules":[]}}`, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, `"observatory"`) || !strings.Contains(rendered, `"tag": "probe-egress"`) || strings.Contains(rendered, `"tag": "unused"`) {
		t.Fatal("unused balancer suppressed an independent observatory")
	}
}

func TestBuildHistoricalOverridesRefusesUnequalPolicy(t *testing.T) {
	base := `{"schema_version":3,"ipv6_failure_policy":"strict","domain_strategy":"AsIs"}`
	target := strings.ReplaceAll(base, "AsIs", "IPOnDemand")
	if _, err := BuildOverridesForTarget(base, target); err == nil {
		t.Fatal("reconstructed unsupported strategy difference silently")
	}
}
