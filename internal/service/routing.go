package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/crypto"
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
	b := entity.RoutingBinding{Scope: scope, TargetID: id, Mode: "inherit", Overrides: "{}"}
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
	if scope != "global" && scope != "group" && scope != "node" {
		return entity.RoutingBinding{}, fmt.Errorf("无效配置范围")
	}
	return routingBinding(repository.DB, scope, id)
}
func resolveRouting(db *gorm.DB, nodeID uint) (*EffectiveRouting, error) {
	b, err := routingBinding(db, "node", nodeID)
	if err != nil {
		return nil, err
	}
	return resolveRoutingBinding(db, nodeID, b)
}

func resolveRoutingBinding(db *gorm.DB, nodeID uint, b entity.RoutingBinding) (*EffectiveRouting, error) {
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
			out.Groups = append(out.Groups, fmt.Sprintf("%s [模板 %d 修订 %d]", g.Name, gb.TemplateID, gb.Revision))
			if base.TemplateID != 0 && (base.TemplateID != gb.TemplateID || base.Revision != gb.Revision) {
				return nil, fmt.Errorf("分组路由冲突：%s；请为节点指定模板", strings.Join(out.Groups, "、"))
			}
			base = gb
			source = "分组模板"
		}
		if base.TemplateID == 0 {
			var err error
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
	out.Warnings = []string{"强制策略只约束目标域名解析，不转换 IP 字面量，也不取决于客户端连接节点的地址族。", "系统规则、自定义规则、强制 IPv4 域名、强制 IPv6 域名按顺序匹配；两组域名重叠时 IPv4 优先。"}
	if p.SchemaVersion < 2 {
		out.Warnings = append(out.Warnings, "旧版策略仍以 IPv4 兜底；编辑并发布新版后，选择新版本才能改为默认直连。")
	}
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
	if v, ok := overrides["outbound_overrides"]; ok && !isEmptyJSONObjectOrNull(v) {
		out.Sources["outbound_overrides"] = "节点覆盖"
		out.Sources["custom_outbounds"] = "节点覆盖"
	}
	if v, ok := overrides["dns_override"]; ok && !isEmptyJSONObjectOrNull(v) {
		out.Sources["dns_override"] = "节点覆盖"
		out.Sources["dns"] = "节点覆盖"
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

// PreviewTemplate resolves a proposed node source without node overrides or any
// writes. The editor must show the immutable revision, not the template draft or
// the node's currently overridden policy.
func (s *RoutingService) PreviewTemplate(nodeID uint, mode string, templateID, revision uint) (*EffectiveRouting, error) {
	if mode != "inherit" && mode != "template" {
		return nil, fmt.Errorf("请选择继承或指定模板")
	}
	if mode == "inherit" && (templateID != 0 || revision != 0) {
		return nil, fmt.Errorf("继承来源不能同时指定模板版本")
	}
	if mode == "template" && (templateID == 0 || revision == 0) {
		return nil, fmt.Errorf("请选择已发布模板及版本")
	}
	var node entity.Node
	if err := repository.DB.Select("id").First(&node, nodeID).Error; err != nil {
		return nil, err
	}
	return resolveRoutingBinding(repository.DB, nodeID, entity.RoutingBinding{
		Scope: "node", TargetID: nodeID, Mode: mode, TemplateID: templateID,
		Revision: revision, Overrides: "{}",
	})
}
func (s *RoutingService) SaveTemplate(id, expected uint, name, description, draft, actor string) (*entity.RoutingTemplate, error) {
	p, err := xray.DecodeRoutingPolicy(draft)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return nil, fmt.Errorf("模板名称不能为空且不能超过 100 字节")
	}
	t := entity.RoutingTemplate{ID: id, Name: strings.TrimSpace(name), Description: description, Version: 1}
	err = repository.DB.Transaction(func(tx *gorm.DB) error {
		var prevPtr *xray.RoutingPolicy
		if id > 0 {
			var prev entity.RoutingTemplate
			if err := tx.First(&prev, id).Error; err != nil {
				return err
			}
			if prev.Version != expected {
				return ErrRoutingConflict
			}
			if prevPolicy, decErr := xray.DecodeRoutingPolicy(prev.Draft); decErr == nil {
				prevPtr = &prevPolicy
			}
		}
		encPolicy, encErr := xray.EncryptPolicySecrets(p, prevPtr)
		if encErr != nil {
			return encErr
		}
		data, marshalErr := json.Marshal(encPolicy)
		if marshalErr != nil {
			return marshalErr
		}
		t.Draft = string(data)
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
		return tx.First(&t, id).Error
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
		policy, err := xray.DecodeRoutingPolicy(t.Draft)
		if err != nil {
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
		rev = entity.RoutingTemplateRevision{TemplateID: id, Revision: max + 1, SchemaVersion: policy.SchemaVersion, Payload: t.Draft, Actor: actor}
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

type BindingImpact struct {
	Changed   []uint          `json:"changed_nodes"`
	Conflicts map[uint]string `json:"conflicts"`
}

func effectiveKey(e *EffectiveRouting, err error) string {
	if err != nil {
		return "error:" + err.Error()
	}
	var normalized any
	if e.Policy != nil {
		if dec, decErr := xray.DecryptPolicySecrets(*e.Policy); decErr == nil {
			normalized = dec
		} else {
			normalized = e.Policy
		}
	}
	b, _ := json.Marshal([]any{e.TemplateID, e.Revision, normalized, e.Binding.ConfigPath})
	return crypto.HashConfig(string(b))
}

func isEmptyJSONObjectOrNull(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == "{}" {
		return true
	}
	var m map[string]any
	return json.Unmarshal(raw, &m) == nil && len(m) == 0
}

func normalizeOptionalMapJSON(raw json.RawMessage) any {
	if isEmptyJSONObjectOrNull(raw) {
		return nil
	}
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}

// MaskEffectiveRouting 面向普通管理员脱敏节点有效路由中的敏感凭据。
func MaskEffectiveRouting(e *EffectiveRouting) *EffectiveRouting {
	if e == nil {
		return nil
	}
	cp := *e
	cp.Binding.Overrides = xray.MaskOverridesJSON(cp.Binding.Overrides)
	if cp.Policy != nil {
		masked := xray.MaskPolicySecrets(*cp.Policy)
		cp.Policy = &masked
	}
	return &cp
}

// MaskRoutingOverview 面向普通管理员脱敏路由概览中的模板草稿与节点有效路由凭据。
func MaskRoutingOverview(o *RoutingOverview) *RoutingOverview {
	if o == nil {
		return nil
	}
	cp := *o
	cp.Templates = make([]RoutingTemplateSummary, len(o.Templates))
	for i, t := range o.Templates {
		t.Draft = xray.MaskPolicyJSON(t.Draft)
		cp.Templates[i] = t
	}
	cp.Nodes = make([]NodeRoutingSummary, len(o.Nodes))
	for i, n := range o.Nodes {
		n.Routing = MaskEffectiveRouting(n.Routing)
		cp.Nodes[i] = n
	}
	return &cp
}
func (s *RoutingService) SaveBinding(b entity.RoutingBinding, expected uint, actor string, super bool, reports ...*BindingImpact) error {
	return s.saveBinding(b, expected, actor, super, false, reports...)
}

// PreviewBinding runs the same validation and inheritance calculation in a
// rolled-back transaction. No binding, outbox, status or audit write is committed.
func (s *RoutingService) PreviewBinding(b entity.RoutingBinding, super bool) (BindingImpact, error) {
	var impact BindingImpact
	err := s.saveBinding(b, b.Version, "", super, true, &impact)
	return impact, err
}

var errBindingPreview = errors.New("binding preview rollback")

func (s *RoutingService) saveBinding(b entity.RoutingBinding, expected uint, actor string, super, preview bool, reports ...*BindingImpact) error {
	impact := BindingImpact{Changed: []uint{}, Conflicts: map[uint]string{}}
	if len(reports) > 0 {
		defer func() { *reports[0] = impact }()
	}

	if b.Scope != "global" && b.Scope != "group" && b.Scope != "node" {
		return fmt.Errorf("无效配置范围")
	}
	if (b.Scope == "global" || b.Scope == "group") && !super {
		return fmt.Errorf("共享绑定仅超级管理员可修改")
	}
	if b.Scope == "global" && b.TargetID != 0 {
		return fmt.Errorf("全局目标必须为 0")
	}
	if b.Mode != "legacy" && b.Mode != "inherit" && b.Mode != "template" {
		return fmt.Errorf("路由来源无效")
	}
	if b.Scope != "node" && b.Mode == "legacy" {
		return fmt.Errorf("基础路由（不使用模板）仅适用于节点")
	}
	if b.ConfigPath != "" && (b.Scope != "node" || !regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`).MatchString(b.ConfigPath) || path.Clean(b.ConfigPath) != b.ConfigPath || path.Ext(b.ConfigPath) != ".json") {
		return fmt.Errorf("配置路径须为绝对 JSON 文件路径，不允许上级目录或特殊字符")
	}
	if b.Mode != "template" && (b.TemplateID != 0 || b.Revision != 0) {
		return fmt.Errorf("继承或基础路由不能同时指定模板版本")
	}
	if b.Scope == "global" && b.Mode != "template" {
		return fmt.Errorf("全局默认必须绑定已发布模板")
	}
	if b.Overrides == "" {
		b.Overrides = "{}"
	}
	if super {
		if err := xray.ValidateRoutingOverrides(b.Overrides); err != nil {
			return err
		}
	}
	if b.Mode == "legacy" && b.Overrides != "{}" {
		return fmt.Errorf("基础路由不接受节点路由覆盖；请先选择继承或指定模板")
	}
	if b.Scope != "node" && b.Overrides != "{}" {
		return fmt.Errorf("只有节点允许字段覆盖")
	}
	err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if b.Scope != "global" {
			table := map[string]string{"node": "nodes", "group": "groups"}[b.Scope]
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
			if json.Unmarshal([]byte(b.Overrides), &next) != nil || next == nil {
				return fmt.Errorf("覆盖必须是 JSON 对象")
			}
			var previousRules, nextRules any
			_ = json.Unmarshal(prev["custom_rules"], &previousRules)
			_ = json.Unmarshal(next["custom_rules"], &nextRules)
			if !reflect.DeepEqual(previousRules, nextRules) {
				return fmt.Errorf("高级规则仅超级管理员可修改")
			}
			var maskedPrev map[string]json.RawMessage
			_ = json.Unmarshal([]byte(xray.MaskOverridesJSON(old.Overrides)), &maskedPrev)
			prevOutOv := normalizeOptionalMapJSON(maskedPrev["outbound_overrides"])
			rawPrevOutOv := normalizeOptionalMapJSON(prev["outbound_overrides"])
			nextOutOv := normalizeOptionalMapJSON(next["outbound_overrides"])
			if !reflect.DeepEqual(prevOutOv, nextOutOv) && !reflect.DeepEqual(rawPrevOutOv, nextOutOv) {
				return fmt.Errorf("节点出站覆盖仅超级管理员可修改")
			}
			prevDNSOv := normalizeOptionalMapJSON(prev["dns_override"])
			nextDNSOv := normalizeOptionalMapJSON(next["dns_override"])
			if !reflect.DeepEqual(prevDNSOv, nextDNSOv) {
				return fmt.Errorf("节点 DNS 覆盖仅超级管理员可修改")
			}
			restoredChanged := false
			if !isEmptyJSONObjectOrNull(prev["outbound_overrides"]) {
				next["outbound_overrides"] = prev["outbound_overrides"]
				restoredChanged = true
			}
			if !isEmptyJSONObjectOrNull(prev["dns_override"]) {
				next["dns_override"] = prev["dns_override"]
				restoredChanged = true
			}
			if restoredChanged {
				restored, marshalErr := json.Marshal(next)
				if marshalErr != nil {
					return marshalErr
				}
				b.Overrides = string(restored)
			}
			if err := xray.ValidateRoutingOverrides(b.Overrides); err != nil {
				return err
			}
		}
		var basePolicyPtr *xray.RoutingPolicy
		if b.TemplateID > 0 {
			var rev entity.RoutingTemplateRevision
			if err := tx.Where("template_id = ? AND revision = ?", b.TemplateID, b.Revision).First(&rev).Error; err != nil {
				return fmt.Errorf("模板修订不存在")
			}
			if bp, decErr := xray.DecodeRoutingPolicy(rev.Payload); decErr == nil {
				basePolicyPtr = &bp
			}
		}
		if b.Mode == "template" && b.TemplateID == 0 {
			return fmt.Errorf("请选择已发布模板")
		}
		if super && b.Overrides != "{}" {
			encOv, encErr := xray.EncryptOverridesSecrets(b.Overrides, old.Overrides, basePolicyPtr)
			if encErr != nil {
				return encErr
			}
			b.Overrides = encOv
		}

		var nodes []entity.Node
		before := map[uint]string{}
		q := tx.Model(&entity.Node{})
		if b.Scope == "node" {
			q = q.Where("nodes.id = ?", b.TargetID)
		}
		if b.Scope == "group" {
			q = q.Joins("JOIN group_nodes ON group_nodes.node_id = nodes.id").Where("group_nodes.group_id = ?", b.TargetID)
		}
		if err := q.Find(&nodes).Error; err != nil {
			return err
		}
		for _, n := range nodes {
			before[n.ID] = effectiveKey(resolveRouting(tx, n.ID))
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
		for _, n := range nodes {
			effective, err := resolveRouting(tx, n.ID)
			if err != nil && b.Scope == "node" {
				return err
			}
			if effectiveKey(effective, err) == before[n.ID] {
				continue
			}
			if err != nil {
				impact.Conflicts[n.ID] = err.Error()
			}
			if err := repository.InvalidateConfigs(tx, []uint{n.ID}); err != nil {
				return err
			}
			if err != nil {
				if e := tx.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Updates(map[string]any{"recovery_pending": false, "last_error": err.Error(), "recovery_attempts": 3}).Error; e != nil {
					return e
				}
			} else {
				impact.Changed = append(impact.Changed, n.ID)
			}
		}

		if preview {
			return errBindingPreview
		}
		return tx.Create(&entity.SyncLog{Actor: actor, Action: "routing_binding", Target: fmt.Sprintf("%s:%d", b.Scope, b.TargetID), Success: true, Message: fmt.Sprintf("绑定版本 %d", b.Version)}).Error
	})
	if preview && errors.Is(err, errBindingPreview) {
		return nil
	}
	return err
}
