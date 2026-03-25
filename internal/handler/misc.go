package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// ---- 日志 ----

type LogHandler struct {
	logRepo *repository.LogRepository
}

func NewLogHandler() *LogHandler {
	return &LogHandler{logRepo: repository.NewLogRepository()}
}

func (h *LogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	logs, total, err := h.logRepo.List(page, pageSize)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.PageSuccess(c, total, logs)
}

// ---- 订阅 ----

type SubscribeHandler struct {
	svc *service.SubscribeService
}

func NewSubscribeHandler() *SubscribeHandler {
	return &SubscribeHandler{svc: service.NewSubscribeService()}
}

// Subscribe 处理 /sub/:token，返回 base64 编码订阅内容
func (h *SubscribeHandler) Subscribe(c *gin.Context) {
	token := c.Param("token")
	content, err := h.svc.GenerateSubscription(token)
	if err != nil {
		c.String(http.StatusNotFound, err.Error())
		return
	}
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Profile-Title", "xray-pilot")
	c.String(http.StatusOK, content)
}

// ---- 认证 ----

type AuthHandler struct {
	svc *service.AuthService
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{svc: service.NewAuthService()}
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	resp, err := h.svc.Login(&req)
	if err != nil {
		response.Fail(c, 401, err.Error())
		return
	}
	response.Success(c, resp)
}

// ---- 系统配置 ----

type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// GetConfig 返回系统可公开配置项
func (h *SystemHandler) GetConfig(c *gin.Context) {
	cfg := config.Global
	response.Success(c, gin.H{
		"server": gin.H{
			"port": cfg.Server.Port,
			"mode": cfg.Server.Mode,
		},
		"database": gin.H{
			"driver": cfg.Database.Driver,
		},
		"scheduler": gin.H{
			"drift_check_interval":  cfg.Scheduler.DriftCheckInterval,
			"health_check_interval": cfg.Scheduler.HealthCheckInterval,
		},
	})
}

// UpdateConfig 更新运行时可调整的配置项（当前支持 scheduler 间隔）
func (h *SystemHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		DriftCheckInterval  *int `json:"drift_check_interval"`
		HealthCheckInterval *int `json:"health_check_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.DriftCheckInterval != nil {
		config.Global.Scheduler.DriftCheckInterval = *req.DriftCheckInterval
	}
	if req.HealthCheckInterval != nil {
		config.Global.Scheduler.HealthCheckInterval = *req.HealthCheckInterval
	}
	response.Success(c, gin.H{"message": "配置已更新（重启后定时器生效）"})
}
