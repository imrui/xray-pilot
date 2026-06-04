package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/entity"
)

type NodeInstallTokenRepository struct{}

func NewNodeInstallTokenRepository() *NodeInstallTokenRepository {
	return &NodeInstallTokenRepository{}
}

func (r *NodeInstallTokenRepository) Create(t *entity.NodeInstallToken) error {
	return DB.Create(t).Error
}

// FindByToken 按 token 字符串查询；未找到返回 gorm.ErrRecordNotFound
func (r *NodeInstallTokenRepository) FindByToken(token string) (*entity.NodeInstallToken, error) {
	var t entity.NodeInstallToken
	err := DB.Where("token = ?", token).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// BindIP 首次调用 panel-pubkey 端点时把请求 IP 锁定到 token；
// 已绑定时不覆盖（保证锁定的稳定性）。
func (r *NodeInstallTokenRepository) BindIP(id uint, ip string) error {
	return DB.Model(&entity.NodeInstallToken{}).
		Where("id = ? AND (used_by_ip IS NULL OR used_by_ip = '')", id).
		Update("used_by_ip", ip).Error
}

// MarkUsed 标记 token 已被消费，回填 NodeID + UsedAt。
// 仅当 used_at 仍为空时才落地，避免并发重复注册。
func (r *NodeInstallTokenRepository) MarkUsed(id uint, nodeID uint, usedAt time.Time) error {
	res := DB.Model(&entity.NodeInstallToken{}).
		Where("id = ? AND used_at IS NULL", id).
		Updates(map[string]any{
			"used_at": usedAt,
			"node_id": nodeID,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 并发场景下被别人抢先标 used
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListActive 列出尚未使用且未过期的 token；供管理员界面轮询展示
func (r *NodeInstallTokenRepository) ListActive(now time.Time) ([]entity.NodeInstallToken, error) {
	var list []entity.NodeInstallToken
	err := DB.Where("used_at IS NULL AND expires_at > ?", now).
		Order("created_at desc").
		Find(&list).Error
	return list, err
}

// Delete 手动撤销
func (r *NodeInstallTokenRepository) Delete(id uint) error {
	return DB.Delete(&entity.NodeInstallToken{}, id).Error
}

// DeleteExpired 清理已过期且未使用的 token；
// 已使用的 token 保留作为审计追溯，由更长周期的日志归档处理。
func (r *NodeInstallTokenRepository) DeleteExpired(now time.Time) (int64, error) {
	res := DB.Where("used_at IS NULL AND expires_at < ?", now).
		Delete(&entity.NodeInstallToken{})
	return res.RowsAffected, res.Error
}
