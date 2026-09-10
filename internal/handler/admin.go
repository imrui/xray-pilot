package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// AdminHandler 管理员账号管理（/api/admins 仅 super_admin；/api/me 所有登录管理员）
type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler() *AdminHandler {
	return &AdminHandler{svc: service.NewAdminService()}
}

func (h *AdminHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, list)
}

func (h *AdminHandler) Create(c *gin.Context) {
	var req dto.CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	resp, err := h.svc.Create(&req, actorFrom(c))
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *AdminHandler) Update(c *gin.Context) {
	id, ok := parseAdminID(c)
	if !ok {
		return
	}
	var req dto.UpdateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	resp, err := h.svc.Update(id, &req, currentAdminID(c), actorFrom(c))
	if err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, resp)
}

func (h *AdminHandler) Delete(c *gin.Context) {
	id, ok := parseAdminID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id, currentAdminID(c), actorFrom(c)); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

// SetPassword 超级管理员重置指定账号密码
func (h *AdminHandler) SetPassword(c *gin.Context) {
	id, ok := parseAdminID(c)
	if !ok {
		return
	}
	var req dto.SetAdminPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.SetPassword(id, req.Password, currentAdminID(c), actorFrom(c)); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

// Me 当前登录管理员信息
func (h *AdminHandler) Me(c *gin.Context) {
	response.Success(c, dto.MeResponse{
		ID:       currentAdminID(c),
		Username: c.GetString(ctxUsername),
		Role:     c.GetString(ctxRole),
	})
}

// ChangeMyPassword 修改自己的密码
func (h *AdminHandler) ChangeMyPassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.ChangeOwnPassword(currentAdminID(c), &req, actorFrom(c)); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	response.Success(c, nil)
}

func parseAdminID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的管理员ID")
		return 0, false
	}
	return uint(id), true
}
