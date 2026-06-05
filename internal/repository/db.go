package repository

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		},
	)
	db, err := gorm.Open(dialector, &gorm.Config{Logger: dbLogger})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	if err := autoMigrate(db); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
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
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("执行 %q 失败: %w", stmt, err)
		}
	}
	return nil
}

func autoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&entity.User{},
		&entity.UserGroup{},
		&entity.Group{},
		&entity.Node{},
		&entity.SyncLog{},
		&entity.InboundProfile{},
		&entity.NodeProfileKey{},
		&entity.SystemSetting{},
		&entity.TrafficSample{},
		&entity.UserTrafficTotal{},
		&entity.NodeInstallToken{},
	)
}

func migrateLegacyUserGroups(db *gorm.DB) error {
	var legacyUsers []struct {
		ID      uint
		GroupID *uint `gorm:"column:group_id"`
	}

	if err := db.Table("users").
		Select("id, group_id").
		Where("group_id IS NOT NULL").
		Find(&legacyUsers).Error; err != nil {
		return err
	}

	if len(legacyUsers) == 0 {
		return nil
	}

	links := make([]entity.UserGroup, 0, len(legacyUsers))
	for _, user := range legacyUsers {
		if user.GroupID == nil {
			continue
		}
		links = append(links, entity.UserGroup{
			UserID:  user.ID,
			GroupID: *user.GroupID,
		})
	}
	if len(links) == 0 {
		return nil
	}

	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&links).Error
}
