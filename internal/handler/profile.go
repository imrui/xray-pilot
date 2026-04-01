package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

type ProfileHandler struct {
	svc *service.ProfileService
}

func NewProfileHandler() *ProfileHandler {
	return &ProfileHandler{svc: service.NewProfileService()}
}

func (h *ProfileHandler) Create(c *gin.Context) {
	var req dto.CreateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.svc.Create(&req)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, p)
}

func (h *ProfileHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	profiles, total, err := h.svc.List(page, pageSize)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.PageSuccess(c, total, profiles)
}

func (h *ProfileHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	var req dto.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	p, err := h.svc.Update(uint(id), &req)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, p)
}

func (h *ProfileHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

// GetNodeKeys 获取节点关联的所有协议密钥
func (h *ProfileHandler) GetNodeKeys(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	keys, err := h.svc.GetNodeKeys(uint(nodeID))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, keys)
}

// UpsertNodeKey 创建或更新节点协议密钥
func (h *ProfileHandler) UpsertNodeKey(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	profileID, err := strconv.ParseUint(c.Param("profile_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	var req dto.UpsertNodeKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	key, err := h.svc.UpsertNodeKey(uint(nodeID), uint(profileID), &req)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, key)
}

// DeleteNodeKey 删除节点协议密钥
func (h *ProfileHandler) DeleteNodeKey(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	profileID, err := strconv.ParseUint(c.Param("profile_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	if err := h.svc.DeleteNodeKey(uint(nodeID), uint(profileID)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

// ToggleNodeKeyLock 更新节点协议锁定状态
func (h *ProfileHandler) ToggleNodeKeyLock(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	profileID, err := strconv.ParseUint(c.Param("profile_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	var req dto.ToggleNodeKeyLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.svc.SetNodeKeyLocked(uint(nodeID), uint(profileID), req.Locked); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

// KeygenNodeKey 为节点+协议自动生成并存储密钥对（仅支持 vless-reality）
func (h *ProfileHandler) KeygenNodeKey(c *gin.Context) {
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	profileID, err := strconv.ParseUint(c.Param("profile_id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的协议配置ID")
		return
	}
	key, err := h.svc.KeygenForNode(uint(nodeID), uint(profileID))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, key)
}
