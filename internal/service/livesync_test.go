package service

import (
	"reflect"
	"sort"
	"testing"
)

func TestNodeSetIf(t *testing.T) {
	ids := []uint{1, 2, 3}
	if got := nodeSetIf(true, ids); !reflect.DeepEqual(got, ids) {
		t.Errorf("active=true 应原样返回，得 %v", got)
	}
	if got := nodeSetIf(false, ids); got != nil {
		t.Errorf("active=false 应返回 nil（未激活用户不在任何节点），得 %v", got)
	}
}

func TestNodeIDsDiff(t *testing.T) {
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
			got := nodeIDsDiff(c.a, c.b)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("nodeIDsDiff(%v,%v) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

// TestUpdateDiffScenarios 验证「旧/新期望节点集」差集对账在典型用户更新场景下算出正确的增删集。
// 模拟 applyUpdateLive 的核心计算（不触 DB/gRPC）。
func TestUpdateDiffScenarios(t *testing.T) {
	diff := func(oldActive bool, oldNodes []uint, newActive bool, newNodes []uint) (add, remove []uint) {
		desiredOld := nodeSetIf(oldActive, oldNodes)
		desiredNew := nodeSetIf(newActive, newNodes)
		return nodeIDsDiff(desiredNew, desiredOld), nodeIDsDiff(desiredOld, desiredNew)
	}
	sortU := func(s []uint) []uint { sort.Slice(s, func(i, j int) bool { return s[i] < s[j] }); return s }
	eq := func(a, b []uint) bool {
		if len(a) == 0 && len(b) == 0 {
			return true
		}
		return reflect.DeepEqual(sortU(a), sortU(b))
	}

	t.Run("加入新分组_节点扩大", func(t *testing.T) {
		add, rm := diff(true, []uint{1}, true, []uint{1, 2})
		if !eq(add, []uint{2}) || !eq(rm, nil) {
			t.Errorf("add=%v rm=%v, want add=[2] rm=[]", add, rm)
		}
	})
	t.Run("移出分组_节点缩小", func(t *testing.T) {
		add, rm := diff(true, []uint{1, 2, 3}, true, []uint{1})
		if !eq(add, nil) || !eq(rm, []uint{2, 3}) {
			t.Errorf("add=%v rm=%v, want add=[] rm=[2 3]", add, rm)
		}
	})
	t.Run("禁用_从所有节点摘除", func(t *testing.T) {
		add, rm := diff(true, []uint{1, 2}, false, []uint{1, 2})
		if !eq(add, nil) || !eq(rm, []uint{1, 2}) {
			t.Errorf("add=%v rm=%v, want add=[] rm=[1 2]", add, rm)
		}
	})
	t.Run("启用_加入所有节点", func(t *testing.T) {
		add, rm := diff(false, []uint{1, 2}, true, []uint{1, 2})
		if !eq(add, []uint{1, 2}) || !eq(rm, nil) {
			t.Errorf("add=%v rm=%v, want add=[1 2] rm=[]", add, rm)
		}
	})
	t.Run("仅延长有效期_仍有效_无增删", func(t *testing.T) {
		add, rm := diff(true, []uint{1, 2}, true, []uint{1, 2})
		if !eq(add, nil) || !eq(rm, nil) {
			t.Errorf("add=%v rm=%v, want 空（仅非节点字段变更应无 live-apply）", add, rm)
		}
	})
	// 以下两例的 active 入参代表 EffectiveActive（启用 且 未过期），
	// 覆盖 v0.4.9 过期/续期场景：期望集必须随过期状态变化，否则续期后用户不会被加回运行时。
	t.Run("过期用户续期_加回所有节点", func(t *testing.T) {
		add, rm := diff(false, []uint{1, 2}, true, []uint{1, 2})
		if !eq(add, []uint{1, 2}) || !eq(rm, nil) {
			t.Errorf("add=%v rm=%v, want add=[1 2] rm=[]", add, rm)
		}
	})
	t.Run("有效用户改为已过期_从所有节点摘除", func(t *testing.T) {
		add, rm := diff(true, []uint{1, 2}, false, []uint{1, 2})
		if !eq(add, nil) || !eq(rm, []uint{1, 2}) {
			t.Errorf("add=%v rm=%v, want add=[] rm=[1 2]", add, rm)
		}
	})
}
