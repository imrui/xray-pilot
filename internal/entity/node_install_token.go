package entity

import "time"

// NodeInstallToken 节点一次性安装 token
// 由管理员在面板侧生成，用于 scripts/node-bootstrap.sh 自动接入流程：
//   - 脚本拉取 panel SSH 公钥并写入节点 authorized_keys
//   - 脚本完成装机后回调 register 接口提交自检结果
//
// 安全约束：
//   - 一次性（UsedAt 落地后不可复用）
//   - 默认 10 分钟 TTL（ExpiresAt）
//   - IP 绑定（首次调用 panel-pubkey 时记录 UsedByIP，后续调用必须匹配）
type NodeInstallToken struct {
	ID        uint   `gorm:"primaryKey"`
	Token     string `gorm:"size:64;uniqueIndex;not null"`

	// NodeMeta 是创建 token 时由管理员填的节点元数据 JSON：
	//   {name, region, owner, remark, ssh_user, ssh_port}
	// 脚本注册成功后用这里的字段创建 Node 记录。
	NodeMeta string `gorm:"type:text;not null"`

	CreatedAt time.Time
	ExpiresAt time.Time `gorm:"index"`

	// UsedAt 在脚本调用 register 接口成功后落地。落地后该 token 进入 410 Gone 状态。
	UsedAt *time.Time `gorm:"index"`

	// UsedByIP 首次调用 panel-pubkey 端点时记录的来源 IP。
	// 后续 panel-pubkey / register 调用必须匹配此 IP，避免 token 截图泄露后异地滥用。
	UsedByIP string `gorm:"size:64"`

	// NodeID 注册成功后回填，便于审计追溯。
	NodeID *uint `gorm:"index"`

	// CreatedByAdmin 创建该 token 的管理员标识（v0.5.0 多管理员落地前先存 username）
	CreatedByAdmin string `gorm:"size:64"`
}

// IsUsed 返回 token 是否已经被使用过
func (t *NodeInstallToken) IsUsed() bool {
	return t.UsedAt != nil
}

// IsExpired 返回 token 是否已经过期
func (t *NodeInstallToken) IsExpired(now time.Time) bool {
	return !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt)
}
