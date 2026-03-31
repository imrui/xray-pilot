export interface User {
  id: number
  username: string
  real_name: string
  group_id?: number
  group_name?: string
  active: boolean
  expires_at?: string  // ISO 8601，空表示永久有效
  remark: string
  subscribe_url: string
  feishu_enabled?: boolean
  feishu_email?: string
  feishu_open_id?: string
  feishu_union_id?: string
  feishu_chat_id?: string
  feishu_identity_ready?: boolean
  feishu_bound_at?: string
  created_at: string
  updated_at: string
}

export interface Group {
  id: number
  name: string
  description: string
  active: boolean
  node_count: number
  node_ids: number[]
  created_at: string
  updated_at: string
}

export type SyncStatus = 'synced' | 'drifted' | 'failed' | 'pending'

export interface Node {
  id: number
  name: string
  region: string
  ip: string
  domain: string          // 客户端连接域名（CDN/中转），空则用 ip
  group_names?: string[]
  online_user_count: number
  ssh_port: number
  ssh_user: string
  ssh_key_path: string
  active: boolean
  xray_active: boolean    // xray 进程是否运行
  xray_version: string    // 远端 xray 版本
  sync_status: SyncStatus
  last_check_ok: boolean
  last_latency_ms: number
  last_sync_at?: string
  last_check_at?: string
  remark: string
  created_at: string
  updated_at: string
}

// InboundProfile 协议接入配置模板
export type Protocol = 'vless-reality' | 'vless-ws-tls' | 'trojan' | 'hysteria2'

export interface InboundProfile {
  id: number
  name: string
  protocol: Protocol
  port: number
  settings: unknown   // JSON object（后端返回 json.RawMessage，已解析为对象）
  active: boolean
  remark: string
  created_at: string
  updated_at: string
}

// 节点密钥材料
export interface NodeKey {
  node_id: number
  profile_id: number
  settings: unknown   // JSON object（编辑节点密钥时返回可直接修改的原值）
  created_at: string
  updated_at: string
}

export interface SyncLog {
  id: number
  action: string
  target: string
  success: boolean
  message: string
  duration_ms: number
  created_at: string
}

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data?: T
}

export interface PageResult<T> {
  total: number
  list: T[]
}

export type DiagnosticStatus = 'ok' | 'warning' | 'error'

export interface DiagnosticItem {
  key: string
  label: string
  status: DiagnosticStatus
  detail: string
  suggestion?: string
  value?: string
}

export interface DiagnosticsResult {
  summary: {
    ok: number
    warning: number
    error: number
  }
  items: DiagnosticItem[]
}

export interface SyncSummary {
  needs_sync: boolean
  drifted_count: number
  failed_count: number
  pending_count: number
  total_affected: number
}

export interface FeishuStatus {
  enabled: boolean
  configured: boolean
  missing_keys?: string[]
  webhook_url?: string
  bot_name?: string
}

export interface FeishuPushResult {
  total: number
  sent: number
  skipped: number
  failed: number
  errors?: string[]
}
