package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 配置、解绑与重新绑定等待已派发的请求结束。不同记录的执行器可并行。
var dnsControls sync.RWMutex
var zoneIDPattern = regexp.MustCompile("^[a-fA-F0-9]{32}$")

type DNSValidationError string

func (e DNSValidationError) Error() string { return string(e) }

const errPreviewChanged DNSValidationError = "节点、Zone 或远端记录已变化，请重新预览"

type DNSSettings struct {
	Enabled    bool   `json:"enabled"`
	Provider   string `json:"provider"`
	ZoneID     string `json:"zone_id"`
	Configured bool   `json:"configured"`
}

type DNSSettingsInput struct {
	Enabled  bool   `json:"enabled"`
	Provider string `json:"provider"`
	ZoneID   string `json:"zone_id"`
	Token    string `json:"token"`
}

func (s *DNSService) Settings() (DNSSettings, error) {
	cfg, err := repository.DNSConfig(repository.DB)
	return DNSSettings{cfg.Enabled, cfg.Provider, cfg.ZoneID, cfg.TokenCipher != ""}, err
}

func dnsAudit(tx *gorm.DB, node uint, actor string, ok bool, message string) error {
	target := "cloudflare"
	if node != 0 {
		target = fmt.Sprintf("node:%d", node)
	}
	return repository.NewLogRepository().WithTx(tx).RecordWithActor("dns_sync", target, actor, ok, message, 0)
}

func (s *DNSService) SaveSettings(in DNSSettingsInput, clear bool, actor string) error {
	defer wakeDNS()
	dnsControls.Lock()
	defer dnsControls.Unlock()
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		old, err := repository.DNSConfig(tx)
		if err != nil {
			return err
		}
		cfg := old
		if clear {
			cfg.TokenCipher = ""
			cfg.Enabled = false
		} else {
			cfg.ZoneID = strings.ToLower(strings.TrimSpace(in.ZoneID))
			cfg.Enabled = in.Enabled
			if in.Provider != "cloudflare" || (cfg.ZoneID != "" && !zoneIDPattern.MatchString(cfg.ZoneID)) {
				return DNSValidationError("请选择 Cloudflare 并填写有效的 Zone ID")
			}
			token := strings.TrimSpace(in.Token)
			if token != "" {
				previous, _ := crypto.Decrypt(old.TokenCipher)
				if previous != token {
					cfg.TokenCipher, err = crypto.Encrypt(token)
					if err != nil {
						return err
					}
				}
			}
			if cfg.Enabled && (cfg.TokenCipher == "" || cfg.ZoneID == "") {
				return DNSValidationError("启用 DNS 同步前请填写 Zone ID 和 API Token")
			}
		}
		if err := tx.Save(&cfg).Error; err != nil {
			return err
		}
		q := tx.Model(&entity.DNSBinding{}).Where("status <> ?", entity.DNSRebind)
		switch {
		case cfg.ZoneID != old.ZoneID:
			err = q.Updates(map[string]any{"status": entity.DNSRebind, "message": "Zone 已变更，请重新绑定", "version": gorm.Expr("version + 1")}).Error
		case cfg.Enabled != old.Enabled:
			status, message := entity.DNSReview, "DNS 同步已关闭"
			if cfg.Enabled {
				status, message = entity.DNSChecking, "正在只读核对远端记录"
			}
			err = q.Updates(map[string]any{"status": status, "message": message, "version": gorm.Expr("version + 1"), "retry_at": nil, "retries": 0, "actor": actor}).Error
		}
		if err != nil {
			return err
		}
		return dnsAudit(tx, 0, actor, true, "已保存 DNS 设置")
	})
}

func (s *DNSService) TestConnection(ctx context.Context) error {
	cfg, err := repository.DNSConfig(repository.DB)
	if err != nil {
		return err
	}
	if cfg.ZoneID == "" || cfg.TokenCipher == "" {
		return DNSValidationError("请先保存 Zone ID 和 Token")
	}
	var list []dnsRecord
	return s.request(ctx, cfg, "GET", "?per_page=1", nil, &list)
}

func (s *DNSService) Bindings() ([]entity.DNSBinding, error) {
	var bindings []entity.DNSBinding
	err := repository.DB.Order("node_id").Find(&bindings).Error
	return bindings, err
}

