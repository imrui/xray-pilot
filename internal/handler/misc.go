package handler

import (
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

//go:embed templates/subscribe.html
var subscribePageHTML string

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

func (h *LogHandler) Cleanup(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Days < 1 || req.Days > 3650 {
		response.BadRequest(c, "清理范围需在 1 到 3650 天之间")
		return
	}

	cutoff := time.Now().AddDate(0, 0, -req.Days)
	deleted, err := h.logRepo.CleanupBefore(cutoff)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	h.logRepo.Record("cleanup_logs", "logs", true, "清理 "+strconv.Itoa(req.Days)+" 天前日志", 0)
	response.Success(c, gin.H{
		"deleted": deleted,
		"before":  cutoff.Format(time.RFC3339),
	})
}

// ---- 订阅 ----

type SubscribeHandler struct {
	svc *service.SubscribeService
}

func NewSubscribeHandler() *SubscribeHandler {
	return &SubscribeHandler{svc: service.NewSubscribeService()}
}

// isProxyClient 检测 UA 是否为代理客户端
func isProxyClient(ua string) bool {
	lower := strings.ToLower(ua)
	for _, kw := range []string{
		"v2rayn", "v2rayng", "clash", "mihomo", "clash-verge", "clash-meta",
		"sing-box", "singbox", "shadowrocket", "quantumult", "surge",
		"stash", "loon", "hiddify", "nekoray", "matsuri", "antenna", "xray", "neko",
	} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// detectFormat 从 UA 推断订阅格式
func detectFormat(ua string) string {
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "clash") || strings.Contains(lower, "mihomo") {
		return "clash"
	}
	if strings.Contains(lower, "sing-box") || strings.Contains(lower, "singbox") {
		return "singbox"
	}
	return "v2ray"
}

func (h *SubscribeHandler) baseURL(c *gin.Context) string {
	if base := strings.TrimRight(strings.TrimSpace(h.svc.GetSetting(service.KeySubscriptionBaseURL)), "/"); base != "" {
		return base
	}

	scheme := forwardedHeaderValue(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = forwardedHeaderValue(c.GetHeader("X-Forwarded-Scheme"))
	}
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := forwardedHeaderValue(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}

	return scheme + "://" + host
}

// Subscribe 处理 /sub/:token
// 优先级：format 参数 > sub=1 参数 > 代理客户端 UA > HTML 信息页
func (h *SubscribeHandler) Subscribe(c *gin.Context) {
	token := c.Param("token")
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	sub := c.Query("sub")
	ua := c.GetHeader("User-Agent")

	switch {
	case format == "html":
		h.handleInfoPage(c, token)
	case format != "":
		h.handleSubscription(c, token, format)
	case sub == "1":
		h.handleSubscription(c, token, "v2ray")
	case isProxyClient(ua):
		h.handleSubscription(c, token, detectFormat(ua))
	default:
		h.handleInfoPage(c, token)
	}
}

// handleSubscription 返回代理客户端格式的订阅内容
func (h *SubscribeHandler) handleSubscription(c *gin.Context, token, format string) {
	var expire int64
	if u, err := h.svc.GetUser(token); err == nil && u.ExpiresAt != nil {
		expire = u.ExpiresAt.Unix()
	}

	var (
		content string
		err     error
	)
	switch format {
	case "clash":
		content, err = h.svc.GenerateClash(token)
	case "singbox":
		content, err = h.svc.GenerateSingbox(token)
	default:
		content, err = h.svc.GenerateSubscription(token)
	}
	if err != nil {
		c.String(http.StatusForbidden, err.Error())
		return
	}

	c.Header("Subscription-Userinfo", fmt.Sprintf("upload=0; download=0; total=0; expire=%d", expire))
	c.Header("Profile-Update-Interval", "24")
	switch format {
	case "clash":
		c.Header("Content-Disposition", `attachment; filename="xray-pilot.yaml"`)
		c.Header("Content-Type", "text/yaml; charset=utf-8")
	case "singbox":
		c.Header("Content-Disposition", `attachment; filename="xray-pilot.json"`)
		c.Header("Content-Type", "application/json; charset=utf-8")
	default:
		c.Header("Content-Disposition", `attachment; filename="xray-pilot"`)
		c.Header("Content-Type", "text/plain; charset=utf-8")
	}
	c.String(http.StatusOK, content)
}

// handleInfoPage 返回 HTML 订阅信息页
func (h *SubscribeHandler) handleInfoPage(c *gin.Context, token string) {
	data, err := h.svc.GetSubscribePageDataWithBaseURL(token, h.baseURL(c))
	if err != nil {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(renderErrorPage(err.Error())))
		return
	}

	tmpl, parseErr := template.New("subscribe").Parse(subscribePageHTML)
	if parseErr != nil {
		c.String(http.StatusInternalServerError, "模板解析失败")
		return
	}

	type tmplData struct {
		*service.SubscribePageData
		ExpiresStr            string
		TrafficLastUpdatedStr string // 预格式化避免模板里写日期 layout
		HasTraffic            bool   // 是否曾产生过流量；用于模板判定占位提示
	}

	td := tmplData{SubscribePageData: data}
	if data.ExpiresAt != nil {
		td.ExpiresStr = data.ExpiresAt.Format("2006-01-02")
	} else {
		td.ExpiresStr = "∞"
	}
	if data.TrafficLastUpdatedAt != nil {
		td.TrafficLastUpdatedStr = data.TrafficLastUpdatedAt.Format("2006-01-02 15:04")
	}
	td.HasTraffic = data.TrafficTotalBytes > 0

	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = tmpl.Execute(c.Writer, td)
}

