package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/repository"
)

// BackupService SQLite 数据库备份服务
//
// 仅支持 SQLite，通过 `VACUUM INTO` 生成完整快照（SQLite 3.27+，modernc.org/sqlite 内置支持）
// 相比直接 file copy 的优点：自动处理 WAL/journal、不复制 unused space、对读写不阻塞
//
// Postgres 场景：用户需自行通过 pg_dump 备份，本服务在非 SQLite 时返回明确错误
type BackupService struct {
	settingSvc *SettingService
	logRepo    *repository.LogRepository
}

func NewBackupService() *BackupService {
	return &BackupService{
		settingSvc: NewSettingService(),
		logRepo:    repository.NewLogRepository(),
	}
}

// BackupFile 单个备份文件元信息
type BackupFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// backupFileNameRe 备份文件名格式（含 UTC 时间戳），用于 ListBackups 过滤
// 也用于 DeleteBackup / DownloadBackup 时校验 name 参数防止 path traversal
var backupFileNameRe = regexp.MustCompile(`^xray-pilot-\d{8}-\d{6}\.db$`)

// backupDir 返回备份目录的绝对路径，并保证目录存在
func (s *BackupService) backupDir() (string, error) {
	dir := strings.TrimSpace(s.settingSvc.Get(KeyBackupDir))
	if dir == "" {
		dir = "data/backup"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve backup dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}
	return abs, nil
}

// RunBackup 执行一次完整备份，返回生成的文件元信息
// 仅当 driver == sqlite 时有效；其他 driver 返回 errors.New("仅支持 SQLite")
func (s *BackupService) RunBackup() (*BackupFile, error) {
	if config.Global.Database.Driver != "sqlite" && config.Global.Database.Driver != "" {
		return nil, errors.New("仅支持 SQLite 数据库的备份；Postgres 请使用 pg_dump")
	}

	dir, err := s.backupDir()
	if err != nil {
		return nil, err
	}
	name := fmt.Sprintf("xray-pilot-%s.db", time.Now().UTC().Format("20060102-150405"))
	full := filepath.Join(dir, name)

	// 防御性：理论上时间戳不会与既有文件冲突；但若发生，拒绝覆盖以保护既有数据
	if _, err := os.Stat(full); err == nil {
		return nil, fmt.Errorf("备份文件已存在: %s", name)
	}

	// VACUUM INTO 不能用参数化（SQLite 限制），手动拼接但路径已用 filepath 规范化
	// 同时把单引号转义防御异常路径
	escaped := strings.ReplaceAll(full, "'", "''")
	if err := repository.DB.Exec(fmt.Sprintf("VACUUM INTO '%s'", escaped)).Error; err != nil {
		// 若文件已部分写入需清理
		_ = os.Remove(full)
		return nil, fmt.Errorf("VACUUM INTO 失败: %w", err)
	}

	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("读取备份文件状态失败: %w", err)
	}

	zap.L().Named("backup").Info("数据库备份完成",
		zap.String("file", name),
		zap.Int64("bytes", info.Size()),
	)

	return &BackupFile{
		Name:      name,
		Size:      info.Size(),
		CreatedAt: info.ModTime(),
	}, nil
}

// ListBackups 返回备份目录下的所有备份文件，按创建时间倒序
func (s *BackupService) ListBackups() ([]BackupFile, error) {
	dir, err := s.backupDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取备份目录失败: %w", err)
	}
	out := make([]BackupFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !backupFileNameRe.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupFile{
			Name:      e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// ResolveBackupPath 校验 name 是合法的备份文件名后返回其绝对路径
// 用于下载/删除接口防止 path traversal（如 "../../etc/passwd"）
func (s *BackupService) ResolveBackupPath(name string) (string, error) {
	if !backupFileNameRe.MatchString(name) {
		return "", errors.New("非法的备份文件名")
	}
	dir, err := s.backupDir()
	if err != nil {
		return "", err
	}
	full := filepath.Join(dir, name)
	// 双保险：清理后路径必须仍在 dir 下（防御符号链接等）
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(dir)+string(filepath.Separator)) {
		return "", errors.New("非法的备份路径")
	}
	if _, err := os.Stat(full); err != nil {
		return "", fmt.Errorf("备份文件不存在: %w", err)
	}
	return full, nil
}

// DeleteBackup 删除指定备份文件
func (s *BackupService) DeleteBackup(name string) error {
	full, err := s.ResolveBackupPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("删除备份失败: %w", err)
	}
	return nil
}

// CleanupRetention 清理超过 retention 天的备份，返回删除数量
func (s *BackupService) CleanupRetention() (int, error) {
	days := s.settingSvc.GetInt(KeyBackupRetentionDays)
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	backups, err := s.ListBackups()
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, b := range backups {
		if b.CreatedAt.Before(cutoff) {
			if err := s.DeleteBackup(b.Name); err != nil {
				zap.L().Named("backup").Warn("清理过期备份失败", zap.String("file", b.Name), zap.Error(err))
				continue
			}
			deleted++
		}
	}
	return deleted, nil
}

// RunOnce 给 scheduler 使用：备份一次 + 清理过期，统一记日志
func (s *BackupService) RunOnce() {
	start := time.Now()
	file, err := s.RunBackup()
	if err != nil {
		zap.L().Named("backup").Error("备份失败", zap.Error(err))
		s.logRepo.Record("backup", "all", false, err.Error(), time.Since(start).Milliseconds())
		return
	}
	deleted, _ := s.CleanupRetention()
	msg := fmt.Sprintf("备份 %s (%d 字节)；清理过期 %d 个", file.Name, file.Size, deleted)
	s.logRepo.Record("backup", file.Name, true, msg, time.Since(start).Milliseconds())
}
