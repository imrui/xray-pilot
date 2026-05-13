package handler

import (
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// BackupHandler 数据库备份接口
type BackupHandler struct {
	svc *service.BackupService
}

func NewBackupHandler() *BackupHandler {
	return &BackupHandler{svc: service.NewBackupService()}
}

// List GET /api/system/backups
func (h *BackupHandler) List(c *gin.Context) {
	backups, err := h.svc.ListBackups()
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, backups)
}

// Run POST /api/system/backups
// 手动触发一次备份
func (h *BackupHandler) Run(c *gin.Context) {
	file, err := h.svc.RunBackup()
	if err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.Success(c, file)
}

// Download GET /api/system/backups/:name/download
// 二进制下载备份文件
func (h *BackupHandler) Download(c *gin.Context) {
	name := c.Param("name")
	full, err := h.svc.ResolveBackupPath(name)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	c.FileAttachment(full, filepath.Base(full))
}

// Delete DELETE /api/system/backups/:name
func (h *BackupHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.svc.DeleteBackup(name); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"deleted": name})
}
