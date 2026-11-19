package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNodeHistorySQLite(t *testing.T) {
	setupRepositoryTestDB(t)
	checkNodeHistory(t)
}

func TestCreateNodeWithoutSQLiteSequence(t *testing.T) {
	setupRepositoryTestDB(t)
	// 复制建节点所需的表结构，模拟使用普通 INTEGER PRIMARY KEY 的旧库。
	var schema []string
	tables := []string{"nodes", "deleted_nodes", "traffic_samples", "traffic_polls", "routing_bindings"}
	if err := DB.Raw("SELECT replace(sql, 'AUTOINCREMENT', '') FROM sqlite_master WHERE type = 'table' AND name IN ?", tables).Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	legacy, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: filepath.Join(t.TempDir(), "legacy.db")})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := legacy.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, ddl := range schema {
		if err := legacy.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	if legacy.Migrator().HasTable("sqlite_sequence") {
		t.Fatal("fixture unexpectedly contains sqlite_sequence")
	}
	if err := legacy.Create(&entity.TrafficPoll{NodeID: 41, PeriodEnd: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	prev := DB
	DB = legacy
	defer func() { DB = prev }()
	node := &entity.Node{Name: "legacy"}
	if err := NewNodeRepository().Create(node); err != nil || node.ID != 42 {
		t.Fatalf("create above historic ID without sqlite_sequence: id=%d err=%v", node.ID, err)
	}
}

// 测试数据及 DDL 全部位于临时 schema 与回滚事务中。
func TestNodeHistoryPostgres(t *testing.T) {
	dsn := os.Getenv("XRAY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set XRAY_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	schema := fmt.Sprintf("xpilot_history_%d", time.Now().UnixNano())
	if err := tx.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("SET LOCAL search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := autoMigrate(tx); err != nil {
		t.Fatal(err)
	}
	prev := DB
	DB = tx
	defer func() { DB = prev }()
	checkNodeHistory(t)
}

func checkNodeHistory(t *testing.T) {
	t.Helper()
	if err := DB.Create(&entity.TrafficPoll{NodeID: 99, PeriodEnd: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if DB.Dialector.Name() == "postgres" {
		if err := advanceNodeSequence(DB); err != nil {
			t.Fatal(err)
		}
	}
	r := NewNodeRepository()
	n := &entity.Node{Name: "old"}
	if err := r.Create(n); err != nil || n.ID <= 99 {
		t.Fatalf("create above history: %v %+v", err, n)
	}
	if err := DB.Model(n).Update("name", "usw2").Error; err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(n.ID); err != nil {
		t.Fatal(err)
	}
	var old entity.DeletedNode
	if err := DB.First(&old, n.ID).Error; err != nil || old.Name != "usw2" || old.DeletedAt.IsZero() {
		t.Fatalf("deleted identity: %+v %v", old, err)
	}
	if _, err := r.FindByID(n.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("deleted node still present", err)
	}
	next := &entity.Node{Name: "usw2"}
	if err := r.Create(next); err != nil || next.ID <= n.ID {
		t.Fatalf("reused history: %+v %v", next, err)
	}
}
