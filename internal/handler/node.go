package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/response"
)

type NodeHandler struct {
	svc      *service.NodeService
	syncSvc  *service.SyncService
	nodeRepo *repository.NodeRepository
}

func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		svc:      service.NewNodeService(),
		syncSvc:  service.NewSyncService(),
		nodeRepo: repository.NewNodeRepository(),
	}
}

func (h *NodeHandler) Create(c *gin.Context) {
	var req dto.CreateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	node, err := h.svc.Create(&req)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, node)
}

func (h *NodeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	nodes, total, err := h.svc.List(page, pageSize)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.PageSuccess(c, total, nodes)
}

func (h *NodeHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	node, err := h.svc.GetByID(uint(id))
	if err != nil {
		response.Fail(c, 404, err.Error())
		return
	}
	response.Success(c, node)
}

func (h *NodeHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	var req dto.UpdateNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	node, err := h.svc.Update(uint(id), &req)
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, node)
}

func (h *NodeHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, nil)
}

func (h *NodeHandler) Toggle(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	if err := h.svc.ToggleActive(uint(id)); err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	c.JSON(http.StatusOK, dto.Response{Code: 0, Message: "ok"})
}

// Sync 同步单个节点
func (h *NodeHandler) Sync(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	result := h.syncSvc.SyncNode(uint(id))
	if !result.Success {
		response.Fail(c, 500, result.Error)
		return
	}
	resp := gin.H{"message": fmt.Sprintf("节点 %s 同步成功", result.Name)}
	if len(result.Warnings) > 0 {
		resp["warnings"] = result.Warnings
	}
	response.Success(c, resp)
}

// PreviewConfig 预览节点生成的 Xray 配置（private_key 已脱敏）
func (h *NodeHandler) PreviewConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	content, warnings, err := h.syncSvc.PreviewConfig(uint(id))
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, gin.H{
		"config":   content,
		"warnings": warnings,
	})
}

// SyncAll 全量同步所有激活节点
func (h *NodeHandler) SyncAll(c *gin.Context) {
	results := h.syncSvc.SyncAll()
	success, failed := 0, 0
	for _, r := range results {
		if r.Success {
			success++
		} else {
			failed++
		}
	}
	response.Success(c, gin.H{
		"total":   len(results),
		"success": success,
		"failed":  failed,
		"results": results,
	})
}

// SyncDrifted 仅同步漂移/失败节点
func (h *NodeHandler) SyncDrifted(c *gin.Context) {
	results := h.syncSvc.SyncDrifted()
	success, failed := 0, 0
	for _, r := range results {
		if r.Success {
			success++
		} else {
			failed++
		}
	}
	response.Success(c, gin.H{
		"total":   len(results),
		"success": success,
		"failed":  failed,
		"results": results,
	})
}

// Keygen 生成 x25519 Reality 密钥对
func (h *NodeHandler) Keygen(c *gin.Context) {
	privateKey, publicKey, err := xray.GenerateX25519KeyPair()
	if err != nil {
		response.Fail(c, 500, fmt.Sprintf("生成密钥失败: %v", err))
		return
	}
	response.Success(c, dto.KeygenResponse{
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	})
}

// TestSSH 测试节点 SSH 连通性
func (h *NodeHandler) TestSSH(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的节点ID")
		return
	}
	node, err := h.nodeRepo.FindByID(uint(id))
	if err != nil {
		response.Fail(c, 404, "节点不存在")
		return
	}

	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = config.Global.SSH.DefaultPort
		if sshPort == 0 {
			sshPort = 22
		}
	}
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = config.Global.SSH.DefaultUser
		if sshUser == "" {
			sshUser = "root"
		}
	}
	keyPath := node.SSHKeyPath
	if keyPath == "" {
		keyPath = config.Global.SSH.DefaultKeyPath
	}

	latencyMs, ok, err := xray.CheckNodeHealth(xray.SSHParams{
		Host:    node.IP,
		Port:    sshPort,
		User:    sshUser,
		KeyPath: keyPath,
	})
	if err != nil || !ok {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		response.Success(c, gin.H{"ok": false, "error": errMsg, "latency_ms": 0})
		return
	}

	_ = h.nodeRepo.UpdateLastCheck(node.ID, true, latencyMs)
	response.Success(c, gin.H{"ok": true, "latency_ms": latencyMs})
}

