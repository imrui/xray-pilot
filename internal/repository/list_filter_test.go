package repository

import (
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

func TestUserListFiltersBeforePagination(t *testing.T) {
	setupRepositoryTestDB(t)
	groups := []entity.Group{{Name: "ops_1"}, {Name: "ops_2"}}
	if err := DB.Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	users := []entity.User{
		{Username: "first", UUID: "u1", Token: "t1", FeishuEmail: "Pilot@Example.test", Groups: groups},
		{Username: "expired", UUID: "u2", Token: "t2", RealName: "PILOT", Groups: groups[:1], ExpiresAt: &past},
		{Username: "disabled", UUID: "u3", Token: "t3", Remark: "100%", Groups: groups[1:]},
	}
	if err := DB.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&users[2]).Update("active", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		filter UserListFilter
		page   int
		total  int64
		name   string
	}{
		{UserListFilter{}, 1, 3, "disabled"},
		{UserListFilter{Keyword: " pilot ", Status: "active", GroupID: groups[0].ID}, 1, 1, "first"},
		{UserListFilter{Keyword: "pilot"}, 2, 2, "first"},
		{UserListFilter{Keyword: "ops_", GroupID: groups[0].ID}, 2, 2, "first"},
		{UserListFilter{Status: "expired"}, 1, 1, "expired"},
		{UserListFilter{Keyword: "%", Status: "inactive"}, 1, 1, "disabled"},
		{UserListFilter{Keyword: "%", GroupID: groups[0].ID}, 1, 0, ""},
	} {
		got, total, err := NewUserRepository().List(tc.page, 1, tc.filter)
		if err != nil || total != tc.total {
			t.Fatalf("filter %+v page %d: total %d, err %v", tc.filter, tc.page, total, err)
		}
		if tc.name == "" && len(got) != 0 || tc.name != "" && (len(got) != 1 || got[0].Username != tc.name) {
			t.Fatalf("filter %+v page %d: got %v, want %q", tc.filter, tc.page, usernames(got), tc.name)
		}
	}
}

func TestNodeListLiteralKeywordWithOtherFilters(t *testing.T) {
	setupRepositoryTestDB(t)
	nodes := []entity.Node{
		{Name: "old", IP: "192.0.2.1", Owner: "east", Remark: "rack_8%"},
		{Name: "new", IP: "192.0.2.2", Owner: "east", Remark: "rack-80"},
		{Name: "rack_8%", IP: "192.0.2.3", Owner: "west"},
	}
	if err := DB.Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	got, total, err := NewNodeRepository().List(1, 1, NodeListFilter{Keyword: " RACK_8% ", Owner: "east"})
	if err != nil || total != 1 || len(got) != 1 || got[0].ID != nodes[0].ID {
		t.Fatalf("filtered page = %+v, total %d, err %v", got, total, err)
	}
}
