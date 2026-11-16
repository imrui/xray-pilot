package entity

const RoutingPresetDefault = "system-default"
const RoutingPresetAIMedia = "ai-media"

// This catalog is initial content, not a live policy or a maintained service domain database.
type RoutingPreset struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Payload     string `json:"payload"`
}

func RoutingPresets() []RoutingPreset {
	return []RoutingPreset{
		{Key: RoutingPresetDefault, Name: "系统默认路由", Description: "普通直连，不强制地址族；两组域名与自定义规则为空。节点已有覆盖需另行恢复继承。", Payload: DefaultRoutingPolicy},
		{Key: RoutingPresetAIMedia, Name: "AI 工具 / 流媒体分流", Description: "可选参考策略：所列 AI 域名强制 IPv4，所列流媒体域名强制 IPv6，其余普通直连。需可用 IPv6 出口；域名列表并非完整服务域名库，不自动应用。", Payload: `{"schema_version":2,"ipv4_force_domains":["domain:anthropic.com","domain:claude.ai","domain:openai.com","domain:chatgpt.com","domain:x.ai","domain:grok.com","domain:gemini.google.com","domain:aistudio.google.com"],"ipv6_force_domains":["domain:youtube.com","domain:googlevideo.com","domain:netflix.com"],"ipv6_failure_policy":"strict","custom_rules":[]}`},
	}
}
