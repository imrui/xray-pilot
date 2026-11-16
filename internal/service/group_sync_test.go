package service

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type groupSyncSnapshot struct {
	status entity.SyncStatus
	state  entity.NodeConfigState
	config string
}

func groupSyncFixture(t *testing.T) ([]entity.Node, uint) {
	t.Helper()
	first, _ := routingFixture(t)
	nodes := []entity.Node{*first}
	var key entity.NodeProfileKey
	if err := repository.DB.First(&key).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vs1", "unrelated"} {
		n := entity.Node{Name: name, IP: "192.0.2.2", Active: true, SyncStatus: entity.SyncStatusSynced}
		if err := repository.NewNodeRepository().Create(&n); err != nil {
			t.Fatal(err)
		}
		if err := repository.DB.Create(&entity.NodeProfileKey{NodeID: n.ID, ProfileID: key.ProfileID, Settings: "{}"}).Error; err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, n)
	}
	var common entity.Group
	if err := repository.DB.First(&common).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Model(&common).Association("Nodes").Replace(nodes); err != nil {
		t.Fatal(err)
	}
	vs := entity.Group{Name: "VS", Active: true, Nodes: nodes[:2]}
	if err := repository.DB.Create(&vs).Error; err != nil {
		t.Fatal(err)
	}
	u := entity.User{Username: "vs-user", UUID: "22222222-2222-4222-8222-222222222222", Token: "vs-token", Active: true, Groups: []entity.Group{vs}}
	if err := repository.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return nodes, vs.ID
}

