import { useQuery } from '@tanstack/react-query'
import type { QueryClient } from '@tanstack/react-query'
import request from './axios'
import { bindingPath } from './routing-model'
import type {
  BindingImpact,
  RoutingBinding,
  RoutingOverview,
  RoutingRevision,
  RoutingScope,
} from './routing-model'
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
export async function invalidateRouting(qc: QueryClient) {
  await Promise.all(
    [
      'routing-overview',
      'routing-binding',
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
