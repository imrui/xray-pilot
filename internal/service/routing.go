package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"gorm.io/gorm"
)

var ErrRoutingConflict = errors.New("配置版本冲突，请刷新后重试")

const DefaultRoutingPolicy = entity.DefaultRoutingPolicy

type RoutingService struct{}

func NewRoutingService() *RoutingService { return &RoutingService{} }

type EffectiveRouting struct {
	Mode       string                `json:"mode"`
	Binding    entity.RoutingBinding `json:"binding"`
	Policy     *xray.RoutingPolicy   `json:"policy"`
	Sources    map[string]string     `json:"sources"`
	Groups     []string              `json:"groups"`
	TemplateID uint                  `json:"template_id"`
	Revision   uint                  `json:"revision"`
	Warnings   []string              `json:"warnings"`
}

func routingBinding(db *gorm.DB, scope string, id uint) (entity.RoutingBinding, error) {
	b := entity.RoutingBinding{Scope: scope, TargetID: id, Mode: "inherit", UDP443Policy: "inherit", Overrides: "{}"}
	if scope == "node" {
		b.Mode = "legacy"
	}
	err := db.Where("scope = ? AND target_id = ?", scope, id).First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return b, err
}
func (s *RoutingService) Binding(scope string, id uint) (entity.RoutingBinding, error) {
	return routingBinding(repository.DB, scope, id)
}
func resolveRouting(db *gorm.DB, nodeID uint) (*EffectiveRouting, error) {
	b, err := routingBinding(db, "node", nodeID)
	if err != nil {
		return nil, err
	}
	out := &EffectiveRouting{Mode: b.Mode, Binding: b, Sources: map[string]string{}, Groups: []string{}}
	if b.Mode == "legacy" {
		return out, nil
	}
	base := b
	source := "节点模板"
	if b.Mode == "inherit" {
		var groups []entity.Group
		if err := db.Joins("JOIN group_nodes ON group_nodes.group_id = groups.id").Where("group_nodes.node_id = ? AND groups.active = ?", nodeID, true).Order("groups.id").Find(&groups).Error; err != nil {
			return nil, err
		}
		base = entity.RoutingBinding{}
		source = "全局默认"
		for _, g := range groups {
			gb, err := routingBinding(db, "group", g.ID)
			if err != nil {
				return nil, err
			}
			if gb.TemplateID == 0 {
				continue
			}
			out.Groups = append(out.Groups, g.Name)
			if base.TemplateID != 0 && (base.TemplateID != gb.TemplateID || base.Revision != gb.Revision) {
				return nil, fmt.Errorf("分组路由冲突：%s；请为节点指定模板", strings.Join(out.Groups, "、"))
			}
			base = gb
			source = "分组模板"
		}
		if base.TemplateID == 0 {
			base, err = routingBinding(db, "global", 0)
			if err != nil {
				return nil, err
			}
		}
	}
	raw := DefaultRoutingPolicy
	if base.TemplateID != 0 {
		var rev entity.RoutingTemplateRevision
		if err := db.Where("template_id = ? AND revision = ?", base.TemplateID, base.Revision).First(&rev).Error; err != nil {
			return nil, fmt.Errorf("绑定的模板修订不存在")
		}
		raw = rev.Payload
	} else {
		return nil, fmt.Errorf("全局默认或模板绑定缺失，请修复绑定")
	}
	p, err := xray.MergeRoutingPolicy(raw, b.Overrides)
	if err != nil {
		return nil, err
	}
	out.Policy = &p
	out.TemplateID, out.Revision = base.TemplateID, base.Revision
	out.Warnings = []string{"地址族策略只约束域名解析，不能转换 IP 字面量；IPv6 preference 未开放。", "自定义规则优先于域名列表；domain:google.com 包含其子域名。"}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	var overrides map[string]json.RawMessage
	_ = json.Unmarshal([]byte(b.Overrides), &overrides)
	for k := range fields {
		out.Sources[k] = source
		if v, ok := overrides[k]; ok && string(v) != "null" {
			out.Sources[k] = "节点覆盖"
		}
	}
	return out, nil
}
func (s *RoutingService) Effective(nodeID uint) (*EffectiveRouting, error) {
	var n entity.Node
	if err := repository.DB.First(&n, nodeID).Error; err != nil {
		return nil, err
	}
	return resolveRouting(repository.DB, nodeID)
}
func (s *RoutingService) SaveTemplate(id, expected uint, name, description, draft, actor string) (*entity.RoutingTemplate, error) {
	p, err := xray.DecodeRoutingPolicy(draft)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(p)
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return nil, fmt.Errorf("模板名称不能为空且不能超过 100 字节")
	}
	t := entity.RoutingTemplate{ID: id, Name: strings.TrimSpace(name), Description: description, Draft: string(data), Version: 1}
	err = repository.DB.Transaction(func(tx *gorm.DB) error {
		if id == 0 {
			return tx.Create(&t).Error
		}
		result := tx.Model(&entity.RoutingTemplate{}).Where("id = ? AND version = ?", id, expected).Updates(map[string]any{"name": t.Name, "description": description, "draft": t.Draft, "version": expected + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRoutingConflict
		}
		t.Version = expected + 1
		return nil
	})
	if err == nil {
		repository.NewLogRepository().RecordWithActor("routing_template_save", fmt.Sprint(t.ID), actor, true, "保存模板草稿", 0)
	}
	return &t, err
}
func (s *RoutingService) Publish(id, expected uint, actor string) (*entity.RoutingTemplateRevision, error) {
	var rev entity.RoutingTemplateRevision
	err := repository.DB.Transaction(func(tx *gorm.DB) error {
		var t entity.RoutingTemplate
		if err := tx.First(&t, id).Error; err != nil {
			return err
		}
		if t.Version != expected {
			return ErrRoutingConflict
		}
		if _, err := xray.DecodeRoutingPolicy(t.Draft); err != nil {
			return err
		}
		r := tx.Model(&entity.RoutingTemplate{}).Where("id = ? AND version = ?", id, expected).Update("version", expected+1)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrRoutingConflict
		}
		var max uint
		if err := tx.Model(&entity.RoutingTemplateRevision{}).Where("template_id = ?", id).Select("COALESCE(MAX(revision),0)").Scan(&max).Error; err != nil {
			return err
		}
		rev = entity.RoutingTemplateRevision{TemplateID: id, Revision: max + 1, SchemaVersion: 1, Payload: t.Draft, Actor: actor}
		return tx.Create(&rev).Error
	})
	if err == nil {
		repository.NewLogRepository().RecordWithActor("routing_publish", fmt.Sprint(id), actor, true, fmt.Sprintf("发布修订 %d", rev.Revision), 0)
	}
	return &rev, err
}
func (s *RoutingService) DeleteTemplate(id uint) error {
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		// Published revisions are audit evidence; archive rather than delete their template.
		var count int64
		if err := tx.Model(&entity.RoutingTemplateRevision{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("已发布模板保留历史，不允许删除；请停止绑定")
		}
		if err := tx.Model(&entity.RoutingBinding{}).Where("template_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("模板仍被引用")
		}
		return tx.Delete(&entity.RoutingTemplate{}, id).Error
	})
}
func (s *RoutingService) SaveBinding(b entity.RoutingBinding, expected uint, actor string, super bool) error {
	if b.Scope != "global" && b.Scope != "group" && b.Scope != "node" && b.Scope != "user" {
		return fmt.Errorf("无效配置范围")
	}
	if (b.Scope == "global" || b.Scope == "group") && !super {
		return fmt.Errorf("共享绑定仅超级管理员可修改")
	}
	if b.Scope == "global" && b.TargetID != 0 {
		return fmt.Errorf("全局目标必须为 0")
	}
	if b.UDP443Policy == "" {
		b.UDP443Policy = "inherit"
	}
	if b.UDP443Policy != "inherit" && b.UDP443Policy != "block" && b.UDP443Policy != "allow" {
		return fmt.Errorf("UDP 443 策略无效")
	}
	if b.Mode != "legacy" && b.Mode != "inherit" && b.Mode != "template" {
		return fmt.Errorf("路由模式无效")
	}
	if b.Scope != "node" && b.Mode == "legacy" {
		return fmt.Errorf("legacy 仅适用于节点")
	}
	if b.Scope == "user" && (b.TemplateID != 0 || b.Mode != "inherit") {
		return fmt.Errorf("用户仅支持订阅策略")
	}
	if b.ConfigPath != "" && (b.Scope != "node" || !regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`).MatchString(b.ConfigPath) || path.Clean(b.ConfigPath) != b.ConfigPath || path.Ext(b.ConfigPath) != ".json") {
		return fmt.Errorf("配置路径须为绝对 JSON 文件路径，不允许上级目录或特殊字符")
	}
	if b.Mode != "template" && (b.TemplateID != 0 || b.Revision != 0) {
		return fmt.Errorf("继承或 legacy 模式不能同时指定模板修订")
	}
	if b.Scope == "global" && b.Mode != "template" {
		return fmt.Errorf("全局默认必须绑定已发布模板")
	}
	if b.Scope == "group" && b.UDP443Policy != "inherit" {
		return fmt.Errorf("分组不提供订阅 UDP 443 策略")
	}
	if b.Overrides == "" {
		b.Overrides = "{}"
	}
	// Validate shape independently with IPv6 allowed; the resolved base is validated below.
	validationBase := strings.Replace(DefaultRoutingPolicy, "false", "true", 1)
	if _, err := xray.MergeRoutingPolicy(validationBase, b.Overrides); err != nil {
		return err
	}
	if b.Mode == "legacy" && b.Overrides != "{}" {
		return fmt.Errorf("legacy 模式不接受字段覆盖；请先选择继承或模板")
	}
	if b.Scope != "node" && b.Overrides != "{}" {
		return fmt.Errorf("只有节点允许字段覆盖")
	}
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		if b.Scope != "global" {
			table := map[string]string{"node": "nodes", "group": "groups", "user": "users"}[b.Scope]
			var n int64
			if err := tx.Table(table).Where("id = ?", b.TargetID).Count(&n).Error; err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("配置对象不存在")
			}
		}
		old, err := routingBinding(tx, b.Scope, b.TargetID)
		if err != nil {
			return err
		}
		if old.Version != expected {
			return ErrRoutingConflict
		}
		if !super {
			if old.ConfigPath != b.ConfigPath {
				return fmt.Errorf("配置路径仅超级管理员可修改")
			}
			var prev, next map[string]json.RawMessage
			_ = json.Unmarshal([]byte(old.Overrides), &prev)
			if json.Unmarshal([]byte(b.Overrides), &next) != nil {
				return fmt.Errorf("覆盖必须是 JSON 对象")
			}
			if string(prev["custom_rules"]) != string(next["custom_rules"]) {
				return fmt.Errorf("高级规则仅超级管理员可修改")
			}
		}
		if b.TemplateID > 0 {
			var rev entity.RoutingTemplateRevision
			if err := tx.Where("template_id = ? AND revision = ?", b.TemplateID, b.Revision).First(&rev).Error; err != nil {
				return fmt.Errorf("模板修订不存在")
			}
		}
		if b.Mode == "template" && b.TemplateID == 0 {
			return fmt.Errorf("请选择已发布模板")
		}
		b.Version = expected + 1
		b.UpdatedAt = time.Now()
		if expected == 0 {
			if err := tx.Create(&b).Error; err != nil {
				return err
			}
		} else {
			r := tx.Model(&entity.RoutingBinding{}).Where("scope = ? AND target_id = ? AND version = ?", b.Scope, b.TargetID, expected).Select("*").Updates(&b)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return ErrRoutingConflict
			}
		}
		var nodes []entity.Node
		if old.Mode == b.Mode && old.TemplateID == b.TemplateID && old.Revision == b.Revision && old.Overrides == b.Overrides && old.ConfigPath == b.ConfigPath {
			return tx.Create(&entity.SyncLog{Actor: actor, Action: "routing_subscription_policy", Target: fmt.Sprintf("%s:%d", b.Scope, b.TargetID), Success: true, Message: fmt.Sprintf("订阅策略版本 %d", b.Version)}).Error
		}
		q := tx.Model(&entity.Node{})
		if b.Scope == "node" {
			q = q.Where("id = ?", b.TargetID)
		}
		if err := q.Find(&nodes).Error; err != nil {
			return err
		}
		for _, n := range nodes {
			if b.Scope == "user" {
				continue
			}
			effective, err := resolveRouting(tx, n.ID)
			if err != nil {
				return err
			}
			if effective.Mode != "legacy" || b.Scope == "node" {
				if err := repository.InvalidateConfigs(tx, []uint{n.ID}); err != nil {
					return err
				}
				if err := tx.Model(&entity.Node{}).Where("id = ?", n.ID).Update("sync_status", entity.SyncStatusDrifted).Error; err != nil {
					return err
				}
			}
		}
		return tx.Create(&entity.SyncLog{Actor: actor, Action: "routing_binding", Target: fmt.Sprintf("%s:%d", b.Scope, b.TargetID), Success: true, Message: fmt.Sprintf("绑定版本 %d", b.Version)}).Error
	})
}
func (s *RoutingService) UDP443Policy(node, user uint) string {
	for _, pair := range []struct {
		scope string
		id    uint
	}{{"user", user}, {"node", node}, {"global", 0}} {
		b, err := routingBinding(repository.DB, pair.scope, pair.id)
		if err == nil && (b.UDP443Policy == "allow" || b.UDP443Policy == "block") {
			return b.UDP443Policy
		}
	}
	return "block"
}
