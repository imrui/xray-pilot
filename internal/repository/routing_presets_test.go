package repository

import (
	"reflect"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
)

func TestRoutingPresetsInitializeAndPreserveEdits(t *testing.T) {
	setupRepositoryTestDB(t)
	if err := migrateRoutingDefaults(DB); err != nil {
		t.Fatal(err)
	}
	var templates []entity.RoutingTemplate
	DB.Order("id").Find(&templates)
	if len(templates) != 2 {
		t.Fatalf("want two initial templates, got %d", len(templates))
	}
	var global entity.RoutingBinding
	DB.First(&global, "scope = ? AND target_id = ?", "global", 0)
	for _, preset := range entity.RoutingPresets() {
		var tmpl entity.RoutingTemplate
		if err := DB.First(&tmpl, "preset_key = ?", preset.Key).Error; err != nil {
			t.Fatal(err)
		}
		rev, err := findPresetRevision(DB, tmpl.ID, preset.Payload)
		if err != nil || rev.Revision != 1 || rev.SchemaVersion != 2 {
			t.Fatalf("preset not ready to select: %+v %v", rev, err)
		}
		if preset.Key == entity.RoutingPresetDefault && global.TemplateID != tmpl.ID {
			t.Fatal("fresh installation did not bind the normal direct preset")
		}
		if preset.Key == entity.RoutingPresetAIMedia {
			if global.TemplateID == tmpl.ID {
				t.Fatal("AI/media preset applied automatically")
			}
			// Renaming and editing must not reset content or recreate the preset.
			if err := DB.Model(&tmpl).Updates(map[string]any{"name": "my renamed preset", "draft": entity.DefaultRoutingPolicy, "version": 9}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := migrateRoutingDefaults(DB); err != nil {
		t.Fatal(err)
	}
	var after []entity.RoutingTemplate
	DB.Find(&after)
	var edited entity.RoutingTemplate
	DB.First(&edited, "preset_key = ?", entity.RoutingPresetAIMedia)
	var revisions int64
	DB.Model(&entity.RoutingTemplateRevision{}).Count(&revisions)
	if len(after) != 2 || revisions != 2 || edited.Name != "my renamed preset" || edited.Draft != entity.DefaultRoutingPolicy || edited.Version != 9 {
		t.Fatal("restart overwrote an edit, published another version or duplicated a preset")
	}
	var saved entity.RoutingBinding
	DB.First(&saved, "scope = ? AND target_id = ?", "global", 0)
	if !reflect.DeepEqual(saved, global) {
		t.Fatal("preset initialization changed an existing global binding")
	}
}

func TestRoutingPresetUpgradeAdoptionAndNameCollision(t *testing.T) {
	for _, oldPolicy := range []string{entity.DefaultRoutingPolicy, `{"enable_ipv6_outbound":false,"ipv4_force_domains":[],"ipv6_force_domains":[],"ipv6_failure_policy":"strict","custom_rules":[]}`} {
		t.Run(oldPolicy, func(t *testing.T) {
			setupRepositoryTestDB(t)
			// The fixture seeds modern defaults; remove only its routing rows to
			// reproduce a database from before preset identities were introduced.
			for _, model := range []any{&entity.RoutingBinding{}, &entity.RoutingTemplateRevision{}, &entity.RoutingTemplate{}} {
				if err := DB.Where("1 = 1").Delete(model).Error; err != nil {
					t.Fatal(err)
				}
			}
			old := entity.RoutingTemplate{Name: "renamed previous system template", Draft: oldPolicy, Version: 4}
			if err := DB.Create(&old).Error; err != nil {
				t.Fatal(err)
			}
			rev := entity.RoutingTemplateRevision{TemplateID: old.ID, Revision: 1, SchemaVersion: 1, Payload: oldPolicy, Actor: "system:migration"}
			if err := DB.Create(&rev).Error; err != nil {
				t.Fatal(err)
			}
			binding := entity.RoutingBinding{Scope: "global", Mode: "template", TemplateID: old.ID, Revision: 1, Version: 7, Overrides: "{}", UDP443Policy: "inherit"}
			if err := DB.Create(&binding).Error; err != nil {
				t.Fatal(err)
			}
			custom := entity.RoutingTemplate{Name: entity.RoutingPresets()[1].Name, Draft: entity.DefaultRoutingPolicy, Version: 3}
			if err := DB.Create(&custom).Error; err != nil {
				t.Fatal(err)
			}
			if err := migrateRoutingDefaults(DB); err != nil {
				t.Fatal(err)
			}
			var normal, media, oldAfter, customAfter entity.RoutingTemplate
			DB.First(&normal, "preset_key = ?", entity.RoutingPresetDefault)
			DB.First(&media, "preset_key = ?", entity.RoutingPresetAIMedia)
			DB.First(&oldAfter, old.ID)
			DB.First(&customAfter, custom.ID)
			if (normal.ID == old.ID) != (oldPolicy == entity.DefaultRoutingPolicy) {
				t.Fatal("adoption must only reuse an equivalent published normal-direct policy")
			}
			if oldAfter.Name != old.Name || oldAfter.Draft != old.Draft || oldAfter.Version != old.Version || !reflect.DeepEqual(customAfter, custom) || media.Name == custom.Name {
				t.Fatal("upgrade overwrote a renamed template or a user's same-name draft")
			}
			var saved entity.RoutingBinding
			DB.First(&saved, "scope = ? AND target_id = ?", "global", 0)
			var revAfter entity.RoutingTemplateRevision
			DB.First(&revAfter, rev.ID)
			if !reflect.DeepEqual(saved, binding) || !reflect.DeepEqual(revAfter, rev) {
				t.Fatal("upgrade changed active binding or historical revision")
			}
		})
	}
}
