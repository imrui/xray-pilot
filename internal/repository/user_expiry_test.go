package repository

import (
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

// TestFindExpiredUnswept 验证过期摘除查询只返回「启用 + 已过期 + 未打标」的用户，
// 且 MarkExpiredSwept 后不再返回
func TestFindExpiredUnswept(t *testing.T) {
	setupRepositoryTestDB(t)
	repo := NewUserRepository()

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	seed := []entity.User{
		{Username: "expired", UUID: "u1", Token: "t1", Active: true, ExpiresAt: &past},
		{Username: "valid", UUID: "u2", Token: "t2", Active: true, ExpiresAt: &future},
		{Username: "forever", UUID: "u3", Token: "t3", Active: true},
		{Username: "disabled-expired", UUID: "u4", Token: "t4", Active: false, ExpiresAt: &past},
		{Username: "already-swept", UUID: "u5", Token: "t5", Active: true, ExpiresAt: &past, ExpiredSweptAt: &past},
	}
	for i := range seed {
		wantActive := seed[i].Active // Create 会把 default:true 回填进结构体，先记下意图
		if err := repo.Create(&seed[i]); err != nil {
			t.Fatalf("create %s: %v", seed[i].Username, err)
		}
		// GORM 对带 default:true 的零值 bool 会写入默认值，禁用态需 Create 后单独落库
		if !wantActive {
			if err := repo.UpdateActive(seed[i].ID, false); err != nil {
				t.Fatalf("disable %s: %v", seed[i].Username, err)
			}
		}
	}

	users, err := repo.FindExpiredUnswept(now)
	if err != nil {
		t.Fatalf("FindExpiredUnswept: %v", err)
	}
	if len(users) != 1 || users[0].Username != "expired" {
		t.Fatalf("want only [expired], got %+v", usernames(users))
	}

	if err := repo.MarkExpiredSwept(users[0].ID, now); err != nil {
		t.Fatalf("MarkExpiredSwept: %v", err)
	}
	users, err = repo.FindExpiredUnswept(now)
	if err != nil {
		t.Fatalf("FindExpiredUnswept after mark: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("打标后不应再返回，got %v", usernames(users))
	}
}

func usernames(users []entity.User) []string {
	out := make([]string, 0, len(users))
	for _, u := range users {
		out = append(out, u.Username)
	}
	return out
}
