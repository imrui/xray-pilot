package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/response"
	"strconv"
)

func listNodeSyncLogs(c *gin.Context) {
	q := repository.DB.Model(&entity.SyncLog{}).Where("action = ?", "sync")
	if raw := c.Query("node_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || id == 0 {
			response.BadRequest(c, "无效节点 ID")
			return
		}
		q = q.Where("target LIKE ? OR target = ?", fmt.Sprintf("%%(%d)", id), fmt.Sprintf("node:%d", id))
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if size < 1 || size > 100 {
		size = 20
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	rows := []entity.SyncLog{}
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		response.Fail(c, 500, err.Error())
		return
	}
	response.PageSuccess(c, total, rows)
}
