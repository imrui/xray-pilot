package repository

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/entity"
)

func setupRepositoryTestDB(t *testing.T) {
	t.Helper()

	if DB != nil {
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(t.TempDir(), "repo-test.db")

	if err := Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		if DB == nil {
			return
		}
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestMigrateLegacyUserGroupsCopiesExistingAssignments(t *testing.T) {
	setupRepositoryTestDB(t)

	group := entity.Group{Name: "cn", Active: true}
	if err := DB.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	user := entity.User{
		Username:      "pilot",
		UUID:          "uuid-1",
		Token:         "token-1",
		Active:        true,
		LegacyGroupID: &group.ID,
	}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := DB.Exec("DELETE FROM user_groups").Error; err != nil {
		t.Fatalf("clear user_groups: %v", err)
	}

	if err := migrateLegacyUserGroups(DB); err != nil {
		t.Fatalf("migrate legacy user groups: %v", err)
	}

	var count int64
	if err := DB.Model(&entity.UserGroup{}).
		Where("user_id = ? AND group_id = ?", user.ID, group.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count migrated rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 migrated user_group row, got %d", count)
	}
}

func TestFindActiveUsersByNodeIDDeduplicatesAcrossGroups(t *testing.T) {
	setupRepositoryTestDB(t)

	node := entity.Node{Name: "node-1", IP: "192.168.1.10", Active: true}
	if err := DB.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	groupA := entity.Group{Name: "cn", Active: true}
	groupB := entity.Group{Name: "aa", Active: true}
	if err := DB.Create(&groupA).Error; err != nil {
		t.Fatalf("create groupA: %v", err)
	}
	if err := DB.Create(&groupB).Error; err != nil {
		t.Fatalf("create groupB: %v", err)
	}
	if err := DB.Model(&groupA).Association("Nodes").Append(&node); err != nil {
		t.Fatalf("append node to groupA: %v", err)
	}
	if err := DB.Model(&groupB).Association("Nodes").Append(&node); err != nil {
		t.Fatalf("append node to groupB: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "tt",
		UUID:      "uuid-tt",
		Token:     "token-tt",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := DB.Model(&user).Association("Groups").Replace([]entity.Group{groupA, groupB}); err != nil {
		t.Fatalf("replace user groups: %v", err)
	}

	repo := NewUserRepository()
	users, err := repo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		t.Fatalf("find active users by node: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 unique user, got %d", len(users))
	}

	total, err := repo.CountActiveByNodeID(node.ID)
	if err != nil {
		t.Fatalf("count active users by node: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected active user count 1, got %d", total)
	}
}