func renderErrorPage(msg string) string {
	return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Xray Pilot</title>` +
		`<style>body{font-family:system-ui,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0;background:#f8fafc}.box{text-align:center;padding:2rem;background:#fff;border-radius:12px;box-shadow:0 1px 4px rgba(0,0,0,.08)}h2{color:#ef4444;margin:0 0 .5rem}p{color:#64748b;margin:0}</style>` +
		`</head><body><div class="box"><h2>订阅不可用</h2><p>` + template.HTMLEscapeString(msg) + `</p></div></body></html>`
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
	settingSvc    *service.SettingService
	diagnosticSvc *service.DiagnosticsService
	syncSummary   *service.SyncSummaryService
	feishuSvc     *service.FeishuService
}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{
		settingSvc:    service.NewSettingService(),
		diagnosticSvc: service.NewDiagnosticsService(),
		syncSummary:   service.NewSyncSummaryService(),
		feishuSvc:     service.NewFeishuService(),
	}
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
			"dsn":    cfg.Database.DSN,
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

// GetDiagnostics 返回部署诊断结果
func (h *SystemHandler) GetDiagnostics(c *gin.Context) {
	response.Success(c, h.diagnosticSvc.Run())
}

// GetSyncSummary 返回全局待同步摘要
func (h *SystemHandler) GetSyncSummary(c *gin.Context) {
	summary, err := h.syncSummary.GetSyncSummary()
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, summary)
}

// GetFeishuStatus 返回飞书配置状态
func (h *SystemHandler) GetFeishuStatus(c *gin.Context) {
	response.Success(c, h.feishuSvc.GetStatus())
}

// TestFeishuConfig 检查飞书配置完整度
func (h *SystemHandler) TestFeishuConfig(c *gin.Context) {
	status := h.feishuSvc.ValidateConfig()
	if !status.Enabled {
		response.Fail(c, 400, "飞书集成当前未启用")
		return
	}
	if !status.Configured {
		response.Fail(c, 400, "飞书配置不完整："+strings.Join(status.MissingKeys, "、"))
		return
	}
	response.Success(c, status)
}
