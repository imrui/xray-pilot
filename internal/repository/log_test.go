package repository

import (
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
)

func TestLogImportantFilter(t *testing.T) {
	setupTrafficTestDB(t)
	for _, entry := range []entity.SyncLog{
		{Actor: "admin:alice", Action: "node_update", Success: true},
		{Actor: "system:scheduler:traffic", Action: "traffic_poll", Success: false},
		{Actor: "system:scheduler:traffic", Action: "traffic_retention", Success: true},
	} {
		if err := DB.Create(&entry).Error; err != nil {
			t.Fatal(err)
		}
	}
	r := NewLogRepository()
	logs, total, err := r.List(1, 10, "", true)
	if err != nil || total != 2 || len(logs) != 2 {
		t.Fatalf("important logs = %+v, %d, %v", logs, total, err)
	}
	_, total, err = r.List(1, 10, "system:", true)
	if err != nil || total != 1 {
		t.Fatalf("combined filter = %d, %v", total, err)
	}
	_, total, err = r.List(1, 10, "")
	if err != nil || total != 3 {
		t.Fatalf("default list = %d, %v", total, err)
	}
}

func TestLogSearchFiltersBeforePaging(t *testing.T) {
	setupTrafficTestDB(t)
	const feishu = "system:feishu-webhook"
	entries := []entity.SyncLog{
		{Actor: feishu, Action: "RETRY", Message: "oldest"},
		{Actor: feishu, Target: "node:retry"},
		{Actor: feishu, Message: "Retry 50%_done"},
		{Actor: "admin:retry", Message: "newest match"},
		{Actor: feishu, Message: "50xxdone"},
		{Actor: feishu, Action: "unrelated"},
	}
	if err := DB.Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		filter  LogListFilter
		page    int
		total   int64
		entryID uint
	}{
		{"actor field", LogListFilter{Keyword: "retry"}, 1, 4, entries[3].ID},
		{"source and message", LogListFilter{Actor: feishu, Keyword: " RETRY "}, 1, 3, entries[2].ID},
		{"target on second page", LogListFilter{Actor: feishu, Keyword: "retry"}, 2, 3, entries[1].ID},
		{"action on third page", LogListFilter{Actor: feishu, Keyword: "retry"}, 3, 3, entries[0].ID},
		{"literal wildcards", LogListFilter{Keyword: "%_"}, 1, 1, entries[2].ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, total, err := NewLogRepository().ListFiltered(tc.page, 1, tc.filter)
			if err != nil || total != tc.total || len(rows) != 1 || rows[0].ID != tc.entryID {
				t.Fatalf("rows = %+v, total = %d, err = %v", rows, total, err)
			}
		})
	}
}
