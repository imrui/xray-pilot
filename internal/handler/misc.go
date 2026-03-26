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
	if content == "" {
		// 订阅内容为空：用户未分组或分组内无可用节点/协议配置
		c.String(http.StatusOK, "# xray-pilot: no active nodes in subscription")
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

type SystemHandler struct {
	settingSvc *service.SettingService
}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{settingSvc: service.NewSettingService()}
}

// GetSystemInfo 返回只读系统信息（服务端口、数据库驱动）
func (h *SystemHandler) GetSystemInfo(c *gin.Context) {
	cfg := config.Global
	response.Success(c, gin.H{
		"server": gin.H{
			"port": cfg.Server.Port,
			"mode": cfg.Server.Mode,
		},
		"database": gin.H{
			"driver": cfg.Database.Driver,
		},
	})
}

// GetSettings 返回所有运行时配置（DB 值优先，无则返回默认值）
func (h *SystemHandler) GetSettings(c *gin.Context) {
	response.Success(c, h.settingSvc.GetAll())
}

// UpdateSettings 批量更新运行时配置
func (h *SystemHandler) UpdateSettings(c *gin.Context) {
	var kv map[string]string
	if err := c.ShouldBindJSON(&kv); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.settingSvc.BatchUpdate(kv); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, h.settingSvc.GetAll())
}
