package entity

import "time"

// InboundProfile 协议接入配置模板（跨节点共享协议参数）
type InboundProfile struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"not null"`
	Protocol string `gorm:"not null"` // vless-reality | vless-ws-tls | trojan | hysteria2
	Port     int    `gorm:"not null;default:443"`
	// Settings JSON 序列化的协议共享参数（SNI、路径等），具体结构见 pkg/types
	Settings string `gorm:"type:text"`
	Active   bool   `gorm:"default:true"`
	Remark   string
	// NodeKeys 关联的节点密钥（通过 NodeProfileKey 中间表）
	NodeKeys  []NodeProfileKey `gorm:"foreignKey:ProfileID"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NodeProfileKey 节点维度的协议密钥材料（每个节点对每个协议都有独立的密钥/证书）
type NodeProfileKey struct {
	ID        uint            `gorm:"primaryKey"`
	NodeID    uint            `gorm:"not null;uniqueIndex:idx_node_profile"`
	ProfileID uint            `gorm:"not null;uniqueIndex:idx_node_profile"`
	Profile   *InboundProfile `gorm:"foreignKey:ProfileID"`
	// Settings JSON 序列化的节点密钥材料，具体结构见 pkg/types（RealityKeyMaterial / TLSCertMaterial）
	Settings  string `gorm:"type:text"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
