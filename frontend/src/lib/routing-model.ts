export type DomainStrategy = 'AsIs' | 'IPIfNonMatch' | 'IPOnDemand'

export type CustomOutbound = {
  tag: string
  protocol: string
  remark?: string
  sendThrough?: string
  settings?: Record<string, unknown>
  streamSettings?: Record<string, unknown>
  mux?: Record<string, unknown>
  [extra: string]: unknown
}

export type RoutingRuleItem = {
  type?: string
  remark?: string
  disabled?: boolean
  domain?: string[]
  ip?: string[]
  port?: string
  network?: string
  protocol?: string[]
  outboundTag?: string
  balancerTag?: string
  [extra: string]: unknown
}

export type OutboundOverride = {
  mode: 'replace' | 'remap'
  outbound?: CustomOutbound
  target_tag?: string
}

export type BalancerStrategyType = 'random' | 'roundRobin' | 'leastPing'

export type BalancerItem = {
  tag: string
  remark?: string
  selector: string[]
  strategy?: { type?: BalancerStrategyType }
  fallbackTag?: string
  [extra: string]: unknown
}

export type ObservatoryConfig = {
  subjectSelector: string[]
  probeURL?: string
  probeInterval?: string
  enableConcurrency?: boolean
  [extra: string]: unknown
}

export type DNSQueryStrategy = 'UseIP' | 'UseIPv4' | 'UseIPv6'

export type DNSServerObjectItem = {
  address: string
  port?: number
  domains?: string[]
  expectIPs?: string[]
  skipFallback?: boolean
  queryStrategy?: DNSQueryStrategy
  [extra: string]: unknown
}

export type DNSServerEntry = string | DNSServerObjectItem
export type DNSHostValue = string | string[]

export type DNSConfigModel = {
  enabled: boolean
  queryStrategy?: DNSQueryStrategy
  disableCache?: boolean
  disableFallback?: boolean
  disableFallbackIfMatch?: boolean
  hosts?: Record<string, DNSHostValue>
  servers?: DNSServerEntry[]
  [extra: string]: unknown
}

export type RoutingPolicy = {
  schema_version?: number
  domain_strategy?: DomainStrategy
  enable_ipv6_outbound?: boolean // Historical schema 1 only.
  ipv4_force_domains: string[]
  ipv6_force_domains: string[]
  ipv6_failure_policy: string
  custom_outbounds?: CustomOutbound[]
  balancers?: BalancerItem[]
  observatory?: ObservatoryConfig | null
  dns?: DNSConfigModel | null
  custom_rules: RoutingRuleItem[]
}

