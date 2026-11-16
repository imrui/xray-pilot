export type RoutingPolicy = {
  enable_ipv6_outbound: boolean
  ipv4_force_domains: string[]
  ipv6_force_domains: string[]
  ipv6_failure_policy: string
  custom_rules: unknown[]
}
export type RoutingTemplate = {
  id: number
  name: string
  description: string
  draft: string
  edit_payload?: string
  version: number
  latest_revision: number
  draft_changed: boolean
  binding_count: number
  node_count: number
}
export type RoutingRevision = {
  revision: number
  payload: string
  actor: string
  created_at: string
}
export type RoutingScope = 'node' | 'group' | 'global' | 'user'
export type RoutingBinding = {
  scope: RoutingScope
  target_id: number
  mode: 'legacy' | 'inherit' | 'template'
  template_id: number
  revision: number
  overrides: string
  udp443_policy: string
  version: number
  config_path: string
}
export type EffectiveRouting = {
  mode: string
  binding: RoutingBinding
  policy: RoutingPolicy | null
  sources: Record<string, string>
  groups: string[]
  template_id: number
  revision: number
  warnings: string[]
}
export type RoutingNode = {
  id: number
  name: string
  ip: string
  active: boolean
  sync_status: string
  job_status?: string
  routing: EffectiveRouting | null
  error?: string
}
export type RoutingOverview = {
  templates: RoutingTemplate[]
  nodes: RoutingNode[]
  references?: {
    scope: string
    target_id: number
    target_name: string
    template_id: number
    revision: number
  }[]
  auto_sync: boolean
}
export type BindingImpact = { changed_nodes: number[]; conflicts: Record<string, string> }
export const emptyPolicy: RoutingPolicy = {
  enable_ipv6_outbound: false,
  ipv4_force_domains: [],
  ipv6_force_domains: [],
  ipv6_failure_policy: 'strict',
  custom_rules: [],
}
export function readPolicy(raw: string): RoutingPolicy {
  const p = JSON.parse(raw) as RoutingPolicy
  return {
    ...p,
    ipv4_force_domains: p.ipv4_force_domains ?? [],
    ipv6_force_domains: p.ipv6_force_domains ?? [],
    custom_rules: p.custom_rules ?? [],
  }
}
export function configConfirmed(
  state: { verified_at?: string; verification: string; runtime_hash: string; disk_hash: string },
  desired: string
): boolean {
  return (
    !!desired &&
    !!state.verified_at &&
    ['protocol', 'service_only', 'grpc_receipt'].includes(state.verification) &&
    state.runtime_hash === desired &&
    state.disk_hash === desired
  )
}
export const domainLines = (text: string) => [
  ...new Set(
    text
      .split(/\r?\n/)
      .map((x) => x.trim())
      .filter(Boolean)
  ),
]
export function parseRules(raw: string): unknown[] {
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    throw new Error('自定义规则不是有效的 JSON，请检查括号和逗号。')
  }
  if (!Array.isArray(value) || value.some((x) => !x || typeof x !== 'object' || Array.isArray(x)))
    throw new Error('自定义规则必须是规则对象组成的数组。')
  return value
}
export function templateBinding(
  binding: RoutingBinding,
  templateID: number,
  revision: number
): RoutingBinding {
  if (!templateID || !revision) throw new Error('请选择已发布的模板版本。')
  // Preserve this node's overrides, path, subscription policy and concurrency version.
  return { ...binding, mode: 'template', template_id: templateID, revision }
}
export function bindingMode(binding: RoutingBinding, mode: RoutingBinding['mode']): RoutingBinding {
  return {
    ...binding,
    mode,
    ...(mode !== 'template' ? { template_id: 0, revision: 0 } : {}),
    ...(mode === 'legacy' ? { overrides: '{}' } : {}),
  }
}
export function routingLabel(node: RoutingNode, templates: RoutingTemplate[]): string {
  if (node.error) return node.error
  const routing = node.routing
  if (!routing) return '来源未加载'
  if (routing.mode === 'legacy') return '保留原有路由'
  const name =
    templates.find((t) => t.id === routing.template_id)?.name ?? `模板 #${routing.template_id}`
  const source =
    routing.mode === 'template'
      ? '节点指定'
      : routing.groups.length
        ? `继承 ${routing.groups.map((g) => g.split(' [')[0]).join('、')}`
        : '继承全局'
  return `${name} v${routing.revision} · ${source}`
}
export const bindingPath = (scope: RoutingScope, id: number) =>
  scope === 'global'
    ? '/settings/routing-default'
    : `/${{ node: 'nodes', group: 'groups', user: 'users' }[scope]}/${id}/routing`
export const affectedIDs = (impact: BindingImpact) => [
  ...new Set([...impact.changed_nodes, ...Object.keys(impact.conflicts).map(Number)]),
]
export function normalizeBinding(binding: RoutingBinding): RoutingBinding {
  const overrides = JSON.parse(binding.overrides || '{}') as Record<string, unknown>
  if (overrides.custom_rules != null) parseRules(JSON.stringify(overrides.custom_rules))
  for (const key of ['ipv4_force_domains', 'ipv6_force_domains']) {
    if (Array.isArray(overrides[key]))
      overrides[key] = domainLines((overrides[key] as string[]).join('\n'))
  }
  return { ...binding, overrides: JSON.stringify(overrides) }
}
export const nodesLink = (ids: number[]) => `/nodes?ids=${ids.join(',')}`
export const jobStatus: Record<string, string> = {
  pending: '等待同步',
  validating: '正在预检',
  staged: '预检通过',
  persisted: '已写入配置',
  applying: '正在同步',
  verifying: '健康验证',
  succeeded: '已同步',
  failed: '同步失败',
  unknown: '需要核对',
  superseded: '配置已更新',
  rolled_back: '已回退，目标未同步',
  rollback_failed: '恢复失败，需处理',
}
