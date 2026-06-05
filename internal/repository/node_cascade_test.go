package repository

import (
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
)

// TestNodeDelete_CascadesAssociations 验证删节点会级联清理 group_nodes 与
// node_profile_keys 关联表，防止 SQLite ID 复用时"继承"旧节点的关联。
func TestNodeDelete_CascadesAssociations(t *testing.T) {
	setupRepositoryTestDB(t)

	// 准备：节点 + 分组 + 协议 + 关联
	node := entity.Node{Name: "n1", IP: "1.1.1.1", Active: true}
	if err := DB.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	group := entity.Group{Name: "g1", Active: true}
	if err := DB.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	profile := entity.InboundProfile{Name: "p1", Protocol: "vless-reality", Port: 443, Active: true}
	if err := DB.Create(&profile).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := DB.Exec("INSERT INTO group_nodes(group_id, node_id) VALUES (?, ?)", group.ID, node.ID).Error; err != nil {
		t.Fatalf("link group_nodes: %v", err)
	}
	key := entity.NodeProfileKey{NodeID: node.ID, ProfileID: profile.ID, Settings: "{}"}
	if err := DB.Create(&key).Error; err != nil {
		t.Fatalf("create node_profile_key: %v", err)
	}

	// 执行删除
	if err := NewNodeRepository().Delete(node.ID); err != nil {
		t.Fatalf("delete node: %v", err)
	}

	// 验证：节点本身被删
	var nodeCount int64
	DB.Model(&entity.Node{}).Where("id = ?", node.ID).Count(&nodeCount)
	if nodeCount != 0 {
		t.Errorf("node row not deleted")
	}

	// 验证：group_nodes 中间表对应行被清
	var gnCount int64
	DB.Table("group_nodes").Where("node_id = ?", node.ID).Count(&gnCount)
	if gnCount != 0 {
		t.Errorf("group_nodes orphan remaining: %d", gnCount)
	}

	// 验证：node_profile_keys 对应行被清
	var npkCount int64
	DB.Model(&entity.NodeProfileKey{}).Where("node_id = ?", node.ID).Count(&npkCount)
	if npkCount != 0 {
		t.Errorf("node_profile_keys orphan remaining: %d", npkCount)
	}
}

// TestCleanupOrphanRelations 验证启动时孤儿清理覆盖所有六个方向。
func TestCleanupOrphanRelations(t *testing.T) {
	setupRepositoryTestDB(t)

	// 故意塞入指向不存在主体的孤儿行（模拟 v0.4.3 之前残留的脏数据）
	DB.Exec("INSERT INTO group_nodes(group_id, node_id) VALUES (?, ?)", 999, 999)
	DB.Exec("INSERT INTO node_profile_keys(node_id, profile_id, settings, port) VALUES (?, ?, ?, ?)", 999, 999, "{}", 0)
	DB.Exec("INSERT INTO user_groups(user_id, group_id) VALUES (?, ?)", 999, 999)

	if err := cleanupOrphanRelations(DB); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	for _, c := range []struct {
		table string
		where string
	}{
		{"group_nodes", "node_id = 999"},
		{"group_nodes", "group_id = 999"},
		{"node_profile_keys", "node_id = 999"},
		{"node_profile_keys", "profile_id = 999"},
		{"user_groups", "user_id = 999"},
		{"user_groups", "group_id = 999"},
	} {
		var count int64
		DB.Table(c.table).Where(c.where).Count(&count)
		if count != 0 {
			t.Errorf("orphan in %s where %s still present: %d", c.table, c.where, count)
		}
	}
}
