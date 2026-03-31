package dto

// Response 通用 API 响应
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// PageResult 通用分页结果
type PageResult struct {
	Total int64 `json:"total"`
	List  any   `json:"list"`
}

// LoginRequest 管理员登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 管理员登录响应
type LoginResponse struct {
	Token string `json:"token"`
}

// SyncSummaryResponse 后台全局待同步摘要
type SyncSummaryResponse struct {
	NeedsSync     bool `json:"needs_sync"`
	DriftedCount  int  `json:"drifted_count"`
	FailedCount   int  `json:"failed_count"`
	PendingCount  int  `json:"pending_count"`
	TotalAffected int  `json:"total_affected"`
}

// FeishuStatusResponse 飞书配置状态摘要
type FeishuStatusResponse struct {
	Enabled     bool     `json:"enabled"`
	Configured  bool     `json:"configured"`
	MissingKeys []string `json:"missing_keys,omitempty"`
	WebhookURL  string   `json:"webhook_url,omitempty"`
	BotName     string   `json:"bot_name,omitempty"`
}

// FeishuPushResponse 飞书推送结果统计
type FeishuPushResponse struct {
	Total   int      `json:"total"`
	Sent    int      `json:"sent"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}
