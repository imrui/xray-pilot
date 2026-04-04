package entity

import "time"

// Group 分组实体
type Group struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"not null"`
	Description string
	Active      bool   `gorm:"default:true"`
	Nodes       []Node `gorm:"many2many:group_nodes"`
	Users       []User `gorm:"many2many:user_groups"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
