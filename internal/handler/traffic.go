package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// TrafficHandler 流量统计接口
type TrafficHandler struct {
	svc *service.TrafficService
}

// Analytics GET /api/traffic/analytics. All timestamps carry an explicit offset.
func (h *TrafficHandler) Analytics(c *gin.Context) {
	start, err := time.Parse(time.RFC3339, c.Query("start"))
	if err != nil {
		response.BadRequest(c, "开始时间必须包含时区")
		return
	}
	end, err := time.Parse(time.RFC3339, c.Query("end"))
	if err != nil {
		response.BadRequest(c, "结束时间必须包含时区")
		return
	}
	r := dto.TrafficRange{Start: start, End: end, Dimension: c.DefaultQuery("dimension", "node")}
	for key, target := range map[string]*uint{"node_id": &r.NodeID, "user_id": &r.UserID} {
		if raw := c.Query(key); raw != "" {
			id, e := strconv.ParseUint(raw, 10, 32)
			if e != nil || id == 0 {
				response.BadRequest(c, "节点或用户 ID 无效")
				return
			}
			*target = uint(id)
		}
	}
	now := time.Now()
	if err := service.ValidateTrafficRange(r, now); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	data, err := h.svc.Analytics(c.Request.Context(), r, now)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, data)
}

func NewTrafficHandler() *TrafficHandler {
	return &TrafficHandler{svc: service.NewTrafficService()}
}

// trafficSummaryResponse 仪表盘流量概览响应
type trafficSummaryResponse struct {
	TotalUpBytes   int64  `json:"total_up_bytes"`
	TotalDownBytes int64  `json:"total_down_bytes"`
	ActiveUsers7d  int64  `json:"active_users_7d"`
	LastUpdatedAt  string `json:"last_updated_at,omitempty"`
}

// Summary GET /api/traffic/summary
// 返回 Dashboard 所需的累计流量与活跃用户数
func (h *TrafficHandler) Summary(c *gin.Context) {
	summary, err := h.svc.GetSummary()
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	resp := trafficSummaryResponse{
		TotalUpBytes:   summary.TotalUp,
		TotalDownBytes: summary.TotalDown,
		ActiveUsers7d:  summary.ActiveUsers7d,
	}
	if summary.LastUpdatedAt != nil {
		resp.LastUpdatedAt = summary.LastUpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	response.Success(c, resp)
}
