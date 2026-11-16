package handler

import (
	"encoding/json"
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"net/http"
	"testing"
)

func TestRoutingUIOverviewFiltersAndHistory(t *testing.T) {
	r := setupHandlerTestEnv(t)
	token := login(t, r, "root", "rootpass")
	for i := 1; i <= 3; i++ {
		n := entity.Node{Name: fmt.Sprintf("n%d", i), Active: true, SyncStatus: entity.SyncStatusSynced}
		if i != 1 {
			n.SyncStatus = entity.SyncStatusPending
		}
		if err := repository.NewNodeRepository().Create(&n); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 105; i++ {
		repository.DB.Create(&entity.ConfigDeployment{NodeID: 1, RequestKey: fmt.Sprintf("test-%d", i), Status: "succeeded"})
	}
	repository.DB.Create(&entity.ConfigDeployment{NodeID: 2, RequestKey: "other", Status: "unknown"})
	repository.DB.Create(&entity.SyncLog{Action: "sync", Target: "node:n1(1)", Success: true})
	repository.DB.Create(&entity.SyncLog{Action: "sync", Target: "node:n11(11)", Success: false})
	cases := []struct {
		path  string
		total int
	}{
		{"/api/nodes?ids=2,3&sync_status=needs_sync", 2},
		{"/api/nodes?ids=1&sync_status=needs_sync", 0},
		{"/api/config-deployments?page=6&page_size=20&node_id=1", 105},
		{"/api/node-sync-logs?page=1&node_id=1", 1},
	}
	for _, tc := range cases {
		w, res := doJSON(t, r, http.MethodGet, tc.path, token, nil)
		if res.Code != 0 {
			t.Fatalf("%s: %s", tc.path, res.Message)
		}
		var body struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Data.Total != tc.total {
			t.Fatalf("%s: total %d want %d (%v)", tc.path, body.Data.Total, tc.total, err)
		}
	}
	for _, path := range []string{"/api/nodes?ids=bad", "/api/config-deployments?page=1&node_id=-1", "/api/node-sync-logs?node_id=0"} {
		_, res := doJSON(t, r, http.MethodGet, path, token, nil)
		if res.Code == 0 {
			t.Fatalf("invalid filter accepted: %s", path)
		}
	}
	_, res := doJSON(t, r, http.MethodGet, "/api/routing-overview", token, nil)
	if res.Code != 0 {
		t.Fatal(res.Message)
	}
	w, res := doJSON(t, r, http.MethodGet, "/api/config-deployments", token, nil)
	if res.Code != 0 {
		t.Fatal(res.Message)
	}
	var old struct {
		Data []entity.ConfigDeployment `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &old)
	if len(old.Data) != 100 {
		t.Fatalf("legacy response broken: %d", len(old.Data))
	}
}
