package handler

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// Exercise the UI's group PUT -> node-list GET flow, including omitted versus
// empty node_ids. No worker or SSH connection is started in this SQLite test.
func TestGroupMembershipAPIOnlyMarksChangedNode(t *testing.T) {
	r := setupHandlerTestEnv(t)
	token := login(t, r, "root", "rootpass")
	nodes := []entity.Node{
		{Name: "vs0", IP: "192.0.2.1", SyncStatus: entity.SyncStatusSynced},
		{Name: "vs1", IP: "192.0.2.2", SyncStatus: entity.SyncStatusSynced},
		{Name: "unrelated", IP: "192.0.2.3", SyncStatus: entity.SyncStatusFailed},
	}
	for i := range nodes {
		if err := repository.NewNodeRepository().Create(&nodes[i]); err != nil {
			t.Fatal(err)
		}
	}
	w, resp := doJSON(t, r, http.MethodPost, "/api/groups", token, map[string]any{"name": "VS", "node_ids": []uint{nodes[0].ID, nodes[1].ID}})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create group: %s", w.Body.String())
	}
	id := uint(resp.Data.(map[string]any)["id"].(float64))
	path := fmt.Sprintf("/api/groups/%d", id)
	for _, tc := range []struct {
		body    map[string]any
		count   int
		changed []uint
	}{
		{map[string]any{"name": "renamed"}, 2, nil},
		{map[string]any{"node_ids": []uint{nodes[0].ID}}, 1, []uint{nodes[1].ID}},
		{map[string]any{"node_ids": []uint{nodes[0].ID, nodes[1].ID}}, 2, []uint{nodes[1].ID}},
		{map[string]any{"node_ids": []uint{}}, 0, []uint{nodes[0].ID, nodes[1].ID}},
		{map[string]any{"node_ids": []uint{}}, 0, nil},
	} {
		for _, n := range nodes {
			if err := repository.DB.Model(&n).Update("sync_status", n.SyncStatus).Error; err != nil {
				t.Fatal(err)
			}
		}
		w, resp = doJSON(t, r, http.MethodPut, path, token, tc.body)
		if w.Code != http.StatusOK || resp.Code != 0 {
			t.Fatalf("save group: %s", w.Body.String())
		}
		if int(resp.Data.(map[string]any)["node_count"].(float64)) != tc.count {
			t.Fatal("response membership does not match save")
		}
		w, resp = doJSON(t, r, http.MethodGet, "/api/nodes?page_size=100", token, nil)
		if w.Code != http.StatusOK || resp.Code != 0 {
			t.Fatalf("list nodes: %s", w.Body.String())
		}
		expected := map[uint]string{}
		for _, n := range nodes {
			expected[n.ID] = string(n.SyncStatus)
		}
		for _, id := range tc.changed {
			expected[id] = string(entity.SyncStatusDrifted)
		}
		list := resp.Data.(map[string]any)["list"].([]any)
		if len(list) != len(nodes) {
			t.Fatal("missing nodes")
		}
		for _, item := range list {
			n := item.(map[string]any)
			id := uint(n["id"].(float64))
			if n["sync_status"] != expected[id] {
				t.Errorf("node %d: got %v, want %s", id, n["sync_status"], expected[id])
			}
		}
	}
}