func (s *DNSService) Binding(node uint) (*entity.DNSBinding, error) {
	var b entity.DNSBinding
	err := repository.DB.Where("node_id = ?", node).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &b, err
}

type DNSPreview struct {
	Domain   string `json:"domain"`
	Type     string `json:"type"`
	RecordID string `json:"record_id"`
	ZoneID   string `json:"zone_id"`
	RemoteIP string `json:"remote_ip"`
	NodeIP   string `json:"node_ip"`
}

func (s *DNSService) Preview(ctx context.Context, id uint) (DNSPreview, error) {
	var p DNSPreview
	cfg, err := repository.DNSConfig(repository.DB)
	if err != nil {
		return p, err
	}
	if !cfg.Enabled {
		return p, DNSValidationError("请先启用 DNS 同步")
	}
	n, err := repository.NewNodeRepository().FindByID(id)
	if err != nil {
		return p, err
	}
	ip, kind := entity.DNSAddress(n.IP)
	name := entity.DNSName(n.Domain)
	if ip == "" || name == "" {
		return p, DNSValidationError("请先保存节点 IP 和完整连接域名")
	}
	row, err := s.lookup(ctx, cfg, name, kind)
	if err != nil {
		return p, err
	}
	return DNSPreview{name, kind, row.ID, cfg.ZoneID, row.Content, ip}, nil
}

func (s *DNSService) Bind(ctx context.Context, id uint, expected DNSPreview, actor string) error {
	dnsControls.Lock()
	defer dnsControls.Unlock()
	actual, err := s.Preview(ctx, id)
	if err != nil {
		return err
	}
	if actual != expected {
		return errPreviewChanged
	}
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		var n entity.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&n, id).Error; err != nil {
			return err
		}
		ip, _ := entity.DNSAddress(n.IP)
		if ip != actual.NodeIP || entity.DNSName(n.Domain) != actual.Domain {
			return errPreviewChanged
		}
		if err := tx.Where("node_id = ?", id).Delete(&entity.DNSBinding{}).Error; err != nil {
			return err
		}
		b := entity.DNSBinding{NodeID: id, ZoneID: actual.ZoneID, RecordID: actual.RecordID, Domain: actual.Domain, Type: actual.Type, ConfirmedIP: actual.RemoteIP, DesiredIP: ip, Status: entity.DNSSynced, Version: 1, Actor: actor}
		if ip != actual.RemoteIP {
			b.Status = entity.DNSReview
			b.Message = "远端与节点 IP 不同，可手动同步"
		}
		insert := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "zone_id"}, {Name: "record_id"}}, DoNothing: true,
		}).Create(&b)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			return DNSValidationError("这条 DNS 记录已被其他节点绑定")
		}
		return dnsAudit(tx, id, actor, true, "绑定 "+b.Domain+"，当前记录 "+b.ConfirmedIP+"，节点 "+ip)
	})
}

func (s *DNSService) Unbind(id uint, actor string) error {
	dnsControls.Lock()
	defer dnsControls.Unlock()
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", id).Delete(&entity.DNSBinding{}).Error; err != nil {
			return err
		}
		return dnsAudit(tx, id, actor, true, "已解除 DNS 绑定")
	})
}

func (s *DNSService) Retry(id uint, actor string) error {
	defer wakeDNS()
	dnsControls.RLock()
	defer dnsControls.RUnlock()
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		cfg, err := repository.DNSConfig(tx)
		if err != nil {
			return err
		}
		var n entity.Node
		if err := tx.First(&n, id).Error; err != nil {
			return err
		}
		if !cfg.Enabled || !n.Active {
			return DNSValidationError("请先启用 DNS 同步和节点")
		}
		result := tx.Model(&entity.DNSBinding{}).Where("node_id = ? AND zone_id = ? AND status IN ?", id, cfg.ZoneID, []string{entity.DNSReview, entity.DNSFailed, entity.DNSSynced}).Updates(map[string]any{"status": entity.DNSPending, "actor": actor, "version": gorm.Expr("version + 1"), "retry_at": nil, "retries": 0, "message": "等待同步"})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return DNSValidationError("当前绑定无法重试，请刷新或重新绑定")
		}
		return dnsAudit(tx, id, actor, true, "手动请求 DNS 同步")
	})
}
