package service

import (
	"encoding/json"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"reflect"
	"testing"
)

func TestRoutingPreviewDoesNotPersistAndMatchesSave(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	b, _ := s.Binding("node", n.ID)
	b.Overrides = `{"ipv4_force_domains":["domain:preview.example"]}`
	var beforeNode entity.Node
	repository.DB.First(&beforeNode, n.ID)
	var beforeStates []entity.NodeConfigState
	repository.DB.Find(&beforeStates)
	var logsBefore int64
	repository.DB.Model(&entity.SyncLog{}).Count(&logsBefore)
	oldBinding, _ := s.Binding("node", n.ID)
	impact, err := s.PreviewBinding(b, true)
	if err != nil || !reflect.DeepEqual(impact.Changed, []uint{n.ID}) {
		t.Fatalf("preview: %+v %v", impact, err)
	}
	afterBinding, _ := s.Binding("node", n.ID)
	var afterNode entity.Node
	repository.DB.First(&afterNode, n.ID)
	var afterStates []entity.NodeConfigState
	repository.DB.Find(&afterStates)
	var logsAfter int64
	repository.DB.Model(&entity.SyncLog{}).Count(&logsAfter)
	if !reflect.DeepEqual(oldBinding, afterBinding) || !reflect.DeepEqual(beforeNode, afterNode) || !reflect.DeepEqual(beforeStates, afterStates) || logsAfter != logsBefore {
		t.Fatal("preview persisted changes")
	}
	var saved BindingImpact
	if err := s.SaveBinding(b, b.Version, "test", true, &saved); err != nil || !reflect.DeepEqual(saved, impact) {
		t.Fatalf("save differs from preview: %+v %v", saved, err)
	}
	if _, err := s.PreviewBinding(b, true); err != ErrRoutingConflict {
		t.Fatalf("stale preview bypassed version check: %v", err)
	}
}

func TestRoutingOverviewCountsEffectiveInheritance(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	global, _ := s.Binding("global", 0)
	legacy := entity.Node{Name: "legacy"}
	repository.DB.Create(&legacy)
	r := publishRouting(t, "another", DefaultRoutingPolicy)
	b, _ := s.Binding("node", n.ID)
	b.Mode, b.TemplateID, b.Revision = "template", r.TemplateID, r.Revision
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	out, err := s.Overview()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range out.References {
		if ref.TemplateID == r.TemplateID && ref.TargetID == n.ID && ref.TargetName == n.Name {
			found = true
		}
	}
	if !found {
		t.Fatal("direct reference missing its node name")
	}
	for _, row := range out.Templates {
		if row.ID == global.TemplateID && row.NodeCount != 0 {
			t.Fatal("shadowed default counted as effective")
		}
		if row.ID == r.TemplateID && (row.NodeCount != 1 || row.BindingCount != 1 || row.DraftChanged) {
			t.Fatalf("wrong use count: %+v", row)
		}
	}
	for _, row := range out.Nodes {
		if row.ID == legacy.ID && row.Routing.Mode != "legacy" {
			t.Fatal("legacy silently inherited")
		}
	}
	data, _ := json.Marshal(out)
	var roundtrip RoutingOverview
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingPreviewEnforcesPermissionsAndOverrides(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	global, _ := s.Binding("global", 0)
	if _, err := s.PreviewBinding(global, false); err == nil {
		t.Fatal("ordinary admin preview bypassed shared permissions")
	}
	b, _ := s.Binding("node", n.ID)
	b.Overrides = `{"custom_rules":[{"type":"field","ip":["203.0.113.0/24"],"outboundTag":"direct-v6"}]}`
	if _, err := s.PreviewBinding(b, true); err == nil {
		t.Fatal("IPv6-disabled target accepted v6 rule")
	}
}

func TestOrdinaryAdminCanPreserveFormattedAdvancedRules(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	b, _ := s.Binding("node", n.ID)
	b.Overrides = `{"custom_rules": [ { "type": "field", "ip": ["203.0.113.0/24"], "outboundTag": "direct-v4" } ]}`
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Binding("node", n.ID)
	b.Overrides = `{"ipv4_force_domains":["domain:example.com"],"custom_rules":[{"outboundTag":"direct-v4","ip":["203.0.113.0/24"],"type":"field"}]}`
	if err := s.SaveBinding(b, b.Version, "ops", false); err != nil {
		t.Fatalf("format-only change rejected: %v", err)
	}
	b, _ = s.Binding("node", n.ID)
	b.Overrides = `{"custom_rules":[{"outboundTag":"block","ip":["203.0.113.0/24"],"type":"field"}]}`
	if err := s.SaveBinding(b, b.Version, "ops", false); err == nil {
		t.Fatal("ordinary admin changed advanced rule")
	}
}
