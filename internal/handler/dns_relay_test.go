package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
)

type unavailableDNSUpstream struct{}

func (unavailableDNSUpstream) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("simulated upstream outage")
}

func TestDNSErrorHTTPStatus(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	check := func(method, path string, body any, status int) {
		t.Helper()
		w, resp := doJSON(t, r, method, path, root, body)
		if w.Code != status || resp.Code != status || resp.Message == "" {
			t.Fatalf("%s %s: HTTP %d, response %+v", method, path, w.Code, resp)
		}
	}
	check("GET", "/api/nodes/invalid/dns", nil, http.StatusBadRequest)
	check("PUT", "/api/system/dns", "invalid", http.StatusBadRequest)
	check("POST", "/api/system/dns/test", nil, http.StatusBadRequest)
	cfg := service.DNSSettingsInput{Enabled: true, Provider: "cloudflare", ZoneID: strings.Repeat("a", 32), Token: "fixture-token"}
	if err := service.NewDNSService().SaveSettings(cfg, false, "admin:test"); err != nil {
		t.Fatal(err)
	}
	check("GET", "/api/nodes/999/dns/preview", nil, http.StatusNotFound)
	transport := http.DefaultTransport
	http.DefaultTransport = unavailableDNSUpstream{}
	defer func() { http.DefaultTransport = transport }()
	check("POST", "/api/system/dns/test", nil, http.StatusBadGateway)
	if err := repository.DB.Migrator().DropTable(&entity.DNSBinding{}); err != nil {
		t.Fatal(err)
	}
	check("GET", "/api/dns/bindings", nil, http.StatusInternalServerError)
}

func TestDNSCredentialsAndRoleBoundary(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	_, resp := doJSON(t, r, "POST", "/api/admins", root, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	if resp.Code != 0 {
		t.Fatal(resp.Message)
	}
	ops := login(t, r, "ops", "opspass")
	zone := strings.Repeat("a", 32)
	secret := "fake-dns-secret"
	input := map[string]any{"enabled": true, "provider": "cloudflare", "zone_id": zone, "token": secret}
	_, resp = doJSON(t, r, "PUT", "/api/system/dns", root, input)
	if resp.Code != 0 {
		t.Fatal(resp.Message)
	}
	cfg, err := repository.DNSConfig(repository.DB)
	if err != nil || cfg.TokenCipher == "" || cfg.TokenCipher == secret {
		t.Fatal("token not encrypted")
	}
	for _, token := range []string{"", secret} {
		input["token"] = token
		_, resp = doJSON(t, r, "PUT", "/api/system/dns", root, input)
		if resp.Code != 0 {
			t.Fatal(resp.Message)
		}
		fresh, _ := repository.DNSConfig(repository.DB)
		if cfg.TokenCipher != fresh.TokenCipher {
			t.Fatal("unchanged token rewritten")
		}
	}
	for _, path := range []string{"/api/system/settings", "/api/system/dns"} {
		w, resp := doJSON(t, r, "GET", path, root, nil)
		if resp.Code != 0 || strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), cfg.TokenCipher) {
			t.Fatal("settings exposed token", path)
		}
	}
	for _, route := range [][2]string{{"PUT", "/api/system/dns"}, {"DELETE", "/api/system/dns/token"}, {"GET", "/api/nodes/1/dns/preview"}, {"POST", "/api/nodes/1/dns/retry"}, {"DELETE", "/api/nodes/1/dns"}} {
		w, _ := doJSON(t, r, route[0], route[1], ops, nil)
		if w.Code != http.StatusForbidden {
			t.Fatal(route, w.Code)
		}
	}
	_, resp = doJSON(t, r, "GET", "/api/dns/bindings", ops, nil)
	if resp.Code != 0 {
		t.Fatal("ops cannot read DNS bindings")
	}
	_, resp = doJSON(t, r, "DELETE", "/api/system/dns/token", root, nil)
	if resp.Code != 0 {
		t.Fatal(resp.Message)
	}
	cfg, _ = repository.DNSConfig(repository.DB)
	if cfg.Enabled || cfg.TokenCipher != "" {
		t.Fatal("token not cleared")
	}
}

func TestRelayHTMLAndMachineSubscriptions(t *testing.T) {
	r := setupHandlerTestEnv(t)
	id := "u002abc"
	first := "http://192.168.0.2:2096/sub/"
	u := entity.User{Username: "office", Token: "office-page-token", UUID: "office-uuid", RelaySubscriptionID: &id, Active: true}
	if err := repository.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	settings := service.NewSettingService()
	if err := settings.BatchUpdate(map[string]string{service.KeyRelayEnabled: "true", service.KeyRelayPrefix: first}); err != nil {
		t.Fatal(err)
	}
	get := func(path string) string {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w.Body.String()
	}
	path := "/sub/" + u.Token
	html := get(path)
	for _, want := range []string{"内网中转订阅", fmt.Sprintf("data-copy=%q", first+id), fmt.Sprintf("data-link=%q", first+id),
		fmt.Sprintf("value=%q checked", first+id), fmt.Sprintf("value=%q", first+id+"?format=json"), fmt.Sprintf("value=%q", first+id+"?format=clash"),
		"/subscription-assets/qrcode.js", "暂无可用节点"} {
		if !strings.Contains(html, want) {
			t.Fatal("missing HTML", want)
		}
	}
	if strings.Contains(html, "fonts.googleapis.com") || strings.Contains(html, "cdn.jsdelivr.net") || !strings.Contains(get("/subscription-assets/qrcode.js"), "QRCode") {
		t.Fatal("external subscription asset")
	}
	for _, format := range []string{"v2ray", "clash", "singbox"} {
		if strings.Contains(get(path+"?format="+format), id) {
			t.Fatal("relay ID exposed in machine subscription")
		}
	}
	if err := settings.BatchUpdate(map[string]string{service.KeyRelayEnabled: "false"}); err != nil {
		t.Fatal(err)
	}
	if html = get(path); strings.Contains(html, "内网中转订阅") || strings.Contains(html, id) {
		t.Fatal("hidden relay still in HTML")
	}
	primary := "http://example.com" + path
	if strings.Contains(html, "备用订阅链接") {
		t.Fatal("duplicate subscription section")
	}
	for _, want := range []string{
		`data-username="office"`,
		`>扫码订阅</div>`, `id="mainHint"`,
		fmt.Sprintf(`<code id="mainURL" class="selected-url" tabindex="0">%s</code>`, primary),
		`id="mainImport"`, `clash://install-config?url=`, `<dl class="manual-links">`,
		fmt.Sprintf(`name="mainFormat" data-format="default" value=%q checked`, primary),
		fmt.Sprintf(`data-format="v2ray" value=%q><span>V2Ray`, primary+"?format=v2ray"),
		fmt.Sprintf(`data-format="clash" value=%q><span>Clash`, primary+"?format=clash"),
		fmt.Sprintf(`data-format="singbox" value=%q><span>sing-box`, primary+"?format=singbox"),
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing primary QR option or username: %s", want)
		}
	}
}
