package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/types"
)

func TestRoutingV3HandlerMaskingAndPermissions(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	doJSON(t, r, http.MethodPost, "/api/admins", root, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	ops := login(t, r, "ops", "opspass")

	node := entity.Node{Name: "hk-node", IP: "192.0.2.50", Active: true}
	if err := repository.NewNodeRepository().Create(&node); err != nil {
		t.Fatal(err)
	}
	prof := entity.InboundProfile{Name: "Reality", Protocol: types.ProtocolVlessReality, Port: 443, Active: true, Settings: `{"sni":"www.microsoft.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"]}`}
	repository.DB.Create(&prof)
	repository.DB.Create(&entity.NodeProfileKey{NodeID: node.ID, ProfileID: prof.ID, Settings: "{}"})

	draft := json.RawMessage(`{
		"schema_version": 3,
		"domain_strategy": "IPIfNonMatch",
		"ipv4_force_domains": [],
		"ipv6_force_domains": [],
		"ipv6_failure_policy": "strict",
		"custom_outbounds": [{
			"tag": "warp-out",
			"protocol": "wireguard",
			"settings": {
				"secretKey": "top-secret-wg-key",
				"peers": [{"endpoint": "engage.cloudflareclient.com:2408", "publicKey": "pub", "preSharedKey": "top-secret-psk"}]
			}
		}],
		"custom_rules": [{"type": "field", "domain": ["geosite:openai"], "outboundTag": "warp-out"}]
	}`)

	w, res := doJSON(t, r, http.MethodPost, "/api/routing-templates", root, map[string]any{"name": "v3-tmpl", "draft": draft})
	if w.Code != http.StatusOK || res.Code != 0 {
		t.Fatalf("create v3 template failed: %d %s", w.Code, res.Message)
	}
	var created struct {
		Data entity.RoutingTemplate `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	tid := created.Data.ID

	w, res = doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/routing-templates/%d/publish", tid), root, map[string]any{"version": created.Data.Version})
	if w.Code != http.StatusOK || res.Code != 0 {
		t.Fatalf("publish v3 template failed: %d %s", w.Code, res.Message)
	}

	binding, _ := service.NewRoutingService().Binding("node", node.ID)
	binding.Mode = "template"
	binding.TemplateID = tid
	binding.Revision = 1
	binding.Overrides = `{"outbound_overrides":{"warp-out":{"mode":"replace","outbound":{"tag":"warp-out","protocol":"socks","settings":{"servers":[{"address":"203.0.113.9","port":1080,"users":[{"user":"u","pass":"top-secret-socks-pass"}]}]}}}}}`
	w, res = doJSON(t, r, http.MethodPut, fmt.Sprintf("/api/nodes/%d/routing", node.ID), root, binding)
	if w.Code != http.StatusOK || res.Code != 0 {
		t.Fatalf("root bind v3 override failed: %d %s", w.Code, res.Message)
	}

	// 普通 admin 访问所有只读与预览端点，均不得泄露明文或 enc:v1: 密文
	readChecks := []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/api/routing-overview", nil},
		{http.MethodGet, "/api/routing-templates", nil},
		{http.MethodGet, fmt.Sprintf("/api/routing-templates/%d", tid), nil},
		{http.MethodGet, fmt.Sprintf("/api/routing-templates/%d/revisions", tid), nil},
		{http.MethodGet, fmt.Sprintf("/api/routing-templates/%d/impact", tid), nil},
		{http.MethodGet, fmt.Sprintf("/api/nodes/%d/routing", node.ID), nil},
		{http.MethodPost, fmt.Sprintf("/api/nodes/%d/routing/template-preview", node.ID), map[string]any{"mode": "template", "template_id": tid, "revision": 1}},
		{http.MethodGet, fmt.Sprintf("/api/nodes/%d/effective-config", node.ID), nil},
	}
	for _, tc := range readChecks {
		respW, apiRes := doJSON(t, r, tc.method, tc.path, ops, tc.body)
		if respW.Code != http.StatusOK || apiRes.Code != 0 {
			t.Fatalf("admin %s %s failed: %d %s", tc.method, tc.path, respW.Code, apiRes.Message)
		}
		rawBody := respW.Body.String()
		for _, leaked := range []string{"top-secret-wg-key", "top-secret-psk", "top-secret-socks-pass", xray.OutboundSecretPrefix} {
			if strings.Contains(rawBody, leaked) {
				t.Fatalf("admin endpoint %s %s leaked %q:\n%s", tc.method, tc.path, leaked, rawBody)
			}
		}
		if !strings.Contains(rawBody, xray.MaskedPlaceholder) && !strings.Contains(rawBody, `\u003cmasked\u003e`) {
			t.Fatalf("admin endpoint %s %s missing <masked> placeholder:\n%s", tc.method, tc.path, rawBody)
		}
	}

	// 普通 admin 篡改 outbound_overrides 必须被拒绝
	latestBinding, _ := service.NewRoutingService().Binding("node", node.ID)
	latestBinding.Overrides = `{"outbound_overrides":{"warp-out":{"mode":"remap","target_tag":"direct"}}}`
	_, forbiddenRes := doJSON(t, r, http.MethodPut, fmt.Sprintf("/api/nodes/%d/routing", node.ID), ops, latestBinding)
	if forbiddenRes.Code == 0 {
		t.Fatal("admin unexpectedly succeeded in modifying outbound_overrides via HTTP")
	}

	// POST /api/nodes/:id/preflight-draft：仅限 super_admin；活跃租约返回 409；静态校验错误返回 200 + valid:false
	preflightPayload := map[string]any{
		"policy": json.RawMessage(`{"schema_version":3,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_outbounds":[],"custom_rules":[{"type":"field","outboundTag":"api","port":"443"}]}`),
	}
	if wOps, _ := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/nodes/%d/preflight-draft", node.ID), ops, preflightPayload); wOps.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for normal admin preflight-draft, got %d", wOps.Code)
	}
	wStatic, resStatic := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/nodes/%d/preflight-draft", node.ID), root, preflightPayload)
	if wStatic.Code != http.StatusOK || resStatic.Code != 0 {
		t.Fatalf("expected 200 static validation response, got %d %s", wStatic.Code, resStatic.Message)
	}
	var pfBody struct {
		Data service.PreflightDraftResult `json:"data"`
	}
	_ = json.Unmarshal(wStatic.Body.Bytes(), &pfBody)
	if pfBody.Data.Valid || pfBody.Data.Error == "" {
		t.Fatalf("expected valid=false with error message, got %+v", pfBody.Data)
	}

	// 持有节点配置租约时，preflight-draft 立即返回 HTTP 409
	_, release, err := repository.AcquireConfigLease(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	validPreflightPayload := map[string]any{
		"policy": draft,
	}
	wBusy, resBusy := doJSON(t, r, http.MethodPost, fmt.Sprintf("/api/nodes/%d/preflight-draft", node.ID), root, validPreflightPayload)
	release()
	if wBusy.Code != http.StatusConflict && resBusy.Code != 409 {
		t.Fatalf("expected 409 conflict when lease held, got http=%d code=%d msg=%s", wBusy.Code, resBusy.Code, resBusy.Message)
	}
}
