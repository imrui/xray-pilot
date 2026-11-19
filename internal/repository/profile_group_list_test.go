package repository

import (
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
)

func TestProfileAndGroupFiltersRunBeforePagination(t *testing.T) {
	setupRepositoryTestDB(t)
	profiles := []entity.InboundProfile{
		{Name: "SOCKS5 迁移", Protocol: "http"},
		{Name: "TGT_1", Protocol: "socks"},
		{Name: "tgtX1", Protocol: "socks"},
		{Name: "tgt_2", Protocol: "http"},
		{Name: "tgt_3", Protocol: "socks"},
	}
	if err := DB.Create(&profiles).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewInboundProfileRepository()
	rows, total, err := repo.List(2, 1, " tgt_ ", "socks", nil)
	if err != nil || total != 2 || len(rows) != 1 || rows[0].ID != profiles[4].ID {
		t.Fatalf("filtered profiles: rows=%v total=%d err=%v", rows, total, err)
	}
	rows, total, err = repo.List(1, 10, "HTTP", "", nil)
	if err != nil || len(rows) != 2 || total != 2 {
		t.Fatalf("search by protocol: rows=%v total=%d err=%v", rows, total, err)
	}
	rows, total, err = repo.List(1, 10, "SOCKS5", "", []string{"socks"})
	if err != nil || len(rows) != 4 || total != 4 {
		t.Fatalf("search by name or display protocol: rows=%v total=%d err=%v", rows, total, err)
	}
	rows, total, err = repo.List(1, 10, "SOCKS5", "http", []string{"socks"})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != profiles[0].ID {
		t.Fatalf("intersect protocol dropdown: rows=%v total=%d err=%v", rows, total, err)
	}

	groups := []entity.Group{
		{Name: "first", Description: "POOL% one"},
		{Name: "pool% two"},
		{Name: "poolX"},
		{Name: "pool% disabled"},
	}
	if err := DB.Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&groups[3]).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	active := true
	hits, total, err := NewGroupRepository().List(2, 1, "pool%", &active)
	if err != nil || total != 2 || len(hits) != 1 || hits[0].ID != groups[0].ID {
		t.Fatalf("filtered groups: rows=%v total=%d err=%v", hits, total, err)
	}
	active = false
	hits, total, err = NewGroupRepository().List(1, 1, "pool%", &active)
	if err != nil || total != 1 || len(hits) != 1 || hits[0].ID != groups[3].ID {
		t.Fatalf("disabled groups: rows=%v total=%d err=%v", hits, total, err)
	}
}
