package handler

import (
	"net/http"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
)

func TestXrayStableVersionSettingsAPI(t *testing.T) {
	r := setupHandlerTestEnv(t)
	rootToken := login(t, r, "root", "rootpass")
	w, resp := doJSON(t, r, http.MethodPost, "/api/admins", rootToken, dto.CreateAdminRequest{
		Username: "ops", Password: "opspass", Role: "admin",
	})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create admin: http=%d msg=%s", w.Code, resp.Message)
	}
	adminToken := login(t, r, "ops", "opspass")
	key := service.KeyXrayStableVersion
	w, resp = doJSON(t, r, http.MethodGet, "/api/system/settings", adminToken, nil)
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("get defaults: http=%d msg=%s", w.Code, resp.Message)
	}
	if data, _ := resp.Data.(map[string]any); data[key] != "26.3.27" {
		t.Fatalf("default version = %v", data[key])
	}
	w, _ = doJSON(t, r, http.MethodPut, "/api/system/settings", "", map[string]string{key: "26.4.17"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated update: http=%d", w.Code)
	}
	w, resp = doJSON(t, r, http.MethodPut, "/api/system/settings", adminToken, map[string]string{key: " v26.4.17 "})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("admin update: http=%d msg=%s", w.Code, resp.Message)
	}
	if data, _ := resp.Data.(map[string]any); data[key] != "26.4.17" {
		t.Fatalf("normalized version = %v", data[key])
	}
	w, resp = doJSON(t, r, http.MethodPut, "/api/system/settings", adminToken, map[string]string{key: "latest"})
	if w.Code != http.StatusOK || resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid version: http=%d code=%d msg=%s", w.Code, resp.Code, resp.Message)
	}
	if got := service.NewSettingService().Get(key); got != "26.4.17" {
		t.Fatalf("invalid update changed saved version: %q", got)
	}
}
