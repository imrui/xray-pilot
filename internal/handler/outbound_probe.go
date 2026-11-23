package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
	"gorm.io/gorm"
)

func testNodeOutbound(c *gin.Context) {
	var req struct {
		Tag string `json:"tag"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求正文仅接受出站 tag"})
		return
	}
	// 路由组中间件已校验 id 是大于零的 uint32。
	nodeID, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	result, err := service.NewSyncService().TestOutbound(c.Request.Context(), uint(nodeID), req.Tag)
	switch {
	case err == nil:
		response.Success(c, result)
	case errors.Is(err, service.ErrOutboundNotConfigured):
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
	case errors.Is(err, service.ErrOutboundProbeBusy), errors.Is(err, service.ErrOutboundRouteUnavailable):
		c.JSON(http.StatusConflict, gin.H{"code": 409, "message": err.Error()})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "节点不存在"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "读取节点配置失败"})
	}
}
