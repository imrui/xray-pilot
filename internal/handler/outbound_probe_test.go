package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func TestOutboundProbeEndpointPermissionsAndInput(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	doJSON(t, r, http.MethodPost, "/api/admins", root, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	ops := login(t, r, "ops", "opspass")
	node := entity.Node{Name: "los1", IP: "192.0.2.1"}
	if err := repository.NewNodeRepository().Create(&node); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/nodes/%d/outbound-test", node.ID)
	const missing = "/api/nodes/999999/outbound-test"
	for _, tc := range []struct {
		name, path, token string
		body              map[string]string
		status            int
	}{
		{"authentication required", path, "", map[string]string{"tag": "los2"}, http.StatusUnauthorized},
		{"unknown outbound", path, ops, map[string]string{"tag": "los2"}, http.StatusBadRequest},
		{"custom target", missing, ops, map[string]string{"tag": "los2", "probe_url": "https://example.org"}, http.StatusBadRequest},
		{"custom configuration", missing, root, map[string]string{"tag": "los2", "config": "{}"}, http.StatusBadRequest},
		{"unknown node", missing, ops, map[string]string{"tag": "los2"}, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := doJSON(t, r, http.MethodPost, tc.path, tc.token, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("http %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
