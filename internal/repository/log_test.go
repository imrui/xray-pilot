package repository

import (
	"github.com/imrui/xray-pilot/internal/entity"
	"testing"
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
