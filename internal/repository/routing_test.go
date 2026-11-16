package repository

import (
	"github.com/imrui/xray-pilot/internal/entity"
	"testing"
	"time"
)

func TestRoutingMigrationPreservesLegacyAndIsRepeatable(t *testing.T) {
	setupRepositoryTestDB(t)
	old := entity.Node{Name: "old", ConfigHash: "unchanged", SyncStatus: entity.SyncStatusSynced}
	if err := DB.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateRoutingDefaults(DB); err != nil {
		t.Fatal(err)
	}
	if err := migrateRoutingDefaults(DB); err != nil {
		t.Fatal(err)
	}
	var binding entity.RoutingBinding
	if err := DB.Where("scope = ? AND target_id = ?", "node", old.ID).First(&binding).Error; err != nil {
		t.Fatal(err)
	}
	if binding.Mode != "legacy" {
		t.Fatal("legacy node opted into new routing")
	}
	var n entity.Node
	DB.First(&n, old.ID)
	if n.ConfigHash != "unchanged" || n.SyncStatus != entity.SyncStatusSynced {
		t.Fatal("migration changed applied state")
	}
	var count int64
	DB.Model(&entity.RoutingTemplate{}).Where("name = ?", "系统默认路由").Count(&count)
	if count != 1 {
		t.Fatalf("duplicate defaults %d", count)
	}
	fresh := entity.Node{Name: "new"}
	if err := NewNodeRepository().Create(&fresh); err != nil {
		t.Fatal(err)
	}
	var freshBinding entity.RoutingBinding
	DB.Where("scope = ? AND target_id = ?", "node", fresh.ID).First(&freshBinding)
	if freshBinding.Mode != "inherit" {
		t.Fatal("new node didn't inherit")
	}
}
func TestConfigLeaseCannotBeStolenOrReleasedByPreviousOwner(t *testing.T) {
	setupRepositoryTestDB(t)
	n := entity.Node{Name: "lease"}
	DB.Create(&n)
	first, release1, err := AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := AcquireConfigLease(n.ID); err == nil {
		t.Fatal("concurrent writer acquired lease")
	}
	DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("lease_until", time.Now().Add(-time.Minute))
	next, release2, err := AcquireConfigLease(n.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	release1()
	var current entity.NodeConfigState
	DB.First(&current, "node_id = ?", n.ID)
	if current.LeaseOwner != next.LeaseOwner || current.LeaseOwner == first.LeaseOwner {
		t.Fatal("old writer released replacement lease")
	}
}
