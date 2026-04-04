package repository

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return fmt.Errorf("连接数据库失败: %w", err)
	}

	if err := autoMigrate(db); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}
	if err := migrateLegacyUserGroups(db); err != nil {
		return fmt.Errorf("迁移用户分组关系失败: %w", err)
	}

	DB = db
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
