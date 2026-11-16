package service

import (
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

func TestPresetSelectionAndDefaultWithNodeOverrides(t *testing.T) {
	n, _ := routingFixture(t)
	svc := NewRoutingService()
	overview, err := svc.Overview()
	if err != nil || len(overview.Presets) != 2 {
		t.Fatalf("reference catalog unavailable: %v", err)
	}
	var normal, media entity.RoutingTemplate
	repository.DB.First(&normal, "preset_key = ?", entity.RoutingPresetDefault)
	repository.DB.First(&media, "preset_key = ?", entity.RoutingPresetAIMedia)
	p, err := xray.DecodeRoutingPolicy(media.Draft)
	if err != nil || !p.RequiresIPv6() || len(p.IPv4Domains) != 8 || len(p.IPv6Domains) != 3 || strings.Contains(media.Draft, `"domain:google.com"`) {
		t.Fatalf("unexpected AI/media preset: %+v %v", p, err)
	}
	b, _ := svc.Binding("node", n.ID)
	b.Mode, b.TemplateID, b.Revision = "template", media.ID, 1
	if err := svc.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	raw, _, err := NewSyncService().PreviewConfig(n.ID)
	if err != nil || !strings.Contains(raw, "ForceIPv6") || !strings.Contains(raw, "ForceIPv4") {
		t.Fatalf("preset not rendered into complete node config: %v", err)
	}
	b, _ = svc.Binding("node", n.ID)
	b.TemplateID = normal.ID
	b.Overrides = `{"ipv6_force_domains":["domain:explicit.example"]}`
	if err := svc.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	effective, _ := svc.Effective(n.ID)
	if !effective.Policy.RequiresIPv6() {
		t.Fatal("selecting default silently cleared existing node overrides")
	}
	b, _ = svc.Binding("node", n.ID)
	b.Overrides = "{}"
	if err := svc.SaveBinding(b, b.Version, "test", true); err != nil {
		t.Fatal(err)
	}
	raw, _, err = NewSyncService().PreviewConfig(n.ID)
	if err != nil || strings.Contains(raw, "direct-v4") || strings.Contains(raw, "direct-v6") {
		t.Fatal("empty default with cleared overrides did not return to normal direct")
	}
	// Editing a preset uses the ordinary draft/revision flow and retains identity.
	saved, err := svc.SaveTemplate(media.ID, media.Version, "renamed AI preset", "custom", media.Draft, "test")
	if err != nil || saved.PresetKey == nil || *saved.PresetKey != entity.RoutingPresetAIMedia {
		t.Fatalf("preset identity lost during ordinary editing: %+v %v", saved, err)
	}
}
