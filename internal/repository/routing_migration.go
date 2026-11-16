package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"reflect"
)

func migrateRoutingDefaults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var defaultTemplate entity.RoutingTemplate
		for _, preset := range entity.RoutingPresets() {
			template, err := ensureRoutingPreset(tx, preset)
			if err != nil {
				return err
			}
			if preset.Key == entity.RoutingPresetDefault {
				defaultTemplate = template
			}
		}
		var binding entity.RoutingBinding
		err := tx.Where("scope = ? AND target_id = ?", "global", 0).First(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			rev, err := findPresetRevision(tx, defaultTemplate.ID, entity.DefaultRoutingPolicy)
			if err != nil {
				return err
			}
			binding = entity.RoutingBinding{Scope: "global", Mode: "template", TemplateID: defaultTemplate.ID, Revision: rev.Revision, Version: 1, Overrides: "{}"}
			if err := tx.Create(&binding).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		var ids []uint
		if err := tx.Model(&entity.Node{}).Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, id := range ids {
			b := entity.RoutingBinding{Scope: "node", TargetID: id, Mode: "legacy", Version: 1, Overrides: "{}"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&b).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func findPresetRevision(db *gorm.DB, id uint, payload string) (entity.RoutingTemplateRevision, error) {
	var rows []entity.RoutingTemplateRevision
	if err := db.Where("template_id = ?", id).Order("revision").Find(&rows).Error; err != nil {
		return entity.RoutingTemplateRevision{}, err
	}
	var want any
	if err := json.Unmarshal([]byte(payload), &want); err != nil {
		return entity.RoutingTemplateRevision{}, err
	}
	for _, row := range rows {
		var got any
		if json.Unmarshal([]byte(row.Payload), &got) == nil && reflect.DeepEqual(got, want) {
			return row, nil
		}
	}
	return entity.RoutingTemplateRevision{}, gorm.ErrRecordNotFound
}

func ensureRoutingPreset(db *gorm.DB, preset entity.RoutingPreset) (entity.RoutingTemplate, error) {
	var template entity.RoutingTemplate
	err := db.Where("preset_key = ?", preset.Key).First(&template).Error
	if err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return template, err
	}
	// Recognize the previous system-created default only when it already has
	// an identical published policy. Never relabel a user's same-name template.
	if preset.Key == entity.RoutingPresetDefault {
		var candidates []entity.RoutingTemplate
		if err := db.Where("preset_key IS NULL AND id IN (?)", db.Model(&entity.RoutingTemplateRevision{}).Select("template_id").Where("actor = ? AND revision = ?", "system:migration", 1)).Order("id").Find(&candidates).Error; err != nil {
			return template, err
		}
		for _, candidate := range candidates {
			if _, err := findPresetRevision(db, candidate.ID, preset.Payload); err == nil {
				candidate.PresetKey = &preset.Key
				return candidate, db.Model(&entity.RoutingTemplate{}).Where("id = ? AND preset_key IS NULL", candidate.ID).UpdateColumn("preset_key", preset.Key).Error
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return template, err
			}
		}
	}
	name := preset.Name
	for suffix := 1; ; suffix++ {
		var count int64
		if err := db.Model(&entity.RoutingTemplate{}).Where("name = ?", name).Count(&count).Error; err != nil {
			return template, err
		}
		if count == 0 {
			break
		}
		name = fmt.Sprintf("%s（预设 %d）", preset.Name, suffix)
	}
	template = entity.RoutingTemplate{PresetKey: &preset.Key, Name: name, Description: preset.Description, Draft: preset.Payload, Version: 1}
	if err := db.Create(&template).Error; err != nil {
		return template, err
	}
	revision := entity.RoutingTemplateRevision{TemplateID: template.ID, Revision: 1, SchemaVersion: 2, Payload: preset.Payload, Actor: "system:preset"}
	return template, db.Create(&revision).Error
}
