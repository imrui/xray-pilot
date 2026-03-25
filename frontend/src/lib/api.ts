import request from './axios'
import type { ApiResponse, PageResult, User, Group, Node, InboundProfile, NodeKey, SyncLog } from '@/types'

// ---- 通用分页参数 ----
interface PageParams {
  page?: number
  page_size?: number
}

// ---- 用户 API ----
export const userApi = {
  list: (params?: PageParams) =>
    request.get<ApiResponse<PageResult<User>>>('/users', { params }),

  create: (data: {
    username: string
    real_name?: string
    group_id?: number
    expires_at?: string | null
    remark?: string
  }) => request.post<ApiResponse<User>>('/users', data),

  update: (id: number, data: {
    real_name?: string
    group_id?: number | null
    active?: boolean
    expires_at?: string | null
    remark?: string
  }) => request.put<ApiResponse<User>>(`/users/${id}`, data),

  toggle: (id: number) =>
    request.patch<ApiResponse<null>>(`/users/${id}/toggle`),

  remove: (id: number) =>
    request.delete<ApiResponse<null>>(`/users/${id}`),

  resetUUID: (id: number) =>
    request.post<ApiResponse<User>>(`/users/${id}/reset-uuid`),

  resetToken: (id: number) =>
    request.post<ApiResponse<User>>(`/users/${id}/reset-token`),
}

// ---- 分组 API ----
export const groupApi = {
  list: (params?: PageParams) =>
    request.get<ApiResponse<PageResult<Group>>>('/groups', { params }),

  create: (data: { name: string; description?: string; node_ids?: number[] }) =>
    request.post<ApiResponse<Group>>('/groups', data),

  update: (id: number, data: { name?: string; description?: string; node_ids?: number[] }) =>
    request.put<ApiResponse<Group>>(`/groups/${id}`, data),

  remove: (id: number) =>
    request.delete<ApiResponse<null>>(`/groups/${id}`),
}

// ---- 节点 API ----
export const nodeApi = {
  list: (params?: PageParams) =>
    request.get<ApiResponse<PageResult<Node>>>('/nodes', { params }),

  get: (id: number) =>
    request.get<ApiResponse<Node>>(`/nodes/${id}`),

  create: (data: {
    name: string
    ip: string
    region?: string
    domain?: string
    ssh_port?: number
    ssh_user?: string
    ssh_key_path?: string
    remark?: string
  }) => request.post<ApiResponse<Node>>('/nodes', data),

  update: (id: number, data: Partial<{
    name: string
    region: string
    ip: string
    domain: string
    ssh_port: number
    ssh_user: string
    ssh_key_path: string
    remark: string
  }>) => request.put<ApiResponse<Node>>(`/nodes/${id}`, data),

  toggle: (id: number) =>
    request.patch<ApiResponse<null>>(`/nodes/${id}/toggle`),

  remove: (id: number) =>
    request.delete<ApiResponse<null>>(`/nodes/${id}`),

  sync: (id: number) =>
    request.post<ApiResponse<{ message: string }>>(`/nodes/${id}/sync`),

  syncAll: () =>
    request.post<ApiResponse<{ total: number; success: number; failed: number }>>('/nodes/sync-all'),

  syncDrifted: () =>
    request.post<ApiResponse<{ total: number; success: number; failed: number }>>('/nodes/sync-drifted'),

  keygen: () =>
    request.post<ApiResponse<{ private_key: string; public_key: string }>>('/nodes/keygen'),

  testSSH: (id: number) =>
    request.post<ApiResponse<{ ok: boolean; latency_ms: number; error?: string }>>(`/nodes/${id}/test-ssh`),

  // 节点协议密钥
  getKeys: (nodeId: number) =>
    request.get<ApiResponse<NodeKey[]>>(`/nodes/${nodeId}/keys`),

  upsertKey: (nodeId: number, profileId: number, settings: string) =>
    request.put<ApiResponse<NodeKey>>(`/nodes/${nodeId}/keys/${profileId}`, { settings }),

  deleteKey: (nodeId: number, profileId: number) =>
    request.delete<ApiResponse<null>>(`/nodes/${nodeId}/keys/${profileId}`),
}

// ---- 协议配置 API ----
export const profileApi = {
  list: (params?: PageParams) =>
    request.get<ApiResponse<PageResult<InboundProfile>>>('/profiles', { params }),

  create: (data: {
    name: string
    protocol: string
    port: number
    settings?: string
    active?: boolean
    remark?: string
  }) => request.post<ApiResponse<InboundProfile>>('/profiles', data),

  update: (id: number, data: {
    name?: string
    port?: number
    settings?: string
    active?: boolean
    remark?: string
  }) => request.put<ApiResponse<InboundProfile>>(`/profiles/${id}`, data),

  remove: (id: number) =>
    request.delete<ApiResponse<null>>(`/profiles/${id}`),
}

// ---- 日志 API ----
export const logApi = {
  list: (params?: PageParams) =>
    request.get<ApiResponse<PageResult<SyncLog>>>('/logs', { params }),
}

// ---- 系统配置 API ----
export const systemApi = {
  getConfig: () =>
    request.get<ApiResponse<{
      server: { port: number; mode: string }
      database: { driver: string }
      scheduler: { drift_check_interval: number; health_check_interval: number }
    }>>('/system/config'),

  updateConfig: (data: { drift_check_interval?: number; health_check_interval?: number }) =>
    request.put<ApiResponse<{ message: string }>>('/system/config', data),
}
