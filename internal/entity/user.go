package entity

import "time"

// User 用户实体
type User struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"not null"`
	RealName     string
	Department   string
	UUID         string `gorm:"uniqueIndex;not null"`
	Token        string `gorm:"uniqueIndex;not null"`
	GroupID      *uint
	Group        *Group `gorm:"foreignKey:GroupID"`
	Active       bool   `gorm:"default:true"`
	Remark       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
