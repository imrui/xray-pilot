package service

import (
	"errors"
	"testing"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"gorm.io/gorm"
)

func TestLiveNodeLookupFailureRetainsSingleQueuedRevision(t *testing.T) {
	nodes, group := groupSyncFixture(t)
	before := captureGroupSync(t, nodes, true)
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "true"); err != nil {
		t.Fatal(err)
	}
	callback := repository.DB.Callback().Query()
	failed := false
	if err := callback.Before("gorm:query").Register("test:failed_live_lookup", func(tx *gorm.DB) {
		_, nodeList := tx.Statement.Dest.(*[]entity.Node)
		if !failed && nodeList && tx.Statement.Table == "nodes" {
			failed = true
			tx.AddError(errors.New("injected live node lookup failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = callback.Remove("test:failed_live_lookup") })
	_, err := NewUserService().Create(&dto.CreateUserRequest{Username: "lookup", GroupIDs: []uint{group}}, "")
	if err != nil || !failed {
		t.Fatalf("expected failed post-commit lookup: %v", err)
	}
	assertGroupSyncImpact(t, nodes, before, nodes[0].ID, nodes[1].ID)
}

func TestLiveSSHFailureKeepsDurableRecovery(t *testing.T) {
	n, u := routingFixture(t)
	nodes := []entity.Node{*n}
	before := captureGroupSync(t, nodes, true)[n.ID].state
	if err := repository.NewSettingRepository().Set(KeyXrayLiveApply, "true"); err != nil {
		t.Fatal(err)
	}
	if err := NewUserService().ToggleActive(u.ID); err != nil {
		t.Fatal(err)
	}
	got := captureGroupSync(t, nodes, false)[n.ID]
	if got.status != entity.SyncStatusDrifted || got.state.ChangeRevision != before.ChangeRevision+1 || !got.state.RecoveryPending || got.state.RecoveryAttempts != 1 || got.state.LastError == "" || got.state.NextRetryAt == nil {
		t.Fatalf("lost or duplicated failed live apply: %+v", got.state)
	}
}