export type RoutingTemplate = {
  id: number
  preset_key?: string
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

export type RoutingScope = 'node' | 'group' | 'global'
export const basicRoutingLabel = '基础路由（不使用模板）'
export const basicRoutingDescription =
  '按面板基础规则生成配置，保留系统保护和协议规则，普通流量直连；不应用路由模板或节点路由覆盖。'

export type RoutingBinding = {
  scope: RoutingScope
  target_id: number
  mode: 'legacy' | 'inherit' | 'template'
  template_id: number
  revision: number
  overrides: string
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
  presets?: { key: string; name: string; description: string; payload: string }[]
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

export const BUILTIN_OUTBOUNDS = [
  { tag: 'direct', label: 'direct（默认直连）' },
  { tag: 'block', label: 'block（黑洞拦截）' },
  { tag: 'direct-v4', label: 'direct-v4（强制 IPv4 直连）' },
  { tag: 'direct-v6', label: 'direct-v6（强制 IPv6 直连）' },
] as const

export const STRUCTURED_OUTBOUND_PROTOCOLS = [
  { value: 'socks', label: 'SOCKS5 代理落地' },
  { value: 'http', label: 'HTTP 代理落地' },
  { value: 'freedom', label: 'Freedom（指定出口 IP / 策略）' },
  { value: 'wireguard', label: 'WireGuard（WARP / 隧道）' },
  { value: 'vless', label: 'VLESS 远端节点' },
] as const

const RESERVED_OUTBOUND_TAGS = new Set([
  'direct',
  'block',
  'direct-v4',
  'direct-v6',
  'api',
  'api-inbound',
  'dns-out',
])

const RESERVED_INBOUND_PREFIXES = [
  'vless-reality-',
  'vless-ws-',
  'trojan-',
  'http-proxy-',
  'socks-proxy-',
]

export function isStructuredOutboundProtocol(protocol: string): boolean {
  return ['socks', 'http', 'freedom', 'wireguard', 'vless'].includes(protocol)
}

export function validateOutboundTag(tag: string): string {
  if (!tag.trim()) return '请填写出站 Tag'
  if (!/^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$/.test(tag)) {
    return 'Tag 仅支持字母、数字、下划线或连字符，且须以字母/数字开头（最长 63 字符）'
  }
  if (
    RESERVED_OUTBOUND_TAGS.has(tag) ||
    RESERVED_INBOUND_PREFIXES.some((prefix) => tag.startsWith(prefix))
  ) {
    return `Tag "${tag}" 为面板系统保留名称，请更换名称`
  }
  return ''
}

export function isEncryptedSecret(value: unknown): boolean {
  return typeof value === 'string' && value.startsWith('enc:v1:')
}

export function extractDialerProxy(outbound: CustomOutbound): string {
  const stream = outbound.streamSettings
  if (!stream || typeof stream !== 'object') return ''
  const sockopt = stream.sockopt as Record<string, unknown> | undefined
  if (!sockopt || typeof sockopt !== 'object') return ''
  return typeof sockopt.dialerProxy === 'string' ? sockopt.dialerProxy : ''
}

export function patchDialerProxy(outbound: CustomOutbound, dialerProxy: string): CustomOutbound {
  const prevStream =
    outbound.streamSettings && typeof outbound.streamSettings === 'object'
      ? { ...outbound.streamSettings }
      : {}
  const prevSockopt =
    prevStream.sockopt && typeof prevStream.sockopt === 'object'
      ? { ...(prevStream.sockopt as Record<string, unknown>) }
      : {}

  if (dialerProxy) {
    prevSockopt.dialerProxy = dialerProxy
    prevStream.sockopt = prevSockopt
  } else {
    delete prevSockopt.dialerProxy
    if (Object.keys(prevSockopt).length > 0) {
      prevStream.sockopt = prevSockopt
    } else {
      delete prevStream.sockopt
    }
  }

  const next = { ...outbound }
  if (Object.keys(prevStream).length > 0) {
    next.streamSettings = prevStream
  } else {
    delete next.streamSettings
  }
  return next
}

export function createDefaultOutbound(protocol: string, index: number): CustomOutbound {
  const tag = `${protocol}-out-${index}`
  switch (protocol) {
    case 'socks':
    case 'http':
      return {
        tag,
        protocol,
        remark: '',
        settings: {
          servers: [{ address: '', port: 1080 }],
        },
      }
    case 'freedom':
      return {
        tag,
        protocol: 'freedom',
        remark: '',
        sendThrough: '',
        settings: { domainStrategy: 'AsIs' },
      }
    case 'wireguard':
      return {
        tag,
        protocol: 'wireguard',
        remark: '',
        settings: {
          secretKey: '',
          address: ['172.16.0.2/32'],
          peers: [
            {
              endpoint: 'engage.cloudflareclient.com:2408',
              publicKey: '',
              keepAlive: 25,
            },
          ],
          mtu: 1280,
        },
      }
    case 'vless':
      return {
        tag,
        protocol: 'vless',
        remark: '',
        settings: {
          vnext: [
            {
              address: '',
              port: 443,
              users: [{ id: '', encryption: 'none' }],
            },
          ],
        },
        streamSettings: {
          network: 'tcp',
          security: 'tls',
          tlsSettings: { serverName: '' },
        },
      }
    default:
      return {
        tag,
        protocol: protocol || 'socks',
        settings: {},
      }
  }
}

export function createDefaultDNSConfig(): DNSConfigModel {
  return {
    enabled: true,
    queryStrategy: 'UseIP',
    servers: ['https://1.1.1.1/dns-query'],
  }
}

export function countOutboundReferences(
  tag: string,
  rules: RoutingRuleItem[],
  outbounds: CustomOutbound[],
  balancers: BalancerItem[] = [],
): number {
  let count = 0
  for (const r of rules) {
    if (r.outboundTag === tag) count += 1
  }
  for (const o of outbounds) {
    if (o.tag !== tag && extractDialerProxy(o) === tag) count += 1
  }
  for (const b of balancers) {
    if (b.fallbackTag === tag || (b.selector ?? []).some((sel) => sel && tag.startsWith(sel))) {
      count += 1
    }
  }
  return count
}

export function countBalancerReferences(tag: string, rules: RoutingRuleItem[]): number {
  let count = 0
  for (const r of rules) {
    if (r.balancerTag === tag) count += 1
  }
  return count
}

export const emptyPolicy: RoutingPolicy = {
  schema_version: 3,
  domain_strategy: 'IPIfNonMatch',
  ipv4_force_domains: [],
  ipv6_force_domains: [],
  ipv6_failure_policy: 'strict',
  custom_outbounds: [],
  custom_rules: [],
}

export function readPolicy(raw: string): RoutingPolicy {
  const p = JSON.parse(raw) as RoutingPolicy
  return {
    ...p,
    schema_version: p.schema_version || 1,
    domain_strategy: p.domain_strategy || (p.schema_version && p.schema_version >= 3 ? 'IPIfNonMatch' : undefined),
    ipv4_force_domains: p.ipv4_force_domains ?? [],
    ipv6_force_domains: p.ipv6_force_domains ?? [],
    custom_outbounds: p.custom_outbounds ?? (p.schema_version && p.schema_version >= 3 ? [] : undefined),
    custom_rules: (p.custom_rules ?? []).map((r) => ({
      type: 'field',
      ...r,
    })),
  }
}

export function upgradeRoutingPolicy(raw: string): RoutingPolicy {
  const parsed = JSON.parse(raw) as Record<string, unknown>
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('路由策略必须是 JSON 对象。')
  }
  const p = readPolicy(raw)
  const ver = p.schema_version ?? 1
  const moveToV4 = ver < 2 && !p.enable_ipv6_outbound
  const strategy: DomainStrategy =
    p.domain_strategy === 'AsIs' || p.domain_strategy === 'IPOnDemand' || p.domain_strategy === 'IPIfNonMatch'
      ? p.domain_strategy
      : 'IPIfNonMatch'

  const next: RoutingPolicy = {
    schema_version: 3,
    domain_strategy: strategy,
    ipv4_force_domains: [
      ...new Set([...p.ipv4_force_domains, ...(moveToV4 ? p.ipv6_force_domains : [])]),
    ],
    ipv6_force_domains: moveToV4 ? [] : p.ipv6_force_domains,
    ipv6_failure_policy: p.ipv6_failure_policy || 'strict',
    custom_outbounds: Array.isArray(p.custom_outbounds) ? p.custom_outbounds : [],
    ...(p.balancers && p.balancers.length > 0 ? { balancers: p.balancers } : {}),
    ...(p.observatory ? { observatory: p.observatory } : {}),
    ...(p.dns ? { dns: p.dns } : {}),
    custom_rules: Array.isArray(p.custom_rules)
      ? p.custom_rules.map((r) => ({ type: 'field', ...r }))
      : [],
  }
  return next
}

function isIPv6HostLiteral(rawHost: string): boolean {
  let h = rawHost.trim()
  if (!h) return false
  if (h.startsWith('[')) {
    const closeIdx = h.indexOf(']')
    if (closeIdx > 1) h = h.slice(1, closeIdx)
  } else if ((h.match(/:/g)?.length ?? 0) === 1) {
    // host:port (IPv4 or domain), not an IPv6 literal
    return false
  }
  return h.includes(':') && /^[0-9a-fA-F:.]+$/.test(h) && !h.toLowerCase().startsWith('::ffff:')
}

function outboundNeedsHostIPv6(co: CustomOutbound): boolean {
  if (co.sendThrough && isIPv6HostLiteral(co.sendThrough)) return true
  const settings = co.settings
  if (!settings || typeof settings !== 'object') return false
  if (co.protocol === 'freedom' && settings.domainStrategy === 'ForceIPv6') return true
  if (Array.isArray(settings.servers)) {
    for (const s of settings.servers) {
      if (s && typeof s === 'object' && typeof (s as { address?: string }).address === 'string') {
        if (isIPv6HostLiteral((s as { address: string }).address)) return true
      }
    }
  }
  if (Array.isArray(settings.vnext)) {
    for (const v of settings.vnext) {
      if (v && typeof v === 'object' && typeof (v as { address?: string }).address === 'string') {
        if (isIPv6HostLiteral((v as { address: string }).address)) return true
      }
    }
  }
  if (co.protocol === 'wireguard' && Array.isArray(settings.peers)) {
    for (const peer of settings.peers) {
      const ep = peer && typeof peer === 'object' ? (peer as { endpoint?: string }).endpoint : ''
      if (typeof ep === 'string' && ep.startsWith('[') && isIPv6HostLiteral(ep)) return true
    }
  }
  return false
}

export function requiresIPv6(policy: RoutingPolicy): boolean {
  const ver = policy.schema_version ?? 1
  if (ver < 2) return !!policy.enable_ipv6_outbound
  if ((policy.ipv6_force_domains ?? []).length > 0) return true

  const outbounds = policy.custom_outbounds ?? []
  const balancers = policy.balancers ?? []
  const byTag = new Map(outbounds.map((o) => [o.tag, o]))
  const balancerByTag = new Map(balancers.map((b) => [b.tag, b]))
  const allCandidateTags = ['direct', 'direct-v4', 'direct-v6', ...outbounds.map((o) => o.tag)]
  const visited = new Set<string>()
  const queue: string[] = []
  const enqueue = (tag?: string) => {
    if (tag && !visited.has(tag)) {
      visited.add(tag)
      queue.push(tag)
    }
  }

  for (const r of policy.custom_rules ?? []) {
    if (r.disabled) continue
    enqueue(r.outboundTag)
    if (r.balancerTag) {
      const bal = balancerByTag.get(r.balancerTag)
      if (bal) {
        if (bal.fallbackTag) enqueue(bal.fallbackTag)
        for (const sel of bal.selector ?? []) {
          if (!sel) continue
          for (const candidate of allCandidateTags) {
            if (candidate && candidate.startsWith(sel)) enqueue(candidate)
          }
        }
      }
    }
  }

  while (queue.length > 0) {
    const curr = queue.shift()!
    if (curr === 'direct-v6') return true
    const co = byTag.get(curr)
    if (co) {
      if (outboundNeedsHostIPv6(co)) return true
      const dp = extractDialerProxy(co)
      if (dp) enqueue(dp)
    }
  }
  return false
}

export function revisionSummaryLabel(payload: string): string {
  const p = readPolicy(payload)
  const ver = p.schema_version ?? 1
  if (ver < 2) return '旧版 IPv4 兜底'
  const parts = ['默认直连']
  const outCount = p.custom_outbounds?.length ?? 0
  const balCount = p.balancers?.length ?? 0
  if (outCount > 0) parts.push(`${outCount} 个自定义出站`)
  if (balCount > 0) parts.push(`${balCount} 个负载均衡`)
  if (p.dns?.enabled) parts.push('自定义 DNS')
  return parts.join(' · ')
}

export function configConfirmed(
  state: { verified_at?: string; verification: string; runtime_hash: string; disk_hash: string },
  desired: string,
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
      .filter(Boolean),
  ),
]

