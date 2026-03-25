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
  created_at: string
  updated_at: string
}

export interface Group {
  id: number
  name: string
  description: string
  active: boolean
  node_count: number
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
  settings: string   // JSON 字符串
  active: boolean
  remark: string
  created_at: string
  updated_at: string
}

// 节点密钥材料
export interface NodeKey {
  node_id: number
  profile_id: number
  settings: string   // JSON 字符串（private_key 已脱敏为 "***"）
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
