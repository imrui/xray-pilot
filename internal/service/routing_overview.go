package service

import (
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

type RoutingTemplateSummary struct {
	entity.RoutingTemplate
	LatestRevision uint `json:"latest_revision"`
	DraftChanged   bool `json:"draft_changed"`
	BindingCount   int  `json:"binding_count"`
	NodeCount      int  `json:"node_count"`
}
type NodeRoutingSummary struct {
	ID              uint              `json:"id"`
	Name            string            `json:"name"`
	IP              string            `json:"ip"`
	Active          bool              `json:"active"`
	SyncStatus      entity.SyncStatus `json:"sync_status"`
	Routing         *EffectiveRouting `json:"routing"`
	Error           string            `json:"error,omitempty"`
	JobStatus       string            `json:"job_status,omitempty"`
	LastSyncError   string            `json:"last_sync_error,omitempty"`
	LastSyncErrorAt *time.Time        `json:"last_sync_error_at,omitempty"`
}
type RoutingOverview struct {
	Presets    []entity.RoutingPreset   `json:"presets"`
	Templates  []RoutingTemplateSummary `json:"templates"`
	Nodes      []NodeRoutingSummary     `json:"nodes"`
	References []RoutingReference       `json:"references"`
	AutoSync   bool                     `json:"auto_sync"`
}
type RoutingReference struct {
	Scope      string `json:"scope"`
	TargetID   uint   `json:"target_id"`
	TargetName string `json:"target_name"`
	TemplateID uint   `json:"template_id"`
	Revision   uint   `json:"revision"`
}

func (s *RoutingService) Overview() (*RoutingOverview, error) {
	out := &RoutingOverview{Presets: entity.RoutingPresets(), Templates: []RoutingTemplateSummary{}, Nodes: []NodeRoutingSummary{}, References: []RoutingReference{}}
	var templates []entity.RoutingTemplate
	var revisions []entity.RoutingTemplateRevision
	var bindings []entity.RoutingBinding
	var nodes []entity.Node
	var groups []entity.Group
	for _, dest := range []any{&templates, &revisions, &bindings, &nodes, &groups} {
		if err := repository.DB.Find(dest).Error; err != nil {
			return nil, err
		}
	}
	latest := map[uint]entity.RoutingTemplateRevision{}
	counts := map[uint]int{}
	uses := map[uint]int{}
	names := map[string]map[uint]string{"node": {}, "group": {}, "global": {0: "全局默认"}}
	for _, n := range nodes {
		names["node"][n.ID] = n.Name
	}
	for _, g := range groups {
		names["group"][g.ID] = g.Name
	}
	var jobs []entity.ConfigDeployment
	if err := repository.DB.Where("id IN (SELECT MAX(id) FROM config_deployments GROUP BY node_id)").Find(&jobs).Error; err != nil {
		return nil, err
	}
	latestJobs := map[uint]entity.ConfigDeployment{}
	for _, job := range jobs {
		latestJobs[job.NodeID] = job
	}
	var states []entity.NodeConfigState
	if err := repository.DB.Select("node_id", "last_error", "updated_at").Where("last_error <> ''").Find(&states).Error; err != nil {
		return nil, err
	}
	failures := map[uint]entity.NodeConfigState{}
	for _, state := range states {
		failures[state.NodeID] = state
	}
	for _, r := range revisions {
		if r.Revision > latest[r.TemplateID].Revision {
			latest[r.TemplateID] = r
		}
	}
	for _, b := range bindings {
		if b.Mode == "template" {
			counts[b.TemplateID]++
			out.References = append(out.References, RoutingReference{Scope: b.Scope, TargetID: b.TargetID, TargetName: names[b.Scope][b.TargetID], TemplateID: b.TemplateID, Revision: b.Revision})
		}
	}
	for _, n := range nodes {
		e, err := resolveRouting(repository.DB, n.ID)
		row := NodeRoutingSummary{ID: n.ID, Name: n.Name, IP: n.IP, Active: n.Active, SyncStatus: n.SyncStatus, Routing: e}
		job := latestJobs[n.ID]
		row.JobStatus = job.Status
		failedJob := job.Status == "failed" || job.Status == "rolled_back" || job.Status == "rollback_failed"
		if n.SyncStatus == entity.SyncStatusFailed || failedJob {
			state := failures[n.ID]
			msg, at := state.LastError, state.UpdatedAt
			if failedJob && job.Error != "" && (msg == "" || !job.UpdatedAt.Before(at)) {
				msg, at = job.Error, job.UpdatedAt
			}
			if msg != "" {
				row.LastSyncError, row.LastSyncErrorAt = syncErrorSummary(msg), &at
			}
		}
		if err != nil {
			row.Error = err.Error()
		} else if e.TemplateID != 0 {
			uses[e.TemplateID]++
		}
		out.Nodes = append(out.Nodes, row)
	}
	for _, t := range templates {
		r := latest[t.ID]
		out.Templates = append(out.Templates, RoutingTemplateSummary{RoutingTemplate: t, LatestRevision: r.Revision, DraftChanged: r.Revision == 0 || !xray.PoliciesSemanticallyEqual(t.Draft, r.Payload), BindingCount: counts[t.ID], NodeCount: uses[t.ID]})
	}
	settings := NewSettingService()
	out.AutoSync = settings.GetInt(KeySchedulerDriftInterval) > 0 || settings.Get(KeyXrayLiveApply) == "true"
	return out, nil
}

// 概览向两种角色返回同一阶段摘要，避免转发 SSH 或内核错误中的参数。
func syncErrorSummary(message string) string {
	for _, cause := range []string{
		"ROLLBACK_FAILED", "ROLLED_BACK", "AUTHORIZATION_STOPPED", "DRIFT_CONFLICT", "PREFLIGHT_FAILED",
		"IPV6_UNAVAILABLE", "PATH_MISMATCH", "VERSION_UNSUPPORTED", "HEALTH_PROBE_UNAVAILABLE", "HEALTH_FAILED",
		"目标文件已存在", "节点配置任务未完成", "恢复配置无效",
		"协议配置不完整", "节点未绑定协议", "生成配置失败", "无法读取当前配置", "恢复配置生成失败", "SSH 连接失败",
	} {
		if strings.Contains(message, cause) {
			return cause
		}
	}
	return "同步失败，请查看同步记录"
}
