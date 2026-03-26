package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

type UserHandler struct {
	svc        *service.UserService
	settingSvc *service.SettingService
}

func NewUserHandler() *UserHandler {
	return &UserHandler{
		svc:        service.NewUserService(),
		settingSvc: service.NewSettingService(),
	}
}

func (h *UserHandler) baseURL(c *gin.Context) string {
	// 优先使用后台配置的 base_url，留空则从请求 Host 自动推断
	if base := h.settingSvc.Get(service.KeySubscriptionBaseURL); base != "" {
		return strings.TrimRight(base, "/")
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}

func (h *UserHandler) Create(c *gin.Context) {
	var req dto.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	user, err := h.svc.Create(&req, h.baseURL(c))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, user)
}

func (h *UserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	users, total, err := h.svc.List(page, pageSize, h.baseURL(c))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.PageSuccess(c, total, users)
}

func (h *UserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	var req dto.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	user, err := h.svc.Update(uint(id), &req, h.baseURL(c))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, user)
}

func (h *UserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *UserHandler) Toggle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	if err := h.svc.ToggleActive(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 0, Message: "ok"})
}

// ResetUUID 重置用户 UUID（导致所有节点下该用户连接失效，触发全量同步）
func (h *UserHandler) ResetUUID(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	user, err := h.svc.ResetUUID(uint(id), h.baseURL(c))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, user)
}

// ResetToken 重置用户订阅 Token（使旧订阅链接失效）
func (h *UserHandler) ResetToken(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的用户ID")
		return
	}
	user, err := h.svc.ResetToken(uint(id), h.baseURL(c))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, user)
}
