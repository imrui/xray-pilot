package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func dnsResult(c *gin.Context, data any, err error) {
	if err == nil {
		response.Success(c, data)
		return
	}
	var validation service.DNSValidationError
	var upstream *service.DNSUpstreamError
	var status int
	message := err.Error()
	switch {
	case errors.As(err, &validation):
		status = http.StatusBadRequest
	case errors.As(err, &upstream):
		status = http.StatusBadGateway
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, message = http.StatusNotFound, "节点不存在"
	default:
		status, message = http.StatusInternalServerError, "DNS 操作失败，请查看服务日志"
		zap.L().Error("DNS 操作失败", zap.Error(err))
	}
	c.JSON(status, dto.Response{Code: status, Message: message})
}

func registerDNSRoutes(api *gin.RouterGroup) {
	svc := service.NewDNSService()
	api.GET("/dns/bindings", func(c *gin.Context) { v, err := svc.Bindings(); dnsResult(c, v, err) })
	settings := api.Group("/system/dns", RequireSuperAdmin())
	settings.GET("", func(c *gin.Context) { v, err := svc.Settings(); dnsResult(c, v, err) })
	settings.PUT("", func(c *gin.Context) {
		var in service.DNSSettingsInput
		if err := c.ShouldBindJSON(&in); err != nil {
			dnsResult(c, nil, service.DNSValidationError("DNS 设置格式无效"))
			return
		}
		dnsResult(c, nil, svc.SaveSettings(in, false, actorFrom(c)))
	})
	settings.DELETE("/token", func(c *gin.Context) {
		dnsResult(c, nil, svc.SaveSettings(service.DNSSettingsInput{}, true, actorFrom(c)))
	})
	settings.POST("/test", func(c *gin.Context) { dnsResult(c, nil, svc.TestConnection(c.Request.Context())) })
	node := api.Group("/nodes/:id/dns")
	withNode := func(fn func(*gin.Context, uint)) gin.HandlerFunc {
		return func(c *gin.Context) {
			id, err := strconv.ParseUint(c.Param("id"), 10, 32)
			if err != nil || id == 0 {
				dnsResult(c, nil, service.DNSValidationError("无效节点 ID"))
				return
			}
			fn(c, uint(id))
		}
	}
	node.GET("", withNode(func(c *gin.Context, id uint) { v, err := svc.Binding(id); dnsResult(c, v, err) }))
	admin := node.Group("", RequireSuperAdmin())
	admin.GET("/preview", withNode(func(c *gin.Context, id uint) { v, err := svc.Preview(c.Request.Context(), id); dnsResult(c, v, err) }))
	admin.PUT("", withNode(func(c *gin.Context, id uint) {
		var p service.DNSPreview
		if err := c.ShouldBindJSON(&p); err != nil {
			dnsResult(c, nil, service.DNSValidationError("请先预览记录"))
			return
		}
		dnsResult(c, nil, svc.Bind(c.Request.Context(), id, p, actorFrom(c)))
	}))
	admin.DELETE("", withNode(func(c *gin.Context, id uint) { dnsResult(c, nil, svc.Unbind(id, actorFrom(c))) }))
	admin.POST("/retry", withNode(func(c *gin.Context, id uint) { dnsResult(c, nil, svc.Retry(id, actorFrom(c))) }))
}
