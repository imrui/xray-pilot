import { useQuery } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import request from './axios'
import { bindingPath } from './routing-model'
import type {
  BindingImpact,
  EffectiveRouting,
  RoutingBinding,
  RoutingOverview,
  RoutingPolicy,
  RoutingRevision,
  RoutingScope,
} from './routing-model'

export interface PreflightDraftRequest {
  policy: RoutingPolicy | Record<string, unknown>
  use_node_overrides?: boolean
  custom_overrides?: Record<string, unknown> | string | null
}

export interface PreflightDraftResponse {
  valid: boolean
  xray_version: string
  requires_ipv6: boolean
  elapsed_ms: number
  warnings: string[]
  redacted_config: string
  error: string
}
export async function routingGet<T>(url: string): Promise<T> {
  return (await request.get<{ data: T }>(url)).data.data
}
export const useRoutingOverview = () =>
  useQuery({
    queryKey: ['routing-overview'],
    queryFn: () => routingGet<RoutingOverview>('/routing-overview'),
    staleTime: 10000,
    refetchInterval: 5000,
  })
export const getBinding = (scope: RoutingScope, id: number) =>
  routingGet<RoutingBinding>(bindingPath(scope, id))
export const getRevisions = (id: number) =>
  routingGet<RoutingRevision[]>(`/routing-templates/${id}/revisions`).then((x) => x ?? [])
export async function previewTemplate(binding: RoutingBinding) {
  return (
    await request.post<{ data: EffectiveRouting }>(
      bindingPath('node', binding.target_id) + '/template-preview',
      { mode: binding.mode, template_id: binding.template_id, revision: binding.revision },
    )
  ).data.data
}
export async function saveBinding(binding: RoutingBinding) {
  return (
    await request.put<{ data: BindingImpact }>(
      bindingPath(binding.scope, binding.target_id),
      binding
    )
  ).data.data
}
export async function previewBinding(binding: RoutingBinding) {
  return (
    await request.post<{ data: BindingImpact }>(
      bindingPath(binding.scope, binding.target_id) + '/preview',
      binding
    )
  ).data.data
}

export interface OutboundTestResult {
  ok: boolean
  elapsed_ms: number
  tcp_rtt_ms?: number | null
  tcp_rtt_error?: string
  checked_at: string
  message: string
}

export async function testNodeOutbound(nodeID: number, tag: string) {
  return (
    await request.post<{ data: OutboundTestResult }>(
      `/nodes/${nodeID}/outbound-test`,
      { tag },
      { timeout: 90000 },
    )
  ).data.data
}

export async function preflightDraft(nodeID: number, payload: PreflightDraftRequest, signal?: AbortSignal) {
  return (
    await request.post<{ data: PreflightDraftResponse }>(
      `/nodes/${nodeID}/preflight-draft`,
      payload,
      { signal },
    )
  ).data.data
}
export async function invalidateRouting(qc: QueryClient) {
  await Promise.all(
    [
      'routing-overview',
      'routing-binding',
      'routing-template-preview',
      'routing-revisions',
      'nodes',
      'sync-summary',
      'previewConfig',
      'routing-effective',
      'config-state',
      'routing-jobs',
    ].map((key) => qc.invalidateQueries({ queryKey: [key] }))
  )
}
