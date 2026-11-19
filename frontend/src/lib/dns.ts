import { useQuery } from '@tanstack/react-query'
import request from './axios'
import type { ApiResponse } from '@/types'

export interface DNSSettings {
  enabled: boolean
  configured: boolean
  provider: string
  zone_id: string
}

export interface DNSPreview {
  domain: string
  type: string
  record_id: string
  zone_id: string
  remote_ip: string
  node_ip: string
}

export const dnsStatus = {
  pending: '待同步', checking: '正在核对', synced: '已更新', review: '需核对', rebind: '需重新绑定', failed: '失败',
}

export interface DNSBinding {
  node_id: number
  domain: string
  type: string
  confirmed_ip: string
  desired_ip: string
  status: keyof typeof dnsStatus
  message: string
}

export const dnsApi = {
  settings: () => request.get<ApiResponse<DNSSettings>>('/system/dns').then(r => r.data.data!),
  save: (data: Omit<DNSSettings, 'configured'> & { token: string }) => request.put('/system/dns', data),
  clear: () => request.delete('/system/dns/token'),
  test: () => request.post('/system/dns/test'),
  preview: (id: number) => request.get<ApiResponse<DNSPreview>>(`/nodes/${id}/dns/preview`).then(r => r.data.data!),
  bind: (id: number, data: DNSPreview) => request.put(`/nodes/${id}/dns`, data),
  unbind: (id: number) => request.delete(`/nodes/${id}/dns`),
  retry: (id: number) => request.post(`/nodes/${id}/dns/retry`),
}

export function useDNSBindings() {
  return useQuery({
    queryKey: ['dns-bindings'],
    queryFn: () => request.get<ApiResponse<DNSBinding[]>>('/dns/bindings').then(r => r.data.data ?? []),
    refetchInterval: query => query.state.data?.some(b => b.status === 'pending' || b.status === 'checking') ? 3000 : false,
  })
}
