package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/repository"
)

// setupBackupTestEnv 初始化测试用的 SQLite db 和备份目录
// 返回备份目录路径，调用方据此设置 setting
func setupBackupTestEnv(t *testing.T) (backupDir string) {
	t.Helper()
	if repository.DB != nil {
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	tmp := t.TempDir()
	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(tmp, "test.db")
	config.Global.Crypto.MasterKey = strings.Repeat("11", 32)
	if err := repository.Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		if repository.DB == nil {
			return
		}
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	backupDir = filepath.Join(tmp, "backup")

	// 通过 SettingService 写入 backup.dir，BackupService 会从这里读取
	settingSvc := NewSettingService()
	if err := settingSvc.BatchUpdate(map[string]string{
		KeyBackupDir:           backupDir,
		KeyBackupRetentionDays: "30",
	}); err != nil {
		t.Fatalf("init settings: %v", err)
	}
	return backupDir
}

// TestRunBackupCreatesValidFile 验证 RunBackup 生成的文件可被 sqlite 重新打开
// 这是 VACUUM INTO 行为的端到端保证
func TestRunBackupCreatesValidFile(t *testing.T) {
	dir := setupBackupTestEnv(t)
	svc := NewBackupService()

	file, err := svc.RunBackup()
	if err != nil {
		t.Fatalf("run backup: %v", err)
	}
	if file.Name == "" || file.Size <= 0 {
		t.Fatalf("invalid backup result: %+v", file)
	}

	// 文件应该存在于备份目录
	full := filepath.Join(dir, file.Name)
	info, err := os.Stat(full)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("backup file is empty")
	}

	// 名字符合预期格式
	if !strings.HasPrefix(file.Name, "xray-pilot-") || !strings.HasSuffix(file.Name, ".db") {
		t.Errorf("unexpected backup name: %s", file.Name)
	}
}

// TestListBackupsSortedDesc 验证列表按时间倒序
func TestListBackupsSortedDesc(t *testing.T) {
	dir := setupBackupTestEnv(t)
	_ = dir

	svc := NewBackupService()

	for i := 0; i < 3; i++ {
		if _, err := svc.RunBackup(); err != nil {
			t.Fatalf("backup %d: %v", i, err)
		}
		// VACUUM INTO 文件名使用秒级时间戳，必须显式间隔避免冲突
		time.Sleep(1100 * time.Millisecond)
	}

	backups, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(backups) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(backups))
	}
	for i := 0; i < len(backups)-1; i++ {
		if backups[i].CreatedAt.Before(backups[i+1].CreatedAt) {
			t.Errorf("not sorted desc: %v before %v", backups[i].CreatedAt, backups[i+1].CreatedAt)
		}
	}
}

// TestResolveBackupPathRejectsTraversal 验证 path traversal 防御
func TestResolveBackupPathRejectsTraversal(t *testing.T) {
	setupBackupTestEnv(t)
	svc := NewBackupService()

	bad := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"xray-pilot-../../etc.db",
		"random.db",
		"xray-pilot-2026.db",     // 格式不完整
		"xray-pilot-20260513.db", // 缺时分秒段
		"",
	}
	for _, name := range bad {
		if _, err := svc.ResolveBackupPath(name); err == nil {
			t.Errorf("should reject %q", name)
		}
	}
}

// TestCleanupRetention 验证过期备份被清理，新备份保留
func TestCleanupRetention(t *testing.T) {
	dir := setupBackupTestEnv(t)
	svc := NewBackupService()

	// 先生成一个真实备份（保留）
	current, err := svc.RunBackup()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// 再构造一个伪造的旧文件（过期，应被清理）
	oldName := "xray-pilot-20200101-000000.db"
	oldPath := filepath.Join(dir, oldName)
	if err := os.WriteFile(oldPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale: %v", err)
	}
	stale := time.Now().AddDate(0, 0, -60)
	if err := os.Chtimes(oldPath, stale, stale); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	deleted, err := svc.CleanupRetention()
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// 新备份应该还在
	if _, err := os.Stat(filepath.Join(dir, current.Name)); err != nil {
		t.Errorf("current backup was wrongly deleted: %v", err)
	}
	// 旧文件不存在了
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("stale backup not deleted")
	}
}

// TestDeleteBackup 验证显式删除接口
func TestDeleteBackup(t *testing.T) {
	dir := setupBackupTestEnv(t)
	_ = dir
	svc := NewBackupService()

	file, err := svc.RunBackup()
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	if err := svc.DeleteBackup(file.Name); err != nil {
		t.Fatalf("delete: %v", err)
	}

	backups, _ := svc.ListBackups()
	if len(backups) != 0 {
		t.Errorf("expected empty after delete, got %d", len(backups))
	}

	// 二次删除应报错
	if err := svc.DeleteBackup(file.Name); err == nil {
		t.Errorf("expected error on double delete")
	}
}
