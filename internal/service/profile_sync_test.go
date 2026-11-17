package service

import (
	"encoding/json"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
)

func TestNodeProtocolSyncScope(t *testing.T) {
	for _, action := range []string{"port", "bind", "unbind", "keygen", "same-encrypted-key", "locked", "failed-save", "inactive"} {
		t.Run(action, func(t *testing.T) {
			nodes, _ := groupSyncFixture(t)
			var profile entity.InboundProfile
			if err := repository.DB.First(&profile).Error; err != nil {
				t.Fatal(err)
			}
			for _, n := range nodes {
				if err := repository.DB.Model(&n).Update("config_hash", "managed").Error; err != nil {
					t.Fatal(err)
				}
			}
			target := nodes[1].ID
			svc := NewProfileService()
			settings := json.RawMessage(`{}`)
			switch action {
			case "bind":
				if err := repository.DB.Where("node_id = ?", target).Delete(&entity.NodeProfileKey{}).Error; err != nil {
					t.Fatal(err)
				}
			case "same-encrypted-key":
				key, err := svc.KeygenForNode(target, profile.ID)
				if err != nil {
					t.Fatal(err)
				}
				settings = key.Settings
			case "locked":
				if err := svc.SetNodeKeyLocked(target, profile.ID, true); err != nil {
					t.Fatal(err)
				}
			case "failed-save":
				if err := repository.DB.Exec("CREATE TRIGGER reject_key_save BEFORE UPDATE ON node_profile_keys BEGIN SELECT RAISE(ABORT, 'injected failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			case "inactive":
				if err := repository.DB.Model(&profile).Update("active", false).Error; err != nil {
					t.Fatal(err)
				}
			}
			captureGroupSync(t, nodes, true)
			if err := repository.DB.Model(&nodes[2]).Update("sync_status", entity.SyncStatusFailed).Error; err != nil {
				t.Fatal(err)
			}
			before := captureGroupSync(t, nodes, false)
			var err error
			port := 24443
			if action == "same-encrypted-key" {
				port = 0
			}
			switch action {
			case "unbind":
				err = svc.DeleteNodeKey(target, profile.ID)
			case "keygen":
				_, err = svc.KeygenForNode(target, profile.ID)
			default:
				_, err = svc.UpsertNodeKey(target, profile.ID, &dto.UpsertNodeKeyRequest{Port: &port, Settings: settings})
			}
			if (err != nil) != (action == "locked" || action == "failed-save") {
				t.Fatalf("unexpected save result: %v", err)
			}
			if action == "same-encrypted-key" || action == "locked" || action == "failed-save" || action == "inactive" {
				assertGroupSyncImpact(t, nodes, before)
			} else {
				assertGroupSyncImpact(t, nodes, before, target)
			}
		})
	}
}

func TestSharedProtocolOnlyInvalidatesBoundNodes(t *testing.T) {
	for _, action := range []string{"create", "metadata", "default-port", "settings", "disable", "delete"} {
		t.Run(action, func(t *testing.T) {
			nodes, _ := groupSyncFixture(t)
			var profile entity.InboundProfile
			if err := repository.DB.First(&profile).Error; err != nil {
				t.Fatal(err)
			}
			for _, n := range nodes {
				if err := repository.DB.Model(&n).Update("config_hash", "managed").Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := repository.DB.Where("node_id = ?", nodes[2].ID).Delete(&entity.NodeProfileKey{}).Error; err != nil {
				t.Fatal(err)
			}
			if err := repository.DB.Model(&entity.NodeProfileKey{}).Where("node_id = ?", nodes[1].ID).Update("port", 24443).Error; err != nil {
				t.Fatal(err)
			}
			before := captureGroupSync(t, nodes, true)
			svc := NewProfileService()
			var err error
			var changed []uint
			switch action {
			case "create":
				_, err = svc.Create(&dto.CreateProfileRequest{Name: "unused", Protocol: types.ProtocolVlessReality, Port: 443, Settings: json.RawMessage(profile.Settings)})
			case "metadata":
				name, remark := "renamed", "description"
				_, err = svc.Update(profile.ID, &dto.UpdateProfileRequest{Name: &name, Remark: &remark})
			case "default-port":
				port := 24444
				_, err = svc.Update(profile.ID, &dto.UpdateProfileRequest{Port: &port})
				changed = []uint{nodes[0].ID}
			case "settings":
				_, err = svc.Update(profile.ID, &dto.UpdateProfileRequest{Settings: json.RawMessage(`{"sni":"www.apple.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"]}`)})
				changed = []uint{nodes[0].ID, nodes[1].ID}
			case "disable":
				active := false
				_, err = svc.Update(profile.ID, &dto.UpdateProfileRequest{Active: &active})
				changed = []uint{nodes[0].ID, nodes[1].ID}
			case "delete":
				err = svc.Delete(profile.ID)
				changed = []uint{nodes[0].ID, nodes[1].ID}
			}
			if err != nil {
				t.Fatal(err)
			}
			assertGroupSyncImpact(t, nodes, before, changed...)
		})
	}
}
