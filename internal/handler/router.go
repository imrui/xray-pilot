package handler

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine) {
	userH := NewUserHandler()
	groupH := NewGroupHandler()
	nodeH := NewNodeHandler()
	logH := NewLogHandler()
	authH := NewAuthHandler()
	systemH := NewSystemHandler()
	subH := NewSubscribeHandler()
	setupH := NewSetupHandler()

	// 首次初始化（无需鉴权，有用户后自动关闭）
	r.POST("/api/setup", setupH.Setup)

	// 订阅（无需鉴权）
	r.GET("/sub/:token", subH.Subscribe)

	// 登录（无需鉴权）
	r.POST("/api/auth/login", authH.Login)

	// 需要 JWT 鉴权的路由
	api := r.Group("/api", JWTMiddleware())
	{
		// 用户管理
		api.GET("/users", userH.List)
		api.POST("/users", userH.Create)
		api.PUT("/users/:id", userH.Update)
		api.DELETE("/users/:id", userH.Delete)
		api.PATCH("/users/:id/toggle", userH.Toggle)

		// 分组管理
		api.GET("/groups", groupH.List)
		api.POST("/groups", groupH.Create)
		api.PUT("/groups/:id", groupH.Update)
		api.DELETE("/groups/:id", groupH.Delete)

		// 节点管理
		api.GET("/nodes", nodeH.List)
		api.POST("/nodes", nodeH.Create)
		// 固定路径路由必须在 :id 之前注册，避免路由冲突
		api.POST("/nodes/sync-all", nodeH.SyncAll)
		api.POST("/nodes/sync-drifted", nodeH.SyncDrifted)
		api.POST("/nodes/keygen", nodeH.Keygen)
		api.PUT("/nodes/:id", nodeH.Update)
		api.DELETE("/nodes/:id", nodeH.Delete)
		api.PATCH("/nodes/:id/toggle", nodeH.Toggle)
		api.POST("/nodes/:id/sync", nodeH.Sync)
		api.POST("/nodes/:id/test-ssh", nodeH.TestSSH)

		// 操作日志
		api.GET("/logs", logH.List)

		// 系统配置
		api.GET("/system/config", systemH.GetConfig)
		api.PUT("/system/config", systemH.UpdateConfig)
	}
}
