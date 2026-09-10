package service

import (
	"testing"
	"time"
)

func TestRetentionCutoff(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, ok := retentionCutoff(0, now); ok {
		t.Error("days=0 应表示不清理")
	}
	if _, ok := retentionCutoff(-1, now); ok {
		t.Error("负数应表示不清理")
	}
	cutoff, ok := retentionCutoff(90, now)
	if !ok {
		t.Fatal("days=90 应清理")
	}
	if want := now.AddDate(0, 0, -90); !cutoff.Equal(want) {
		t.Errorf("cutoff = %v, want %v", cutoff, want)
	}
}
