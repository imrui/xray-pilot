package repository

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 两个真实连接交错保存用户备注及分组成员，验证 READ COMMITTED 下的授权快照。
func TestPostgresConcurrentExpiryAndGroupChange(t *testing.T) {
	dsn := os.Getenv("XRAY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set XRAY_TEST_POSTGRES_DSN for PostgreSQL integration")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin := stdlib.OpenDB(*cfg)
	defer admin.Close()
	schema := fmt.Sprintf("xpilot_auth_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg.RuntimeParams["search_path"] = schema
	pool := stdlib.OpenDB(*cfg)
	defer pool.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: pool}))
	if err != nil {
		t.Fatal(err)
	}
	if err = autoMigrate(db); err != nil {
		t.Fatal(err)
	}
	prev := DB
	DB = db
	defer func() { DB = prev }()
	n := entity.Node{Name: "pg-auth", Active: true}
	if err = db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	p := entity.InboundProfile{Name: "test", Protocol: types.ProtocolTrojan, Active: true}
	if err = db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&entity.NodeProfileKey{NodeID: n.ID, ProfileID: p.ID}).Error; err != nil {
		t.Fatal(err)
	}
	g := entity.Group{Name: "pg-group", Active: true, Nodes: []entity.Node{n}}
	if err = db.Omit("Nodes.*").Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	past := now.Add(-time.Minute)
	u := entity.User{Username: "pg-user", UUID: "uuid", Token: "token", Active: true, ExpiresAt: &past, Groups: []entity.Group{g}}
	if err = db.Omit("Groups.*").Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	var workers sync.WaitGroup
	unblock := sync.OnceFunc(func() { close(resume) })
	defer func() { unblock(); workers.Wait() }()
	result := make(chan error, 2)
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := NewUserRepository().ChangeAccess(u.ID, func(tx *gorm.DB, _ *entity.User) error {
			close(entered)
			<-resume
			return tx.Model(&entity.User{}).Where("id = ?", u.ID).Update("remark", "changed").Error
		})
		result <- err
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("authorization edit failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("authorization edit did not start")
	}
	workers.Add(1)
	go func() { defer workers.Done(); result <- NewGroupRepository().ReplaceNodes(&g, nil) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting bool
		err := admin.QueryRow("SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype='advisory' AND objid=910302 AND NOT granted AND database=(SELECT oid FROM pg_database WHERE datname=current_database()))").Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("group transaction bypassed authorization lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	unblock()
	for range 2 {
		if err = <-result; err != nil {
			t.Fatal(err)
		}
	}
	targets, names, err := PrepareExpirySweep(now)
	if err != nil || len(names) != 0 || len(targets) != 1 || targets[0].NodeID != n.ID {
		t.Fatalf("lost old node: %v %v %v", targets, names, err)
	}
	names, err = ConfirmExpiryTargets([]string{targets[0].Ticket}, now)
	if err != nil || len(names) != 1 || names[0] != u.Username {
		t.Fatalf("wrong receipt: %v %v", names, err)
	}
}
