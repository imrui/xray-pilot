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
  source?: string[] | null
  inboundTag?: string[] | null
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

const KNOWN_RULE_KEYS = new Set([
  'type',
  'remark',
  'disabled',
  'domain',
  'ip',
  'source',
  'inboundTag',
  'port',
  'network',
  'protocol',
  'outboundTag',
  'balancerTag',
])

function isPublicProbeIPv4(a: number, b: number, c: number): boolean {
  if (a === 0 || a === 10 || a === 127 || a >= 224) return false
  if (a === 169 && b === 254) return false
  if (a === 172 && b >= 16 && b <= 31) return false
  if (a === 192 && b === 168) return false
  if (a === 100 && (b & 0xc0) === 64) return false
  if (a === 192 && b === 0 && (c === 0 || c === 2)) return false
  if (a === 198 && (b === 18 || b === 19)) return false
  if (a === 198 && b === 51 && c === 100) return false
  if (a === 203 && b === 0 && c === 113) return false
  return true
}

function isPublicProbeIPv6(val: bigint): boolean {
  if ((val >> 125n) !== 1n) return false
  if (
    ipv6MatchesRuleTarget(val, '2001::/23') ||
    ipv6MatchesRuleTarget(val, '2001:db8::/32') ||
    ipv6MatchesRuleTarget(val, '2002::/16')
  ) {
    return false
  }
  return true
}