export function parseRules(raw: string): RoutingRuleItem[] {
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    throw new Error('自定义规则不是有效的 JSON，请检查括号和逗号。')
  }
  if (!Array.isArray(value) || value.some((x) => !x || typeof x !== 'object' || Array.isArray(x)))
    throw new Error('自定义规则必须是规则对象组成的数组。')
  return value as RoutingRuleItem[]
}

export function parseSingleObject<T extends Record<string, unknown>>(raw: string, label: string): T {
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch {
    throw new Error(`${label}不是有效的 JSON，请检查括号与逗号。`)
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`${label}必须是一个 JSON 对象。`)
  }
  return value as T
}

export function templateBinding(
  binding: RoutingBinding,
  templateID: number,
  revision: number,
): RoutingBinding {
  if (!templateID || !revision) throw new Error('请选择已发布的模板版本。')
  // Preserve this node's overrides, path and concurrency version.
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
  if (routing.mode === 'legacy') return basicRoutingLabel
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
    : `/${{ node: 'nodes', group: 'groups' }[scope]}/${id}/routing`

export const affectedIDs = (impact: BindingImpact) => [
  ...new Set([...impact.changed_nodes, ...Object.keys(impact.conflicts).map(Number)]),
]

export function normalizeBinding(binding: RoutingBinding): RoutingBinding {
  const overrides = JSON.parse(binding.overrides || '{}') as Record<string, unknown>
  if (overrides.custom_rules != null) parseRules(JSON.stringify(overrides.custom_rules))
  for (const key of ['ipv4_force_domains', 'ipv6_force_domains']) {
    if (Array.isArray(overrides[key])) {
      const domains = domainLines((overrides[key] as string[]).join('\n'))
      if (overrides[key].length > 0 && domains.length === 0)
        throw new Error(
          `${key === 'ipv4_force_domains' ? 'IPv4' : 'IPv6'} 自定义列表不能为空；如需移除该组域名规则，请选择“清空规则”。`,
        )
      overrides[key] = domains
    }
  }
  if (overrides.outbound_overrides != null) {
    if (typeof overrides.outbound_overrides !== 'object' || Array.isArray(overrides.outbound_overrides)) {
      throw new Error('节点出站覆盖格式无效。')
    }
    const entries = overrides.outbound_overrides as Record<string, OutboundOverride>
    const cleaned: Record<string, OutboundOverride> = {}
    for (const [tag, ov] of Object.entries(entries)) {
      if (!ov) continue
      if (ov.mode === 'remap') {
        if (!ov.target_tag) throw new Error(`出站「${tag}」重映射必须选择目标出站。`)
        cleaned[tag] = { mode: 'remap', target_tag: ov.target_tag }
      } else if (ov.mode === 'replace') {
        if (!ov.outbound || typeof ov.outbound !== 'object') {
          throw new Error(`出站「${tag}」自定义参数无效。`)
        }
        cleaned[tag] = { mode: 'replace', outbound: { ...ov.outbound, tag } }
      }
    }
    if (Object.keys(cleaned).length > 0) {
      overrides.outbound_overrides = cleaned
    } else {
      delete overrides.outbound_overrides
    }
  }
  if (overrides.dns_override != null) {
    if (typeof overrides.dns_override !== 'object' || Array.isArray(overrides.dns_override)) {
      throw new Error('节点 DNS 覆盖格式无效。')
    }
    const dnsOv = overrides.dns_override as DNSConfigModel
    if (dnsOv.enabled === false) {
      overrides.dns_override = { enabled: false }
    } else if (dnsOv.enabled === true) {
      if (!Array.isArray(dnsOv.servers) || dnsOv.servers.length === 0) {
        throw new Error('启用本节点自定义 DNS 时请至少配置 1 个上游 DNS 服务器。')
      }
      overrides.dns_override = dnsOv
    } else {
      delete overrides.dns_override
    }
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
