package handler

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册所有路由
func RegisterRoutes(r *gin.Engine) {
	userH := NewUserHandler()
	groupH := NewGroupHandler()
	nodeH := NewNodeHandler()
	profileH := NewProfileHandler()
	logH := NewLogHandler()
	authH := NewAuthHandler()
	systemH := NewSystemHandler()
	subH := NewSubscribeHandler()
	feishuH := NewFeishuHandler()

	// 订阅（无需鉴权）
	r.GET("/sub/:token", subH.Subscribe)
	r.POST("/api/feishu/events", feishuH.Events)

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
		api.POST("/users/:id/reset-uuid", userH.ResetUUID)
		api.POST("/users/:id/reset-token", userH.ResetToken)
		api.POST("/users/:id/feishu-push", feishuH.PushUser)
		api.POST("/users/:id/feishu-bind", feishuH.BindUser)
		api.POST("/users/:id/feishu-unbind", feishuH.UnbindUser)
		api.POST("/users/feishu-push", feishuH.PushUsers)

		// 分组管理
		api.GET("/groups", groupH.List)
		api.POST("/groups", groupH.Create)
		api.PUT("/groups/:id", groupH.Update)
		api.DELETE("/groups/:id", groupH.Delete)

		// 节点管理
		api.GET("/nodes", nodeH.List)
		api.POST("/nodes", nodeH.Create)
		// 固定路径路由必须在 :id 之前注册
		api.POST("/nodes/sync-all", nodeH.SyncAll)
		api.POST("/nodes/sync-drifted", nodeH.SyncDrifted)
		api.POST("/nodes/keygen", nodeH.Keygen)
		api.GET("/nodes/:id", nodeH.Get)
		api.PUT("/nodes/:id", nodeH.Update)
		api.DELETE("/nodes/:id", nodeH.Delete)
		api.PATCH("/nodes/:id/toggle", nodeH.Toggle)
		api.POST("/nodes/:id/sync", nodeH.Sync)
		api.GET("/nodes/:id/preview-config", nodeH.PreviewConfig)
		api.POST("/nodes/:id/test-ssh", nodeH.TestSSH)
		// 节点协议密钥管理（静态路径 keygen 必须在动态 :profile_id 之前）
		api.GET("/nodes/:id/keys", profileH.GetNodeKeys)
		api.POST("/nodes/:id/keys/:profile_id/keygen", profileH.KeygenNodeKey)
		api.PUT("/nodes/:id/keys/:profile_id", profileH.UpsertNodeKey)
		api.PATCH("/nodes/:id/keys/:profile_id/lock", profileH.ToggleNodeKeyLock)
		api.DELETE("/nodes/:id/keys/:profile_id", profileH.DeleteNodeKey)

		// 协议接入配置（InboundProfile）
		api.GET("/profiles", profileH.List)
		api.POST("/profiles", profileH.Create)
		api.PUT("/profiles/:id", profileH.Update)
		api.DELETE("/profiles/:id", profileH.Delete)

		// 操作日志
		api.GET("/logs", logH.List)
		api.POST("/logs/cleanup", logH.Cleanup)

		// 系统信息（只读）
		api.GET("/system/info", systemH.GetSystemInfo)
		api.GET("/system/diagnostics", systemH.GetDiagnostics)
		api.GET("/system/sync-summary", systemH.GetSyncSummary)
		api.GET("/system/feishu-status", systemH.GetFeishuStatus)
		api.POST("/system/feishu-test", systemH.TestFeishuConfig)
		// 运行时配置（KV，可读写）
		api.GET("/system/settings", systemH.GetSettings)
		api.PUT("/system/settings", systemH.UpdateSettings)
	}
}