func captureGroupSync(t *testing.T, nodes []entity.Node, seed bool) map[uint]groupSyncSnapshot {
	t.Helper()
	result := map[uint]groupSyncSnapshot{}
	for _, n := range nodes {
		if seed {
			s := entity.NodeConfigState{NodeID: n.ID, ChangeRevision: 7, DesiredHash: "desired", DiskHash: "disk", RuntimeHash: "runtime", RecoveryAttempts: 2, LastError: "previous diagnostic"}
			if err := repository.DB.Save(&s).Error; err != nil {
				t.Fatal(err)
			}
			if err := repository.DB.Model(&n).Update("sync_status", entity.SyncStatusSynced).Error; err != nil {
				t.Fatal(err)
			}
		}
		var s entity.NodeConfigState
		if err := repository.DB.First(&s, "node_id = ?", n.ID).Error; err != nil {
			t.Fatal(err)
		}
		fresh, err := repository.NewNodeRepository().FindByID(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _, err := NewSyncService().buildConfig(fresh)
		if err != nil {
			t.Fatal(err)
		}
		result[n.ID] = groupSyncSnapshot{status: fresh.SyncStatus, state: s, config: raw}
	}
	return result
}

func assertGroupSyncImpact(t *testing.T, nodes []entity.Node, before map[uint]groupSyncSnapshot, changed ...uint) {
	t.Helper()
	after := captureGroupSync(t, nodes, false)
	expected := map[uint]bool{}
	for _, id := range changed {
		expected[id] = true
	}
	for _, n := range nodes {
		a, b := after[n.ID], before[n.ID]
		if !expected[n.ID] {
			if !reflect.DeepEqual(a, b) {
				t.Errorf("unaffected node %d changed config or synchronization state", n.ID)
			}
			continue
		}
		if a.status != entity.SyncStatusDrifted || a.state.ChangeRevision != b.state.ChangeRevision+1 || !a.state.RecoveryPending || a.state.NextRetryAt == nil || a.state.RecoveryAttempts != 0 {
			t.Errorf("node %d must be invalidated exactly once: %+v", n.ID, a.state)
		}
		if a.state.DiskHash != b.state.DiskHash || a.state.RuntimeHash != b.state.RuntimeHash {
			t.Errorf("node %d lost applied state before synchronization", n.ID)
		}
		if a.config == b.config {
			t.Errorf("expected a real user/routing change on node %d", n.ID)
		}
	}
}

func TestGroupMembershipSyncScope(t *testing.T) {
	nodes, id := groupSyncFixture(t)
	svc := NewGroupService()
	for _, add := range []bool{false, true} {
		before := captureGroupSync(t, nodes, true)
		ids := []uint{nodes[0].ID}
		if add {
			ids = append(ids, nodes[1].ID)
		}
		resp, err := svc.UpdateGroup(id, &dto.UpdateGroupRequest{NodeIDs: ids})
		if err != nil || resp.NodeCount != len(ids) {
			t.Fatalf("membership response: %+v %v", resp, err)
		}
		assertGroupSyncImpact(t, nodes, before, nodes[1].ID)
		raw := captureGroupSync(t, nodes, false)[nodes[1].ID].config
		if strings.Contains(raw, "22222222-2222-4222-8222-222222222222") != add {
			t.Fatal("rendered users do not match group membership")
		}
	}
}

func TestGroupChangesSyncScope(t *testing.T) {
	for _, action := range []string{"metadata", "same-members", "disable", "enable", "disable-and-remove", "inactive-members", "delete", "delete-inactive", "empty-delete", "empty-save"} {
		t.Run(action, func(t *testing.T) {
			nodes, id := groupSyncFixture(t)
			svc := NewGroupService()
			if action == "enable" || action == "inactive-members" || action == "delete-inactive" {
				if err := svc.ToggleActive(id); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(action, "empty-") {
				if _, err := svc.UpdateGroup(id, &dto.UpdateGroupRequest{NodeIDs: []uint{}}); err != nil {
					t.Fatal(err)
				}
			}
			before := captureGroupSync(t, nodes, true)
			var err error
			var changed []uint
			switch action {
			case "metadata":
				name, description := "VS renamed", "new description"
				_, err = svc.UpdateGroup(id, &dto.UpdateGroupRequest{Name: &name, Description: &description})
			case "same-members":
				_, err = svc.UpdateGroup(id, &dto.UpdateGroupRequest{NodeIDs: []uint{nodes[1].ID, nodes[0].ID, nodes[1].ID}})
			case "disable", "enable":
				err = svc.ToggleActive(id)
				changed = []uint{nodes[0].ID, nodes[1].ID}
			case "disable-and-remove":
				active := false
				_, err = svc.UpdateGroup(id, &dto.UpdateGroupRequest{Active: &active, NodeIDs: []uint{nodes[0].ID}})
				changed = []uint{nodes[0].ID, nodes[1].ID}
			case "inactive-members":
				_, err = svc.UpdateGroup(id, &dto.UpdateGroupRequest{NodeIDs: []uint{nodes[0].ID}})
			case "delete", "delete-inactive", "empty-delete":
				err = svc.Delete(id)
				if action == "delete" {
					changed = []uint{nodes[0].ID, nodes[1].ID}
				}
			case "empty-save":
				_, err = svc.UpdateGroup(id, &dto.UpdateGroupRequest{NodeIDs: []uint{}})
			}
			if err != nil {
				t.Fatal(err)
			}
			assertGroupSyncImpact(t, nodes, before, changed...)
		})
	}
}

func TestGroupSaveFailureIsAtomic(t *testing.T) {
	for _, failure := range []string{"missing-node", "database-write"} {
		t.Run(failure, func(t *testing.T) {
			nodes, id := groupSyncFixture(t)
			before := captureGroupSync(t, nodes, true)
			ids := []uint{nodes[0].ID, 999999}
			if failure == "database-write" {
				ids = []uint{nodes[0].ID, nodes[2].ID}
				sql := fmt.Sprintf("CREATE TRIGGER reject_group_insert BEFORE INSERT ON group_nodes WHEN NEW.node_id = %d BEGIN SELECT RAISE(ABORT, 'injected failure'); END", nodes[2].ID)
				if err := repository.DB.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			name := "must not persist"
			if _, err := NewGroupService().UpdateGroup(id, &dto.UpdateGroupRequest{Name: &name, NodeIDs: ids}); err == nil {
				t.Fatal("expected rejected save")
			}
			g, err := repository.NewGroupRepository().FindByID(id)
			if err != nil || g.Name != "VS" || len(g.Nodes) != 2 {
				t.Fatalf("partial group save: %+v %v", g, err)
			}
			assertGroupSyncImpact(t, nodes, before)
		})
	}
}

func TestNewGroupDoesNotInvalidateNodes(t *testing.T) {
	nodes, _ := groupSyncFixture(t)
	before := captureGroupSync(t, nodes, true)
	svc := NewGroupService()
	g, err := svc.Create(&dto.CreateGroupRequest{Name: "new", NodeIDs: []uint{nodes[1].ID}})
	if err != nil || g.NodeCount != 1 {
		t.Fatalf("create: %+v %v", g, err)
	}
	if _, err := svc.Create(&dto.CreateGroupRequest{Name: "invalid", NodeIDs: []uint{999999}}); err == nil {
		t.Fatal("unknown node accepted")
	}
	var count int64
	repository.DB.Model(&entity.Group{}).Where("name = ?", "invalid").Count(&count)
	if count != 0 {
		t.Fatal("failed creation left a group")
	}
	assertGroupSyncImpact(t, nodes, before)
}

func TestGroupMembershipInvalidatesInheritedRouting(t *testing.T) {
	nodes, _ := groupSyncFixture(t)
	g := entity.Group{Name: "routing-only", Active: true, Nodes: nodes[:2]}
	if err := repository.DB.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	r := publishRouting(t, "group route", `{"schema_version":2,"ipv4_force_domains":["domain:group.example"],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[]}`)
	b := entity.RoutingBinding{Scope: "group", TargetID: g.ID, Mode: "template", TemplateID: r.TemplateID, Revision: r.Revision, Overrides: "{}"}
	if err := NewRoutingService().SaveBinding(b, 0, "test", true); err != nil {
		t.Fatal(err)
	}
	before := captureGroupSync(t, nodes, true)
	if _, err := NewGroupService().UpdateGroup(g.ID, &dto.UpdateGroupRequest{NodeIDs: []uint{nodes[0].ID}}); err != nil {
		t.Fatal(err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[1].ID)
	if strings.Contains(captureGroupSync(t, nodes, false)[nodes[1].ID].config, "domain:group.example") {
		t.Fatal("removed group route remains in generated config")
	}
}
