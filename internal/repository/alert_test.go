package repository

import (
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
)

// TestAlertState_ConcurrentObserveAndNotify 回归 v0.5.1：同步失败的告警发送期间同步又成功，
// Flush 回写送达状态不能把 Observe 新写入的「已恢复」判定覆盖回去，否则恢复消息永远发不出。
func TestAlertState_ConcurrentObserveAndNotify(t *testing.T) {
	setupRepositoryTestDB(t)
	repo := NewAlertStateRepository()

	// 失败：进入告警，尚未送达
	st, _ := repo.Get(7, entity.AlertKindSyncFailed)
	st.FailCount, st.Alerting, st.LastError = 1, true, "mv failed"
	if err := repo.SaveObservation(st); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Flush 读出待发送状态（此后持有的是旧快照）
	flushed, _ := repo.Get(7, entity.AlertKindSyncFailed)

	// 发送期间同步成功：Observe 推进为恢复
	cur, _ := repo.Get(7, entity.AlertKindSyncFailed)
	cur.FailCount, cur.OKCount, cur.Alerting = 0, 1, false
	if err := repo.SaveObservation(cur); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Flush 送达后按旧快照回写送达字段
	if err := repo.MarkNotified(flushed.ID, flushed.Alerting, time.Now()); err != nil {
		t.Fatalf("mark: %v", err)
	}

	got, _ := repo.Get(7, entity.AlertKindSyncFailed)
	if got.Alerting {
		t.Fatal("送达回写覆盖了 Observe 的恢复判定")
	}
	if !got.NotifiedAlerting {
		t.Fatal("送达状态未回写")
	}
	// 判定 false、已送达 true → 下一轮应发恢复
	pending, _ := repo.FindPending()
	if len(pending) != 1 {
		t.Fatalf("应有 1 条待发送的恢复，got %d", len(pending))
	}
}
