package handler

import (
	"encoding/json"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"net/http"
	"testing"
)

func TestRoutingRoutesAuthorizationAndVersionConflict(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	doJSON(t, r, http.MethodPost, "/api/admins", root, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	ops := login(t, r, "ops", "opspass")
	body := map[string]any{"name": "policy", "draft": json.RawMessage(service.DefaultRoutingPolicy)}
	if w, _ := doJSON(t, r, http.MethodPost, "/api/routing-templates", ops, body); w.Code != http.StatusForbidden {
		t.Fatalf("admin can create shared template: %d", w.Code)
	}
	if w, _ := doJSON(t, r, http.MethodGet, "/api/routing-templates", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth: %d", w.Code)
	}
	w, response := doJSON(t, r, http.MethodPost, "/api/routing-templates", root, body)
	if w.Code != http.StatusOK || response.Code != 0 {
		t.Fatalf("create %d %s", w.Code, response.Message)
	}
	n := entity.Node{Name: "test"}
	repository.NewNodeRepository().Create(&n)
	binding, _ := service.NewRoutingService().Binding("node", n.ID)
	binding.Overrides = `{"custom_rules":[{"type":"field","port":"443","outboundTag":"block"}]}`
	w, response = doJSON(t, r, http.MethodPut, "/api/nodes/1/routing", ops, binding)
	if response.Code == 0 {
		t.Fatalf("advanced override bypass: %d", w.Code)
	}
	binding.Overrides = "{}"
	w, response = doJSON(t, r, http.MethodPut, "/api/nodes/1/routing", root, binding)
	if response.Code != 0 {
		t.Fatal(response.Message)
	}
	w, response = doJSON(t, r, http.MethodPut, "/api/nodes/1/routing", root, binding)
	if response.Code != 409 {
		t.Fatalf("missing optimistic conflict: %d/%d", w.Code, response.Code)
	}
	w, response = doJSON(t, r, http.MethodPut, "/api/system/settings", ops, map[string]string{service.KeyXrayProbeURL: "https://example.com/health"})
	if response.Code != 403 {
		t.Fatalf("probe target permission bypass: %d", w.Code)
	}
	w, response = doJSON(t, r, http.MethodGet, "/api/nodes/not-an-id/effective-config", root, nil)
	if response.Code != 400 {
		t.Fatalf("invalid id accepted: %d", w.Code)
	}
}
