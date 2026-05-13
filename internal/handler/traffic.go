package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// TrafficHandler 流量统计接口
type TrafficHandler struct {
	svc *service.TrafficService
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
