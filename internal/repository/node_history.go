package repository

import (
	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
)

const maxKnownNodeID = `SELECT COALESCE(MAX(n), 0) FROM (
 SELECT MAX(id) AS n FROM nodes UNION ALL SELECT MAX(node_id) FROM deleted_nodes
 UNION ALL SELECT MAX(node_id) FROM traffic_samples UNION ALL SELECT MAX(node_id) FROM traffic_polls
) AS node_ids`

func nextSQLiteNodeID(tx *gorm.DB, node *entity.Node) error {
	var hasSequence bool
	if err := tx.Raw("SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence')").Scan(&hasSequence).Error; err != nil {
		return err
	}
	highWater := "(" + maxKnownNodeID + ")"
	if hasSequence {
		// 旧流量已清理时仍保留 AUTOINCREMENT 记录的高水位。
		highWater = "MAX(" + highWater + ", COALESCE((SELECT seq FROM sqlite_sequence WHERE name = 'nodes'), 0))"
	}
	return tx.Raw("SELECT " + highWater + " + 1").Scan(&node.ID).Error
}

func advanceNodeSequence(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("LOCK TABLE nodes IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
			return err
		}
		var highest int64
		if err := tx.Raw(maxKnownNodeID).Scan(&highest).Error; err != nil {
			return err
		}
		var seq string
		if err := tx.Raw("SELECT pg_get_serial_sequence('nodes', 'id')").Scan(&seq).Error; err != nil {
			return err
		}
		var state struct {
			LastValue int64
			IsCalled  bool
		}
		// pg_get_serial_sequence 返回数据库自身引用过的限定标识符。
		if err := tx.Raw("SELECT last_value, is_called FROM " + seq).Scan(&state).Error; err != nil {
			return err
		}
		if highest < state.LastValue || (highest == state.LastValue && state.IsCalled) {
			return nil
		}
		return tx.Exec("SELECT setval(?::regclass, ?, true)", seq, highest).Error
	})
}
