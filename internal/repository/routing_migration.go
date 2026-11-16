package repository

import (
	"errors"
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func migrateRoutingDefaults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var binding entity.RoutingBinding
		err := tx.Where("scope = ? AND target_id = ?", "global", 0).First(&binding).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			template := entity.RoutingTemplate{Name: "系统默认路由", Description: "默认正常直连；仅指定域名强制 IPv4 或 IPv6，不自动启用预置域名。", Draft: entity.DefaultRoutingPolicy, Version: 1}
			if err := tx.Create(&template).Error; err != nil {
				return err
			}
			rev := entity.RoutingTemplateRevision{TemplateID: template.ID, Revision: 1, SchemaVersion: 2, Payload: entity.DefaultRoutingPolicy, Actor: "system:migration"}
			if err := tx.Create(&rev).Error; err != nil {
				return err
			}
			binding = entity.RoutingBinding{Scope: "global", Mode: "template", TemplateID: template.ID, Revision: 1, Version: 1, UDP443Policy: "inherit", Overrides: "{}"}
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
			b := entity.RoutingBinding{Scope: "node", TargetID: id, Mode: "legacy", Version: 1, Overrides: "{}", UDP443Policy: "inherit"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&b).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