export function isValidObservatoryProbeURL(raw: string): boolean {
  const s = raw.trim()
  if (s !== raw || !/^https:\/\//i.test(s)) return false
  for (let i = 0; i < s.length; i++) {
    const code = s.charCodeAt(i)
    if (code <= 0x1f || code === 0x7f) return false
    if (code === 0x25 /* % */) {
      if (i + 2 >= s.length || !/^[0-9a-fA-F]{2}$/.test(s.slice(i + 1, i + 3))) {
        return false
      }
    }
  }
  let parsedURL: URL
  try {
    parsedURL = new URL(s)
  } catch {
    return false
  }
  if (
    parsedURL.protocol !== 'https:' ||
    parsedURL.username ||
    parsedURL.password ||
    parsedURL.search ||
    parsedURL.hash
  ) {
    return false
  }
  const rest = s.slice('https://'.length)
  const pathIdx = rest.search(/[/?#]/)
  const authority = pathIdx === -1 ? rest : rest.slice(0, pathIdx)
  if (!authority || authority.includes('@') || authority.includes('%')) return false

  if (authority.startsWith('[')) {
    const closeIdx = authority.indexOf(']')
    if (closeIdx <= 1) return false
    const rawHost = authority.slice(1, closeIdx)
    const after = authority.slice(closeIdx + 1)
    if (after !== '' && after !== ':' && after !== ':443') return false
    const v6 = parseIPv6BigInt(rawHost)
    if (!v6) return false
    if (v6.ipv4Mapped) {
      const low32 = Number(v6.value & 0xffffffffn)
      return isPublicProbeIPv4(
        (low32 >>> 24) & 0xff,
        (low32 >>> 16) & 0xff,
        (low32 >>> 8) & 0xff,
      )
    }
    return isPublicProbeIPv6(v6.value)
  }

  const parts = authority.split(':')
  if (parts.length > 2) return false
  if (parts.length === 2 && parts[1] !== '' && parts[1] !== '443') return false
  const v4 = parseIPv4Octets(parts[0])
  if (!v4) return false
  return isPublicProbeIPv4(v4[0], v4[1], v4[2])
}
const ALLOWED_DNS_URI_PREFIXES = [
  'https://',
  'https+local://',
  'tcp://',
  'tcp+local://',
  'quic+local://',
]

function parseIPv4Octets(s: string): [number, number, number, number] | null {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(s)
  if (!m) return null
  const octets: number[] = []
  for (let i = 1; i <= 4; i++) {
    const part = m[i]
    if (part.length > 1 && part.startsWith('0')) return null
    const n = Number(part)
    if (n < 0 || n > 255) return null
    octets.push(n)
  }
  return octets as [number, number, number, number]
}

function parseIPv6BigInt(raw: string): { value: bigint; ipv4Mapped: boolean } | null {
  let s = raw.trim()
  if (!s) return null
  if (s.startsWith('[') && s.endsWith(']')) {
    s = s.slice(1, -1)
  }
  if (!s.includes(':')) return null
  const halves = s.split('::')
  if (halves.length > 2) return null

  const parseSide = (side: string, allowTrailingV4: boolean): number[] | null => {
    if (!side) return []
    const parts = side.split(':')
    const groups: number[] = []
    for (let i = 0; i < parts.length; i++) {
      const p = parts[i]
      if (allowTrailingV4 && i === parts.length - 1 && p.includes('.')) {
        const v4 = parseIPv4Octets(p)
        if (!v4) return null
        groups.push((v4[0] << 8) | v4[1], (v4[2] << 8) | v4[3])
      } else {
        if (!/^[0-9a-fA-F]{1,4}$/.test(p)) return null
        groups.push(parseInt(p, 16))
      }
    }
    return groups
  }

  let groups: number[]
  if (halves.length === 1) {
    const g = parseSide(halves[0], true)
    if (!g || g.length !== 8) return null
    groups = g
  } else {
    const left = parseSide(halves[0], false)
    const right = parseSide(halves[1], true)
    if (!left || !right || left.length + right.length >= 8) return null
    const pad = new Array<number>(8 - left.length - right.length).fill(0)
    groups = [...left, ...pad, ...right]
  }

  let val = 0n
  for (const g of groups) {
    val = (val << 16n) | BigInt(g)
  }
  const ipv4Mapped =
    groups[0] === 0 &&
    groups[1] === 0 &&
    groups[2] === 0 &&
    groups[3] === 0 &&
    groups[4] === 0 &&
    groups[5] === 0xffff
  return { value: val, ipv4Mapped }
}

function classifyIPLiteral(raw: string): 'v4' | 'v6' | null {
  const s = raw.trim()
  if (!s) return null
  if (parseIPv4Octets(s)) return 'v4'
  const v6 = parseIPv6BigInt(s)
  if (!v6) return null
  return v6.ipv4Mapped ? 'v4' : 'v6'
}

function strategyNeedsIPv6(strategy: string): boolean {
  const lower = strategy.toLowerCase()
  return lower === 'forceipv6' || lower === 'useipv6'
}

function localDialNeedsIPv6(address: string, sendThrough: string, strategy: string): boolean {
  if (classifyIPLiteral(sendThrough) === 'v6' || classifyIPLiteral(address) === 'v6') {
    return true
  }
  return classifyIPLiteral(address) === null && strategyNeedsIPv6(strategy)
}

function splitEndpointHost(endpoint: string): string | null {
  const s = endpoint.trim()
  if (!s) return null
  if (s.startsWith('[')) {
    const close = s.indexOf(']:')
    if (close <= 1) return null
    return s.slice(1, close)
  }
  const lastColon = s.lastIndexOf(':')
  if (lastColon <= 0 || s.indexOf(':') !== lastColon) return null
  return s.slice(0, lastColon)
}

function matchOutboundSelectors(selectors: string[], candidates: string[]): string[] {
  const matched = new Set<string>()
  for (const rawSel of selectors) {
    const sel = rawSel.trim()
    if (!sel || sel !== rawSel) continue
    if ('block'.startsWith(sel) || 'api'.startsWith(sel) || 'api-inbound'.startsWith(sel)) {
      continue
    }
    for (const c of candidates) {
      if (c && c.startsWith(sel)) matched.add(c)
    }
  }
  return [...matched]
}

type ParsedDnsEndpoint = {
  host: string
  port: number
  uri: boolean
  local: boolean
  system: boolean
}

function parseDnsEndpointForIPv6(address: string, explicitPort = 0): ParsedDnsEndpoint | null {
  if (!address || address.trim() !== address || /[\s]/.test(address)) return null
  if (address === 'localhost') {
    return explicitPort ? null : { host: '', port: 0, uri: false, local: false, system: true }
  }
  for (const prefix of ALLOWED_DNS_URI_PREFIXES) {
    if (address.startsWith(prefix)) {
      const local = prefix.includes('+local://')
      const normalized = address.replace('+local://', '://')
      let u: URL
      try {
        u = new URL(normalized)
      } catch {
        return null
      }
      if (!u.hostname || u.username || u.password || u.hash) return null
      let host = u.hostname.toLowerCase()
      if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1)
      if (host.endsWith('.')) host = host.slice(0, -1)
      const isHTTPS = prefix.startsWith('https')
      const isQUIC = prefix.startsWith('quic')
      let port = explicitPort
      if (u.port) {
        port = Number(u.port)
      }
      if (!port) {
        port = isHTTPS ? 443 : isQUIC ? 853 : 53
      }
      return { host, port, uri: true, local, system: false }
    }
  }
  if (address.includes('://')) return null
  if (classifyIPLiteral(address) === null) return null
  return {
    host: address.startsWith('[') && address.endsWith(']') ? address.slice(1, -1) : address,
    port: explicitPort || 53,
    uri: false,
    local: false,
    system: false,
  }
}

function ipv6MatchesRuleTarget(hostIPv6: bigint, target: string): boolean {
  const slash = target.indexOf('/')
  if (slash === -1) {
    const parsed = parseIPv6BigInt(target)
    return !!parsed && !parsed.ipv4Mapped && parsed.value === hostIPv6
  }
  const ipPart = target.slice(0, slash)
  const bits = Number(target.slice(slash + 1))
  if (!Number.isInteger(bits) || bits < 0 || bits > 128) return false
  const net = parseIPv6BigInt(ipPart)
  if (!net || net.ipv4Mapped) return false
  if (bits === 0) return true
  const shift = BigInt(128 - bits)
  return (hostIPv6 >> shift) === (net.value >> shift)
}

function dnsRuleMatchesEndpoint(rule: RoutingRuleItem, ep: ParsedDnsEndpoint, hostBig: bigint): boolean {
  if (
    (rule.domain?.length ?? 0) > 0 ||
    (rule.protocol?.length ?? 0) > 0 ||
    (rule.source?.length ?? 0) > 0 ||
    (rule.inboundTag?.length ?? 0) > 0
  ) {
    return false
  }
  for (const k of Object.keys(rule)) {
    if (!KNOWN_RULE_KEYS.has(k)) return false
  }
  const netProto = ep.uri ? 'tcp' : 'udp'
  if (rule.network && rule.network !== 'tcp,udp' && rule.network !== netProto) {
    return false
  }
  if (rule.port) {
    let matchedPort = false
    for (const part of rule.port.split(',')) {
      const bounds = part.trim().split('-')
      const low = Number(bounds[0])
      const high = bounds.length === 2 ? Number(bounds[1]) : low
      if (ep.port >= low && ep.port <= high) matchedPort = true
    }
    if (!matchedPort) return false
  }
  if (!rule.ip || rule.ip.length === 0) return true
  for (const target of rule.ip) {
    if (ipv6MatchesRuleTarget(hostBig, target)) return true
  }
  return false
}

export function requiresIPv6(policy: RoutingPolicy, defaultProbeURL = ''): boolean {
  const ver = policy.schema_version ?? 1
  if (ver < 2) return !!policy.enable_ipv6_outbound

  const outbounds = policy.custom_outbounds ?? []
  const byTag = new Map(outbounds.map((o) => [o.tag, o]))
  const selectableTags = [
    ...outbounds.map((o) => o.tag).filter(Boolean),
    'direct',
    'direct-v4',
    'direct-v6',
  ]
  const visiting = new Set<string>()

  const checkOutbound = (tag: string, target: string): boolean => {
    const key = `${tag}\0${target}`
    if (visiting.has(key)) return false
    visiting.add(key)
    try {
      switch (tag) {
        case 'block':
          return false
        case 'direct-v4':
          return localDialNeedsIPv6(target, '', 'ForceIPv4')
        case 'direct-v6':
          return localDialNeedsIPv6(target, '', 'ForceIPv6')
        case 'direct': {
          const strategy =
            policy.dns && policy.dns.enabled ? (policy.dns.queryStrategy ?? '') : 'AsIs'
          return localDialNeedsIPv6(target, '', strategy)
        }
      }
      const co = byTag.get(tag)
      if (!co || co.protocol === 'blackhole') return false

      const settings =
        co.settings && typeof co.settings === 'object' ? co.settings : ({} as Record<string, unknown>)
      const stream =
        co.streamSettings && typeof co.streamSettings === 'object'
          ? co.streamSettings
          : ({} as Record<string, unknown>)
      const sockopt =
        stream.sockopt && typeof stream.sockopt === 'object'
          ? (stream.sockopt as Record<string, unknown>)
          : ({} as Record<string, unknown>)

      let strategy = typeof sockopt.domainStrategy === 'string' ? sockopt.domainStrategy : ''
      const outerStrategy = typeof co.targetStrategy === 'string' ? co.targetStrategy : ''
      if (co.protocol === 'freedom') {
        const ts = typeof settings.targetStrategy === 'string' ? settings.targetStrategy : ''
        const ds = typeof settings.domainStrategy === 'string' ? settings.domainStrategy : ''
        const freedomStrategy = ts || ds
        if (freedomStrategy && freedomStrategy.toLowerCase() !== 'asis') {
          strategy = freedomStrategy
        }
      }
      if (outerStrategy && outerStrategy.toLowerCase() !== 'asis') {
        strategy = outerStrategy
      }

      const dp = extractDialerProxy(co)
      const dial = (address: string): boolean => {
        if (dp) {
          let nextAddr = address
          if (classifyIPLiteral(nextAddr) === null && strategyNeedsIPv6(strategy)) {
            nextAddr = '2001:db8::1'
          }
          return checkOutbound(dp, nextAddr)
        }
        return localDialNeedsIPv6(address, co.sendThrough ?? '', strategy)
      }

      switch (co.protocol) {
        case 'freedom':
          return dial(target)
        case 'socks':
        case 'http':
        case 'trojan':
        case 'shadowsocks': {
          if (typeof settings.address === 'string' && settings.address && dial(settings.address)) {
            return true
          }
          if (Array.isArray(settings.servers)) {
            for (const s of settings.servers) {
              const addr =
                s && typeof s === 'object' && typeof (s as { address?: unknown }).address === 'string'
                  ? (s as { address: string }).address
                  : ''
              if (dial(addr)) return true
            }
          }
          return false
        }
        case 'vless':
        case 'vmess': {
          if (typeof settings.address === 'string' && settings.address && dial(settings.address)) {
            return true
          }
          if (Array.isArray(settings.vnext)) {
            for (const v of settings.vnext) {
              const addr =
                v && typeof v === 'object' && typeof (v as { address?: unknown }).address === 'string'
                  ? (v as { address: string }).address
                  : ''
              if (dial(addr)) return true
            }
          }
          return false
        }
        case 'wireguard': {
          if (Array.isArray(settings.peers)) {
            for (const p of settings.peers) {
              const ep =
                p && typeof p === 'object' && typeof (p as { endpoint?: unknown }).endpoint === 'string'
                  ? (p as { endpoint: string }).endpoint
                  : ''
              const host = splitEndpointHost(ep)
              if (host !== null && dial(host)) return true
            }
          }
          return false
        }
      }
      return false
    } finally {
      visiting.delete(key)
    }
  }

  const checkRule = (r: RoutingRuleItem, target: string): boolean => {
    if (r.outboundTag) return checkOutbound(r.outboundTag, target)
    for (const b of policy.balancers ?? []) {
      if (b.tag !== r.balancerTag) continue
      const tags = matchOutboundSelectors(b.selector ?? [], selectableTags)
      for (const t of tags) {
        if (checkOutbound(t, target)) return true
      }
      return !!b.fallbackTag && checkOutbound(b.fallbackTag, target)
    }
    return false
  }

  if (checkOutbound('direct', '')) return true
  if ((policy.ipv6_force_domains ?? []).length > 0 && checkOutbound('direct-v6', '')) return true
  for (const r of policy.custom_rules ?? []) {
    if (!r.disabled && checkRule(r, '')) return true
  }

  // Observatory（显式与自动补齐）
  const activeBalancerTags = new Set(
    (policy.custom_rules ?? [])
      .filter((r) => !r.disabled && !!r.balancerTag)
      .map((r) => r.balancerTag!),
  )
  const autoObsSelectors: string[] = []
  let needsAutoObs = false
  for (const b of policy.balancers ?? []) {
    if (!activeBalancerTags.has(b.tag)) continue
    const st = b.strategy?.type || 'random'
    if (st === 'leastPing' || !!b.fallbackTag) {
      needsAutoObs = true
      for (const s of b.selector ?? []) {
        if (!autoObsSelectors.includes(s)) autoObsSelectors.push(s)
      }
    }
  }
  const trimmedDefaultProbe = defaultProbeURL.trim()
  const safeDefaultProbe = isValidObservatoryProbeURL(trimmedDefaultProbe)
    ? trimmedDefaultProbe
    : 'https://1.1.1.1/'

  let obsProbeURL = ''
  let obsSelectors: string[] | null = null
  if (policy.observatory) {
    obsProbeURL = policy.observatory.probeURL || safeDefaultProbe
    obsSelectors = policy.observatory.subjectSelector ?? []
  } else if (needsAutoObs) {
    obsProbeURL = safeDefaultProbe
    obsSelectors = autoObsSelectors
  }
  if (obsSelectors !== null) {
    let probeHost = ''
    try {
      const u = new URL(obsProbeURL)
      probeHost =
        u.hostname.startsWith('[') && u.hostname.endsWith(']')
          ? u.hostname.slice(1, -1)
          : u.hostname
    } catch {
      probeHost = ''
    }
    const tags = matchOutboundSelectors(obsSelectors, selectableTags)
    for (const t of tags) {
      if (checkOutbound(t, probeHost)) return true
    }
  }

  // DNS 上游物理拨号检查
  if (policy.dns && policy.dns.enabled) {
    for (const srv of policy.dns.servers ?? []) {
      const address = typeof srv === 'string' ? srv : (srv?.address ?? '')
      const port = typeof srv === 'object' && srv !== null ? (srv.port ?? 0) : 0
      const ep = parseDnsEndpointForIPv6(address, port)
      if (!ep || ep.system || classifyIPLiteral(ep.host) !== 'v6') continue
      const parsedV6 = parseIPv6BigInt(ep.host)
      if (!parsedV6 || parsedV6.ipv4Mapped) continue
      // Loopback (::1) 或 Link-local (fe80::/10) 不要求默认 IPv6 出口
      if (parsedV6.value === 1n || (parsedV6.value >> 118n) === 0x3fan) continue
      if (ep.local) return true

      let matched = false
      for (const rule of policy.custom_rules ?? []) {
        if (rule.disabled || !dnsRuleMatchesEndpoint(rule, ep, parsedV6.value)) continue
        matched = true
        if (checkRule(rule, ep.host)) return true
        break
      }
      if (!matched && checkOutbound('direct', ep.host)) return true
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
