package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

func TestReplaceHostPreservesConcurrentNodeEdits(t *testing.T) {
	setupRepositoryTestDB(t)
	repo := NewNodeRepository()
	install := entity.Node{Name: "original", Domain: "old.example", IP: "192.0.2.1", XrayVersion: "old",
		XrayActive: true, SyncStatus: entity.SyncStatusSynced, ConfigHash: "old-machine"}
	if err := repo.Create(&install); err != nil {
		t.Fatal(err)
	}
	_, release, err := AcquireConfigLease(install.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// 在安装器读取节点后，管理员保存了新的域名绑定与备注，健康检测也写回了结果。
	now := time.Now().UTC()
	if err := DB.Model(&entity.Node{}).Where("id = ?", install.ID).Updates(map[string]any{
		"name": "renamed", "remark": "new note", "domain": "bound.example", "active": false,
		"last_check_at": now, "last_check_ok": true, "last_latency_ms": 12,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Save(&entity.DNSConfig{ID: 1, ZoneID: "zone", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	binding := entity.DNSBinding{NodeID: install.ID, ZoneID: "zone", Domain: "bound.example", Type: "A",
		DesiredIP: install.IP, Status: entity.DNSSynced}
	if err := DB.Create(&binding).Error; err != nil {
		t.Fatal(err)
	}
	install.IP, install.SSHUser, install.SSHPort = "192.0.2.2", "new-admin", 2222
	install.XrayVersion, install.RegisteredAt = "", &now
	if err := repo.ReplaceHost(&install, "", "test"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindByID(install.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []*entity.Node{&install, stored} {
		if n.Name != "renamed" || n.Remark != "new note" || n.Domain != "bound.example" || n.Active ||
			n.LastCheckAt == nil || !n.LastCheckAt.Equal(now) || !n.LastCheckOK || n.LastLatencyMs != 12 {
			t.Fatalf("replacement overwrote concurrent updates: %+v", n)
		}
		if n.IP != "192.0.2.2" || n.SSHUser != "new-admin" || n.SSHPort != 2222 || n.XrayVersion != "" ||
			n.RegisteredAt == nil || !n.RegisteredAt.Equal(now) || n.XrayActive || n.ConfigHash != "" || n.SyncStatus != entity.SyncStatusPending {
			t.Fatalf("replacement did not apply new machine details: %+v", n)
		}
	}
	if err := DB.First(&binding, binding.ID).Error; err != nil || binding.DesiredIP != install.IP || binding.Status != entity.DNSPending {
		t.Fatalf("replacement lost the current DNS binding: %+v %v", binding, err)
	}
}

func TestReplaceHostDoesNotRecreateDeletedNode(t *testing.T) {
	setupRepositoryTestDB(t)
	repo := NewNodeRepository()
	n := entity.Node{Name: "deleted", IP: "192.0.2.1"}
	if err := repo.Create(&n); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(n.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceHost(&n, "", "test"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("replacement of deleted node: %v", err)
	}
	if _, err := repo.FindByID(n.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("deleted node was recreated: %v", err)
	}
	var count int64
	if err := DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("deleted node left recovery work: %d %v", count, err)
	}
}
