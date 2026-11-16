package service

import (
	"encoding/json"
	"reflect"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type RoutingTemplateSummary struct {
	entity.RoutingTemplate
	LatestRevision uint `json:"latest_revision"`
	DraftChanged   bool `json:"draft_changed"`
	BindingCount   int  `json:"binding_count"`
	NodeCount      int  `json:"node_count"`
}
type NodeRoutingSummary struct {
	ID         uint              `json:"id"`
	Name       string            `json:"name"`
	IP         string            `json:"ip"`
	Active     bool              `json:"active"`
	SyncStatus entity.SyncStatus `json:"sync_status"`
	Routing    *EffectiveRouting `json:"routing"`
	Error      string            `json:"error,omitempty"`
	JobStatus  string            `json:"job_status,omitempty"`
}
type RoutingOverview struct {
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
	out := &RoutingOverview{Templates: []RoutingTemplateSummary{}, Nodes: []NodeRoutingSummary{}, References: []RoutingReference{}}
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
	jobStatus := map[uint]string{}
	for _, job := range jobs {
		jobStatus[job.NodeID] = job.Status
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
		row.JobStatus = jobStatus[n.ID]
		if err != nil {
			row.Error = err.Error()
		} else if e.TemplateID != 0 {
			uses[e.TemplateID]++
		}
		out.Nodes = append(out.Nodes, row)
	}
	for _, t := range templates {
		r := latest[t.ID]
		var draft, published any
		_ = json.Unmarshal([]byte(t.Draft), &draft)
		_ = json.Unmarshal([]byte(r.Payload), &published)
		out.Templates = append(out.Templates, RoutingTemplateSummary{RoutingTemplate: t, LatestRevision: r.Revision, DraftChanged: r.Revision == 0 || !reflect.DeepEqual(draft, published), BindingCount: counts[t.ID], NodeCount: uses[t.ID]})
	}
	settings := NewSettingService()
	out.AutoSync = settings.GetInt(KeySchedulerDriftInterval) > 0 || settings.Get(KeyXrayLiveApply) == "true"
	return out, nil
}
