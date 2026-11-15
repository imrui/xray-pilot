package service

import (
	"encoding/json"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
	"testing"
)

func TestRealityShortIDMigration(t *testing.T) {
	for _, tc := range []struct {
		name, raw, old string
		reject, empty  bool
	}{
		{"new-default", `{"private_key":"fixture"}`, "", false, false},
		{"explicit-empty-rejected", `{"private_key":"fixture","short_ids":[""]}`, "", true, false},
		{"explicit-empty-allowed", `{"private_key":"fixture","short_ids":[""],"allow_empty_short_id":true}`, "", false, true},
		{"legacy-preserved", `{"private_key":"fixture"}`, `{"private_key":"fixture","short_ids":[""]}`, false, true},
		{"odd-hex", `{"short_ids":["abc"]}`, "", true, false},
		{"invalid-hex", `{"short_ids":["zz"]}`, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := normalizeRealityShortIDs(tc.raw, tc.old)
			if (err != nil) != tc.reject {
				t.Fatalf("error=%v", err)
			}
			if tc.reject {
				return
			}
			var keys types.RealityKeyMaterial
			if err := json.Unmarshal([]byte(raw), &keys); err != nil {
				t.Fatal(err)
			}
			if len(keys.ShortIds) == 0 || (keys.ShortIds[0] == "") != tc.empty {
				t.Fatalf("short IDs: %v", keys.ShortIds)
			}
		})
	}
}

func TestRevokeLastProtocolKeepsOnlyAPI(t *testing.T) {
	n, _ := routingFixture(t)
	n.ConfigHash = "previously-managed"
	if err := repository.DB.Model(&entity.InboundProfile{}).Where("1 = 1").Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	raw, _, err := NewSyncService().buildConfig(n)
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Inbounds []struct{ Protocol, Tag string }
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 1 || cfg.Inbounds[0].Tag != "api-inbound" {
		t.Fatalf("unexpected listeners: %+v", cfg.Inbounds)
	}
}
