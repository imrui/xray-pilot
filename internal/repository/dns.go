package repository

import (
	"errors"

	"github.com/imrui/xray-pilot/internal/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func DNSConfig(tx *gorm.DB) (entity.DNSConfig, error) {
	cfg := entity.DNSConfig{ID: 1, Provider: "cloudflare"}
	err := tx.First(&cfg, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return cfg, err
}

// 调用方已在保存节点的事务内；只推进期望值，保留在途请求，供执行器回读确认。
func updateDNSIntent(tx *gorm.DB, node *entity.Node, actor string) error {
	var b entity.DNSBinding
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("node_id = ?", node.ID).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	ip, kind := entity.DNSAddress(node.IP)
	valid := entity.DNSName(node.Domain) == b.Domain && kind == b.Type
	if (valid && ip == b.DesiredIP) || b.Status == entity.DNSRebind {
		return nil
	}
	cfg, err := DNSConfig(tx)
	if err != nil {
		return err
	}
	status, message := entity.DNSPending, "IP 已保存，DNS 待同步"
	switch {
	case !valid || cfg.ZoneID != b.ZoneID:
		status, message = entity.DNSRebind, "域名、地址族或 Zone 已改变，请重新绑定"
	// DNS 跟随节点地址，节点启停不影响已绑定记录的同步。
	case !cfg.Enabled:
		status, message = entity.DNSReview, "自动同步暂停，请核对后手动同步"
	case b.Status == entity.DNSReview:
		status, message = entity.DNSReview, b.Message
	}
	return tx.Model(&b).Updates(map[string]any{"desired_ip": ip, "version": gorm.Expr("version + 1"), "status": status, "message": message, "actor": actor, "retries": 0}).Error
}
