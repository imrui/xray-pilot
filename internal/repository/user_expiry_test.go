package repository

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

// 旧库中无授权节点的到期用户可一次完成，禁用和已确认用户不会重复处理。
func TestPrepareExpirySweep(t *testing.T) {
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
			if err := DB.Model(&seed[i]).Update("active", false).Error; err != nil {
				t.Fatalf("disable %s: %v", seed[i].Username, err)
			}
		}
	}

	targets, names, err := PrepareExpirySweep(now)
	if err != nil || len(targets) != 0 || len(names) != 1 || names[0] != "expired" {
		t.Fatalf("first sweep: targets=%v names=%v err=%v", targets, names, err)
	}
	targets, names, err = PrepareExpirySweep(now)
	if err != nil || len(targets) != 0 || len(names) != 0 {
		t.Fatalf("repeat sweep: targets=%v names=%v err=%v", targets, names, err)
	}
}

func usernames(users []entity.User) []string {
	names := make([]string, 0, len(users))
	for _, user := range users {
		names = append(names, user.Username)
	}
	return names
}

func TestNodeDifference(t *testing.T) {
	cases := []struct {
		name string
		a, b []uint
		want []uint
	}{
		{"加入新节点", []uint{1, 2, 3}, []uint{1}, []uint{2, 3}},
		{"无变化", []uint{1, 2}, []uint{1, 2}, nil},
		{"全部移除", nil, []uint{1, 2}, nil},
		{"从空集加入", []uint{1, 2}, nil, []uint{1, 2}},
		{"部分重叠", []uint{1, 2, 3}, []uint{2, 4}, []uint{1, 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nodeDifference(c.a, c.b)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("nodeDifference(%v,%v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}
