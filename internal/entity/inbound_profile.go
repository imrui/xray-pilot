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
	Settings string `gorm:"type:text"`
	// Port 节点级监听端口覆盖；0 表示继承所属 InboundProfile.Port。
	// 支持同一节点上多协议错开端口（如 Reality 占 443、Trojan 改 8443）。
	Port      int
	Locked    bool `gorm:"default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

// EffectivePort 返回该节点协议实际监听端口：优先节点级覆盖，回退协议模板端口。
// 调用前需确保 Profile 已加载（Preload），否则覆盖为空时返回 0。
func (k *NodeProfileKey) EffectivePort() int {
	if k.Port > 0 {
		return k.Port
	}
	if k.Profile != nil {
		return k.Profile.Port
	}
	return 0
}
