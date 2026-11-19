package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/response"
	"net/http"
	"strconv"
)

func registerRoutingRoutes(api *gin.RouterGroup) {
	api = api.Group("")
	api.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 300*1024)
		if raw := c.Param("id"); raw != "" {
			if n, err := strconv.ParseUint(raw, 10, 32); err != nil || n == 0 {
				response.Fail(c, 400, "无效对象 ID")
				c.Abort()
				return
			}
		}
		c.Next()
	})
	s := service.NewRoutingService()
	api.GET("/node-sync-logs", listNodeSyncLogs)
	reply := func(c *gin.Context, v any, err error) {
		if err != nil {
			if errors.Is(err, service.ErrRoutingConflict) || errors.Is(err, service.ErrNodeSyncBusy) {
				response.Fail(c, 409, err.Error())
				return
			}
			response.Fail(c, 400, err.Error())
			return
		}
		response.Success(c, v)
	}
	isSuper := func(c *gin.Context) bool { return c.GetString(ctxRole) == entity.RoleSuperAdmin }
	id := func(c *gin.Context) uint { n, _ := strconv.ParseUint(c.Param("id"), 10, 32); return uint(n) }
	api.GET("/routing-overview", func(c *gin.Context) {
		v, err := s.Overview()
		if err == nil && !isSuper(c) {
			v = service.MaskRoutingOverview(v)
		}
		reply(c, v, err)
	})
	api.POST("/nodes/:id/routing/template-preview", func(c *gin.Context) {
		var req struct {
			Mode       string `json:"mode"`
			TemplateID uint   `json:"template_id"`
			Revision   uint   `json:"revision"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			reply(c, nil, err)
			return
		}
		v, err := s.PreviewTemplate(id(c), req.Mode, req.TemplateID, req.Revision)
		if err == nil && !isSuper(c) {
			v = service.MaskEffectiveRouting(v)
		}
		reply(c, v, err)
	})
	api.GET("/routing-templates", func(c *gin.Context) {
		var rows []entity.RoutingTemplate
		err := repository.DB.Order("id").Find(&rows).Error
		if err == nil && !isSuper(c) {
			for i := range rows {
				rows[i].Draft = xray.MaskPolicyJSON(rows[i].Draft)
			}
		}
		reply(c, rows, err)
	})
	api.GET("/routing-templates/:id", func(c *gin.Context) {
		var row entity.RoutingTemplate
		err := repository.DB.First(&row, id(c)).Error
		if err == nil && !isSuper(c) {
			row.Draft = xray.MaskPolicyJSON(row.Draft)
		}
		reply(c, row, err)
	})
	api.GET("/routing-templates/:id/impact", func(c *gin.Context) {
		var rows []entity.RoutingBinding
		err := repository.DB.Where("template_id = ?", id(c)).Find(&rows).Error
		if err == nil && !isSuper(c) {
			for i := range rows {
				rows[i].Overrides = xray.MaskOverridesJSON(rows[i].Overrides)
			}
		}
		reply(c, rows, err)
	})
	api.GET("/routing-templates/:id/revisions", func(c *gin.Context) {
		var rows []entity.RoutingTemplateRevision
		err := repository.DB.Where("template_id = ?", id(c)).Order("revision DESC").Find(&rows).Error
		if err == nil && !isSuper(c) {
			for i := range rows {
				rows[i].Payload = xray.MaskPolicyJSON(rows[i].Payload)
			}
		}
		reply(c, rows, err)
	})
	save := func(c *gin.Context) {
		var r struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Draft       json.RawMessage `json:"draft"`
			Version     uint            `json:"version"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			reply(c, nil, err)
			return
		}
		v, err := s.SaveTemplate(id(c), r.Version, r.Name, r.Description, string(r.Draft), actorFrom(c))
		reply(c, v, err)
	}
	api.POST("/routing-templates", RequireSuperAdmin(), save)
	api.PATCH("/routing-templates/:id", RequireSuperAdmin(), save)
	api.POST("/routing-templates/:id/publish", RequireSuperAdmin(), func(c *gin.Context) {
		var r struct {
			Version uint `json:"version"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			reply(c, nil, err)
			return
		}
		v, err := s.Publish(id(c), r.Version, actorFrom(c))
		reply(c, v, err)
	})
	api.DELETE("/routing-templates/:id", RequireSuperAdmin(), func(c *gin.Context) { reply(c, nil, s.DeleteTemplate(id(c))) })
	for path, scope := range map[string]string{"/nodes/:id/routing": "node", "/groups/:id/routing": "group", "/settings/routing-default": "global"} {
		scope := scope
		api.POST(path+"/preview", func(c *gin.Context) {
			var b entity.RoutingBinding
			if err := c.ShouldBindJSON(&b); err != nil {
				reply(c, nil, err)
				return
			}
			b.Scope, b.TargetID = scope, id(c)
			v, err := s.PreviewBinding(b, c.GetString(ctxRole) == entity.RoleSuperAdmin)
			reply(c, v, err)
		})
		api.GET(path, func(c *gin.Context) {
			v, err := s.Binding(scope, id(c))
			if err == nil && !isSuper(c) {
				v.Overrides = xray.MaskOverridesJSON(v.Overrides)
			}
			reply(c, v, err)
		})
		api.PUT(path, func(c *gin.Context) {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 300*1024)
			var b entity.RoutingBinding
			if err := c.ShouldBindJSON(&b); err != nil {
				reply(c, nil, err)
				return
			}
			b.Scope = scope
			b.TargetID = id(c)
			var impact service.BindingImpact
			err := s.SaveBinding(b, b.Version, actorFrom(c), c.GetString(ctxRole) == entity.RoleSuperAdmin, &impact)
			reply(c, impact, err)
		})
	}
	api.POST("/nodes/:id/preflight", func(c *gin.Context) { reply(c, nil, service.NewSyncService().PreflightNode(id(c))) })
	replyBaseline := func(c *gin.Context, result any, err error) {
		if errors.Is(err, service.ErrBaselineChanged) || errors.Is(err, service.ErrNodeSyncBusy) || errors.Is(err, service.ErrConfigLeaseLost) {
			c.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "message": err.Error()})
			return
		}
		reply(c, result, err)
	}
	api.POST("/nodes/:id/inspect-baseline", RequireSuperAdmin(), func(c *gin.Context) {
		result, err := service.NewSyncService().InspectBaseline(id(c))
		replyBaseline(c, result, err)
	})
	api.POST("/nodes/:id/adopt-baseline", RequireSuperAdmin(), func(c *gin.Context) {
		var req service.AdoptBaselineRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			reply(c, nil, err)
			return
		}
		jobs, err := service.NewSyncService().AdoptBaseline(id(c), req, actorFrom(c))
		replyBaseline(c, jobs, err)
	})
	api.POST("/nodes/:id/preflight-draft", RequireSuperAdmin(), func(c *gin.Context) {
		var req service.PreflightDraftRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			reply(c, nil, err)
			return
		}
		res, err := service.NewSyncService().PreflightDraft(id(c), req)
		reply(c, res, err)
	})
	api.POST("/nodes/:id/restore-policy", RequireSuperAdmin(), func(c *gin.Context) {
		var req struct {
			Version uint   `json:"version"`
			Key     string `json:"request_key"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			reply(c, nil, err)
			return
		}
		if len(req.Key) < 8 || len(req.Key) > 100 {
			reply(c, nil, fmt.Errorf("请提供 8–100 字符幂等键"))
			return
		}
		if err := service.NewSyncService().RestorePolicy(id(c), req.Version, actorFrom(c)); err != nil {
			reply(c, nil, err)
			return
		}
		jobs, err := service.EnqueueDeployments([]uint{id(c)}, req.Key, actorFrom(c))
		reply(c, jobs, err)
	})
	api.GET("/nodes/:id/config-state", func(c *gin.Context) {
		var row entity.NodeConfigState
		err := repository.DB.First(&row, "node_id = ?", id(c)).Error
		reply(c, row, err)
	})
	api.POST("/nodes/:id/sni-diagnostics", func(c *gin.Context) {
		var req struct {
			Host string `json:"host"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			reply(c, nil, err)
			return
		}
		out, err := service.NewSyncService().DiagnoseSNI(id(c), req.Host)
		reply(c, out, err)
	})
	api.GET("/nodes/:id/effective-config", func(c *gin.Context) {
		v, err := s.Effective(id(c))
		if err != nil {
			reply(c, nil, err)
			return
		}
		if !isSuper(c) {
			v = service.MaskEffectiveRouting(v)
		}
		content, warnings, err := service.NewSyncService().PreviewConfig(id(c))
		hash, hashErr := service.NewSyncService().DesiredConfigHash(id(c))
		if err == nil {
			err = hashErr
		}
		reply(c, gin.H{"routing": v, "config": content, "warnings": warnings, "desired_hash": hash}, err)
	})
	api.GET("/config-deployments", func(c *gin.Context) {
		var rows []entity.ConfigDeployment
		q := repository.DB.Model(&entity.ConfigDeployment{})
		if raw := c.Query("node_id"); raw != "" {
			n, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || n == 0 {
				reply(c, nil, fmt.Errorf("无效节点 ID"))
				return
			}
			q = q.Where("node_id = ?", n)
		}
		if c.Query("page") == "" {
			err := q.Order("id DESC").Limit(100).Find(&rows).Error
			reply(c, rows, err)
			return
		}
		page, _ := strconv.Atoi(c.Query("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		if size < 1 || size > 100 {
			size = 20
		}
		var total int64
		if err := q.Count(&total).Error; err != nil {
			reply(c, nil, err)
			return
		}
		err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error
		reply(c, gin.H{"list": rows, "total": total}, err)
	})
	api.GET("/config-deployments/:id", func(c *gin.Context) {
		var row entity.ConfigDeployment
		err := repository.DB.First(&row, id(c)).Error
		reply(c, row, err)
	})
	api.POST("/config-deployments/:id/reconcile", func(c *gin.Context) { row, err := service.ReconcileDeployment(id(c)); reply(c, row, err) })
	api.POST("/config-deployments", func(c *gin.Context) {
		var r struct {
			Nodes    []uint          `json:"node_ids"`
			Key      string          `json:"request_key"`
			Expected map[uint]string `json:"expected_hashes"`
		}
		if err := c.ShouldBindJSON(&r); err != nil {
			reply(c, nil, err)
			return
		}
		if len(r.Nodes) == 0 || len(r.Nodes) > 50 || len(r.Key) < 8 || len(r.Key) > 100 {
			reply(c, nil, fmt.Errorf("请选择 1–50 个节点，并提供 8–100 字符幂等键"))
			return
		}
		rows, err := service.EnqueueDeployments(r.Nodes, r.Key, actorFrom(c), r.Expected)
		reply(c, rows, err)
	})
	api.POST("/config-deployments/:id/retry", func(c *gin.Context) {
		var job entity.ConfigDeployment
		if err := repository.DB.First(&job, id(c)).Error; err != nil {
			reply(c, nil, err)
			return
		}
		if job.Status != "failed" && job.Status != "unknown" && job.Status != "rolled_back" && job.Status != "rollback_failed" {
			reply(c, nil, fmt.Errorf("只允许重试失败或未确认任务"))
			return
		}
		if key := c.GetHeader("Idempotency-Key"); len(key) < 8 || len(key) > 100 {
			reply(c, nil, fmt.Errorf("重试必须提供 8–100 字符幂等键"))
			return
		}
		rows, err := service.EnqueueDeployments([]uint{job.NodeID}, fmt.Sprintf("retry-%d-%s", job.ID, c.GetHeader("Idempotency-Key")), actorFrom(c))
		reply(c, rows, err)
	})
}
