package service

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func TestVisionSubscriptionPolicyKeepsServerFlow(t *testing.T) {
	n, u := routingFixture(t)
	s := NewRoutingService()
	sync := NewSyncService()
	before, err := sync.DesiredConfigHash(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"block", "allow"} {
		b, _ := s.Binding("node", n.ID)
		b.UDP443Policy = policy
		if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
			t.Fatal(err)
		}
		raw, err := NewSubscribeService().GenerateSubscription(u.Token)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		flow := "xtls-rprx-vision"
		if policy == "allow" {
			flow += "-udp443"
		}
		if !strings.Contains(string(decoded), "flow="+flow+"&") {
			t.Fatalf("unexpected subscription flow for %s", policy)
		}
		config, _, err := sync.PreviewConfig(n.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(config, `"flow": "xtls-rprx-vision"`) || strings.Contains(config, "xtls-rprx-vision-udp443") {
			t.Fatal("client flow suffix leaked into server configuration")
		}
		after, err := sync.DesiredConfigHash(n.ID)
		if err != nil || before != after {
			t.Fatalf("client policy changed server config hash: %v", err)
		}
	}
}

func TestTemplatePreviewUsesProposedSourceWithoutWrites(t *testing.T) {
	n, _ := routingFixture(t)
	s := NewRoutingService()
	policy := strings.Replace(DefaultRoutingPolicy, `"ipv4_force_domains":[]`, `"ipv4_force_domains":["domain:published.example"]`, 1)
	rev := publishRouting(t, "preview", policy)
	var template entity.RoutingTemplate
	repository.DB.First(&template, rev.TemplateID)
	s.SaveTemplate(template.ID, template.Version, template.Name, "", strings.Replace(policy, "published.example", "draft.example", 1), "test")
	b, _ := s.Binding("node", n.ID)
	b.Mode, b.TemplateID, b.Revision = "template", rev.TemplateID, rev.Revision
	b.Overrides = `{"ipv4_force_domains":[],"ipv6_force_domains":["domain:node.example"]}`
	if err := s.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}

	// Previewing must not alter persisted bindings, sync state, logs or jobs.
	snapshot := func() string {
		state := map[string][]map[string]any{}
		for _, table := range []string{"routing_bindings", "node_config_states", "nodes", "sync_logs", "config_deployments"} {
			var rows []map[string]any
			if err := repository.DB.Table(table).Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			state[table] = rows
		}
		data, _ := json.Marshal(state)
		return string(data)
	}
	before := snapshot()
	preview, err := s.PreviewTemplate(n.ID, "template", rev.TemplateID, rev.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview.Policy.IPv4Domains, []string{"domain:published.example"}) || len(preview.Policy.IPv6Domains) != 0 {
		t.Fatalf("preview mixed draft or node overrides: %+v", preview.Policy)
	}
	global, _ := s.Binding("global", 0)
	preview, err = s.PreviewTemplate(n.ID, "inherit", 0, 0)
	if err != nil || preview.TemplateID != global.TemplateID {
		t.Fatalf("proposed inheritance ignored: %+v %v", preview, err)
	}
	if snapshot() != before {
		t.Fatal("preview wrote persisted state")
	}

	var g entity.Group
	repository.DB.First(&g)
	gb := entity.RoutingBinding{Scope: "group", TargetID: g.ID, Mode: "template", TemplateID: rev.TemplateID, Revision: rev.Revision}
	if err := s.SaveBinding(gb, 0, "test", true); err != nil {
		t.Fatal(err)
	}
	preview, err = s.PreviewTemplate(n.ID, "inherit", 0, 0)
	if err != nil || preview.TemplateID != rev.TemplateID || len(preview.Groups) != 1 {
		t.Fatalf("group inheritance: %+v %v", preview, err)
	}
	other := entity.Group{Name: "conflict", Active: true, Nodes: []entity.Node{*n}}
	repository.DB.Create(&other)
	gb.TargetID, gb.TemplateID, gb.Revision = other.ID, global.TemplateID, global.Revision
	if err := s.SaveBinding(gb, 0, "test", true); err != nil {
		t.Fatal(err)
	}
	before = snapshot()
	if _, err := s.PreviewTemplate(n.ID, "inherit", 0, 0); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("conflict hidden: %v", err)
	}
	if _, err := s.PreviewTemplate(n.ID, "template", rev.TemplateID, rev.Revision); err != nil {
		t.Fatal(err)
	}
	for _, b := range []entity.RoutingBinding{{Mode: "legacy"}, {Mode: "inherit", TemplateID: 1}, {Mode: "template"}, {Mode: "template", TemplateID: rev.TemplateID, Revision: 999}} {
		if _, err := s.PreviewTemplate(n.ID, b.Mode, b.TemplateID, b.Revision); err == nil {
			t.Fatalf("invalid source accepted: %+v", b)
		}
	}
	if _, err := s.PreviewTemplate(99999, "inherit", 0, 0); err == nil {
		t.Fatal("missing node accepted")
	}
	if snapshot() != before {
		t.Fatal("failed preview wrote persisted state")
	}
}
