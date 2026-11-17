package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
)

func TestNodeProtocolPortAPIOnlyInvalidatesTarget(t *testing.T) {
	r := setupHandlerTestEnv(t)
	token := login(t, r, "root", "rootpass")
	profile := entity.InboundProfile{Name: "Reality", Protocol: types.ProtocolVlessReality, Port: 443, Active: true, Settings: `{"sni":"www.apple.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"]}`}
	if err := repository.DB.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	nodes := []entity.Node{{Name: "tw02", Active: true, SyncStatus: entity.SyncStatusSynced}, {Name: "other", Active: true, SyncStatus: entity.SyncStatusSynced}, {Name: "failed", Active: true, SyncStatus: entity.SyncStatusFailed}}
	for i := range nodes {
		if err := repository.NewNodeRepository().Create(&nodes[i]); err != nil {
			t.Fatal(err)
		}
		if err := repository.DB.Create(&entity.NodeProfileKey{NodeID: nodes[i].ID, ProfileID: profile.ID, Settings: "{}"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	path := fmt.Sprintf("/api/nodes/%d/keys/%d", nodes[0].ID, profile.ID)
	w, resp := doJSON(t, r, http.MethodPut, path, token, map[string]any{"port": 24443, "settings": map[string]any{}})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("save port: %s", w.Body.String())
	}
	if resp.Data.(map[string]any)["port"] != float64(24443) {
		t.Fatal("override not saved")
	}
	w, resp = doJSON(t, r, http.MethodGet, "/api/nodes?page_size=100", token, nil)
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("node list: %s", w.Body.String())
	}
	list := resp.Data.(map[string]any)["list"].([]any)
	if len(list) != 3 {
		t.Fatal("missing nodes")
	}
	for _, row := range list {
		n := row.(map[string]any)
		want := entity.SyncStatusSynced
		if n["name"] == "tw02" {
			want = entity.SyncStatusDrifted
		}
		if n["name"] == "failed" {
			want = entity.SyncStatusFailed
		}
		if n["sync_status"] != string(want) {
			t.Errorf("%s status changed incorrectly: %v", n["name"], n["sync_status"])
		}
	}
}
