export interface User {
  id: number
  username: string
  real_name: string
  department: string
  group_id?: number
  group_name?: string
  active: boolean
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
  port: number
  public_key: string
  short_id: string
  sni: string
  ssh_port: number
  ssh_user: string
  ssh_key_path: string
  active: boolean
  sync_status: SyncStatus
  last_check_ok: boolean
  last_latency_ms: number
  last_sync_at?: string
  last_check_at?: string
  remark: string
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
