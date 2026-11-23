package repository

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	_ "modernc.org/sqlite"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/entity"
)

var DB *gorm.DB

// Connect 初始化数据库连接，支持 sqlite 和 postgres
func Connect() error {
	cfg := config.Global.Database
	var dialector gorm.Dialector

	switch cfg.Driver {
	case "postgres":
		dialector = postgres.Open(cfg.DSN)
	default: // sqlite
		dialector = sqlite.Dialector{
			DriverName: "sqlite",
			DSN:        cfg.DSN,
		}
	}

	// IgnoreRecordNotFoundError：ErrRecordNotFound 在业务上几乎都是预期路径
	// （如 SettingService.Get 拿不到 key 时回退默认、install 检查同名节点等），
	// 默认 GORM logger 把它当 warning 打日志会让控制台被噪音淹没。
	dbLogger := gormlogger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
			ParameterizedQueries:      true,
		},
	)
	db, err := gorm.Open(dialector, &gorm.Config{Logger: dbLogger})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	if cfg.Driver != "postgres" {
		// SQLite 是单写者模型：前端批量操作（批量删除/切换节点等）并发发起
		// 多个写请求时，连接池里多条连接同时开写事务会直接报
		// "database is locked (5) (SQLITE_BUSY)"。收敛为单连接把所有语句
		// 串行化，从根上消除进程内锁冲突；busy_timeout 兜底外部进程
		// （如手动 sqlite3 CLI）持锁的场景。
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("获取底层数据库连接失败: %w", err)
		}
		sqlDB.SetMaxOpenConns(1)
		if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
			return fmt.Errorf("设置 busy_timeout 失败: %w", err)
		}
	}

	if err := autoMigrate(db); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := migrateRoutingDefaults(db); err != nil {
		return fmt.Errorf("迁移路由默认值失败: %w", err)
	}
	if err := migrateLegacyUserGroups(db); err != nil {
		return fmt.Errorf("迁移用户分组关系失败: %w", err)
	}
	if err := cleanupOrphanRelations(db); err != nil {
		// 清理失败不阻断启动，仅日志（业务上不影响），但下次仍会尝试
		fmt.Printf("[warn] 清理孤儿关联表失败: %v\n", err)
	}

	DB = db
	return nil
}

// cleanupOrphanRelations 清理多对多 / 关联表中指向不存在主体的孤儿行。
//
// 历史背景：v0.4.3 之前 NodeRepository.Delete 是 hard delete 但未级联清理
// 中间表，叠加 SQLite ID 复用机制后会让"删节点 → 一键接入新节点"撞到旧
// 节点 ID 的 orphan 关联（继承旧分组 / 协议绑定 / 用户）。本函数在每次
// 启动后跑一次兜底，清掉所有方向的孤儿，确保新建实体不会撞库。
//
// 用 Exec 走 raw SQL 而非 GORM 抽象——中间表 group_nodes 没有 entity 定义，
// 直接 SQL 最直白；性能也好（避免 GORM 反射开销）。
func cleanupOrphanRelations(db *gorm.DB) error {
	statements := []string{
		"DELETE FROM group_nodes WHERE node_id NOT IN (SELECT id FROM nodes)",
		"DELETE FROM group_nodes WHERE group_id NOT IN (SELECT id FROM groups)",
		"DELETE FROM node_profile_keys WHERE node_id NOT IN (SELECT id FROM nodes)",
		"DELETE FROM node_profile_keys WHERE profile_id NOT IN (SELECT id FROM inbound_profiles)",
		"DELETE FROM user_groups WHERE user_id NOT IN (SELECT id FROM users)",
		"DELETE FROM user_groups WHERE group_id NOT IN (SELECT id FROM groups)",
		"DELETE FROM node_alert_states WHERE node_id <> 0 AND node_id NOT IN (SELECT id FROM nodes)",
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("执行 %q 失败: %w", stmt, err)
		}
	}
	return nil
}

func autoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&entity.RoutingTemplate{}, &entity.RoutingTemplateRevision{}, &entity.RoutingBinding{}, &entity.ConfigDeployment{}, &entity.NodeConfigState{},
		&entity.User{},
		&entity.UserGroup{}, &entity.UserExpiryTarget{},
		&entity.Group{},
		&entity.Node{},
		&entity.SyncLog{},
		&entity.InboundProfile{},
		&entity.NodeProfileKey{},
		&entity.SystemSetting{},
		&entity.DNSConfig{}, &entity.DNSBinding{}, &entity.DeletedNode{}, &entity.TrafficSample{},
		&entity.TrafficPoll{},
		&entity.UserTrafficTotal{},
		&entity.NodeInstallToken{},
		&entity.AdminUser{},
		&entity.NodeAlertState{},
	); err != nil {
		return err
	}
	// 旧版 review 同时表示远端冲突和暂停后的核对。只读重查这些旧记录，由执行器区分两者。
	if err := db.Model(&entity.DNSBinding{}).
		Where("status = ? AND (message LIKE ? OR message LIKE ?)", entity.DNSReview, "远端当前值 %，请核对绑定", "DNS 记录与绑定不一致，请重新核对%").
		Updates(map[string]any{"status": entity.DNSChecking, "message": "正在只读核对远端记录", "version": gorm.Expr("version + 1"), "retries": 0, "retry_at": nil}).Error; err != nil {
		return err
	}
	if db.Dialector.Name() == "postgres" {
		return advanceNodeSequence(db)
	}
	if db.Dialector.Name() == "sqlite" {
		// Expression indexes keep normalized legacy timestamp range queries indexed.
		for _, table := range []string{"traffic_samples", "traffic_polls"} {
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_" + table + "_instant ON " + table + " (" + sqliteTrafficInstant + ")").Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateLegacyUserGroups(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// 已编辑过的多分组关系优先。首次迁移后消费旧列，避免重启复活已移除的分组。
		if err := tx.Exec(`INSERT INTO user_groups (user_id, group_id)
			SELECT u.id, u.group_id FROM users u JOIN groups g ON g.id = u.group_id
			WHERE u.group_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM user_groups ug JOIN groups current_group ON current_group.id = ug.group_id WHERE ug.user_id = u.id)
			ON CONFLICT (user_id, group_id) DO NOTHING`).Error; err != nil {
			return err
		}
		return tx.Model(&entity.User{}).Where("group_id IS NOT NULL").UpdateColumn("group_id", nil).Error
	})
}
