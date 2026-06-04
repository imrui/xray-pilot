package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// InstallHandler 节点一键接入流程的 HTTP 入口
//
// JWT 域端点（/api/nodes/install-tokens/*）由管理员使用；
// token 域端点（/api/install/*）由 node-bootstrap.sh 装机脚本调用。
type InstallHandler struct {
	svc *service.InstallService
}

func NewInstallHandler() *InstallHandler {
	return &InstallHandler{svc: service.NewInstallService()}
}

// CreateToken POST /api/nodes/install-tokens
// 管理员侧创建一次性 token，返回完整的 curl 命令。
func (h *InstallHandler) CreateToken(c *gin.Context) {
	var req dto.CreateInstallTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	admin, _ := c.Get("username")
	adminStr, _ := admin.(string)
	if adminStr == "" {
		adminStr = "unknown"
	}

	resp, err := h.svc.CreateToken(&req, adminStr)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, resp)
}

// ListTokens GET /api/nodes/install-tokens
// 列出当前活跃 token；前端对话框用于轮询状态。
func (h *InstallHandler) ListTokens(c *gin.Context) {
	tokens, err := h.svc.ListActive()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, tokens)
}

// GetToken GET /api/nodes/install-tokens/:token
// 单点查询，前端拿到 token 字符串后高频轮询用；返回包括 used 状态。
func (h *InstallHandler) GetToken(c *gin.Context) {
	tokenStr := c.Param("token")
	resp, err := h.svc.FindByToken(tokenStr)
	if err != nil {
		if errors.Is(err, service.ErrInstallTokenNotFound) {
			response.Fail(c, http.StatusNotFound, err.Error())
			return
		}
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, resp)
}

// DeleteToken DELETE /api/nodes/install-tokens/:id
// 管理员主动撤销
func (h *InstallHandler) DeleteToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的 token id")
		return
	}
	admin, _ := c.Get("username")
	adminStr, _ := admin.(string)
	if err := h.svc.Delete(uint(id), adminStr); err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted_id": id})
}

// GetPanelPubkey GET /api/install/panel-pubkey?token=xxx
// 无 JWT；token 鉴权（一次性 + 短 TTL + IP 绑定）。
// 首次调用时把请求 IP 写入 used_by_ip，后续 panel-pubkey 与 register 必须匹配。
func (h *InstallHandler) GetPanelPubkey(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		response.BadRequest(c, "缺少 token 参数")
		return
	}

	ip := c.ClientIP()
	t, err := h.svc.AuthorizeToken(tokenStr, ip)
	if err != nil {
		writeTokenError(c, err)
		return
	}

	// 首次调用绑定 IP（后续 register 调用复用同一 IP）
	if err := h.svc.BindTokenIP(t, ip); err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	pubkey, err := h.svc.ReadPanelPubKey()
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	// 直接以 text/plain 返回公钥内容，方便脚本 curl 后直接写 authorized_keys
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, pubkey)
}

// Register POST /api/install/register?token=xxx
// 节点装机脚本回调；创建 Node 记录 + 标记 token used。
func (h *InstallHandler) Register(c *gin.Context) {
	tokenStr := c.Query("token")
	if tokenStr == "" {
		response.BadRequest(c, "缺少 token 参数")
		return
	}

	var req dto.RegisterNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	ip := c.ClientIP()
	t, err := h.svc.AuthorizeToken(tokenStr, ip)
	if err != nil {
		writeTokenError(c, err)
		return
	}

	// register 阶段要求 IP 匹配；若 panel-pubkey 阶段尚未绑定 IP，这里补绑（兜底但不常见）
	if err := h.svc.BindTokenIP(t, ip); err != nil {
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	resp, err := h.svc.RegisterNode(t, ip, &req)
	if err != nil {
		if errors.Is(err, service.ErrInstallTokenUsed) {
			writeTokenError(c, err)
			return
		}
		response.Fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, resp)
}

// writeTokenError 把 install token 鉴权错误映射到合适的 HTTP 状态码。
func writeTokenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInstallTokenNotFound):
		response.Fail(c, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrInstallTokenUsed):
		response.Fail(c, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrInstallTokenExpired):
		response.Fail(c, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrInstallTokenIPMismatch):
		response.Fail(c, http.StatusForbidden, err.Error())
	default:
		response.Fail(c, http.StatusBadRequest, err.Error())
	}
}
