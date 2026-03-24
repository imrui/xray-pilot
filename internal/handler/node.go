package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/curve25519"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"github.com/imrui/xray-pilot/pkg/response"
)

type NodeHandler struct {
	svc     *service.NodeService
	syncSvc *service.SyncService
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
	node, err := h.svc.UpdateNodeIP(uint(id), &req)
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
	response.Success(c, gin.H{"message": fmt.Sprintf("节点 %s 同步成功", result.Name)})
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

// Keygen 生成 x25519 Reality 密钥对（纯 Go 实现，无需依赖 xray binary）
func (h *NodeHandler) Keygen(c *gin.Context) {
	privateKey, publicKey, err := generateX25519KeyPair()
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
		sshPort = 22
	}
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = "root"
	}
	latencyMs, err := xssh.TestConnectivity(xssh.Config{
		Host:    node.IP,
		Port:    sshPort,
		User:    sshUser,
		KeyPath: node.SSHKeyPath,
	})
	if err != nil {
		response.Success(c, gin.H{"ok": false, "error": err.Error(), "latency_ms": 0})
		return
	}
	// 更新健康检测结果
	_ = h.nodeRepo.UpdateLastCheck(node.ID, true, latencyMs)
	response.Success(c, gin.H{"ok": true, "latency_ms": latencyMs})
}

// generateX25519KeyPair 生成 Xray Reality 使用的 x25519 密钥对（Base64 URL 编码）
func generateX25519KeyPair() (privateKeyB64, publicKeyB64 string, err error) {
	var privateKey [32]byte
	if _, err = rand.Read(privateKey[:]); err != nil {
		return
	}
	// x25519 密钥规范化（RFC 7748）
	privateKey[0] &= 248
	privateKey[31] &= 127
	privateKey[31] |= 64

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	privateKeyB64 = base64.RawURLEncoding.EncodeToString(privateKey[:])
	publicKeyB64 = base64.RawURLEncoding.EncodeToString(publicKey[:])
	return
}
