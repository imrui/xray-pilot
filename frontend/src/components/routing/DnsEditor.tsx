import { useState } from 'react'
import { ArrowDown, ArrowUp, Globe, Info, Plus, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, FieldGroup, SelectField, Switch } from '@/components/ui/Form'
import { createDefaultDNSConfig, domainLines } from '@/lib/routing-model'
import { editorItemKey, retainEditorItemKey } from '@/lib/routing-editor'
import type {
  DNSConfigModel,
  DNSHostValue,
  DNSQueryStrategy,
  DNSServerEntry,
  DNSServerObjectItem,
} from '@/lib/routing-model'

const controlTextarea =
  'w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-2.5 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

const QUICK_DNS_DOMAINS = [
  'geosite:netflix',
  'geosite:disney',
  'geosite:openai',
  'geosite:cn',
]

function SingleDnsServerCard({
  entry,
  index,
  total,
  disabled,
  onMove,
  onDelete,
  onChange,
}: {
  entry: DNSServerEntry
  index: number
  total: number
  disabled?: boolean
  onMove: (delta: -1 | 1) => void
  onDelete: () => void
  onChange: (next: DNSServerEntry) => void
}) {
  const [domainsDraft, setDomainsDraft] = useState<string | null>(null)
  const [expectDraft, setExpectDraft] = useState<string | null>(null)
  const isObject = typeof entry === 'object' && entry !== null

  const switchEntryMode = (target: 'simple' | 'object') => {
    if (target === 'simple' && isObject) {
      onChange(entry.address || '1.1.1.1')
      setDomainsDraft(null)
      setExpectDraft(null)
    } else if (target === 'object' && !isObject) {
      onChange({
        address: entry || '',
        domains: [],
        skipFallback: true,
      })
    }
  }

  const patchObject = (patch: Partial<DNSServerObjectItem>) => {
    if (!isObject) return
    const next: DNSServerObjectItem = { ...entry, ...patch }
    if (!next.port) delete next.port
    if (!next.queryStrategy) delete next.queryStrategy
    if (!next.skipFallback) delete next.skipFallback
    if (!next.domains || next.domains.length === 0) delete next.domains
    if (!next.expectIPs || next.expectIPs.length === 0) delete next.expectIPs
    onChange(next)
  }

  return (
    <div className="space-y-3 rounded-lg border border-[var(--border)] bg-[var(--panel)] p-3.5">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] pb-2.5">
        <div className="flex items-center gap-2">
          <Badge label={`DNS #${index + 1}`} variant="blue" />
          <Badge
            label={isObject ? '分域名规则模式' : '全局上游简写'}
            variant={isObject ? 'yellow' : 'gray'}
          />
        </div>

        <div className="flex items-center gap-1.5">
          <div className="inline-flex rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-0.5 text-xs">
            <button
              type="button"
              disabled={disabled}
              onClick={() => switchEntryMode('simple')}
              className={`rounded px-2 py-1 font-medium transition ${
                !isObject
                  ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
                  : 'text-soft hover:text-[var(--text)]'
              }`}
            >
              简写
            </button>
            <button
              type="button"
              disabled={disabled}
              onClick={() => switchEntryMode('object')}
              className={`rounded px-2 py-1 font-medium transition ${
                isObject
                  ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
                  : 'text-soft hover:text-[var(--text)]'
              }`}
            >
              分域名规则
            </button>
          </div>
          <button
            type="button"
            aria-label={`上移 DNS #${index + 1}`}
            disabled={disabled || index === 0}
            onClick={() => onMove(-1)}
            className="rounded border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 text-soft hover:text-[var(--text)] disabled:opacity-40"
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            aria-label={`下移 DNS #${index + 1}`}
            disabled={disabled || index === total - 1}
            onClick={() => onMove(1)}
            className="rounded border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 text-soft hover:text-[var(--text)] disabled:opacity-40"
          >
            <ArrowDown className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            aria-label={`删除 DNS #${index + 1}`}
            disabled={disabled || total <= 1}
            title={total <= 1 ? '启用 DNS 时至少保留 1 个上游服务器' : '删除此上游 DNS'}
            onClick={onDelete}
            className="rounded border border-[var(--danger-border)] bg-[var(--danger-soft)] p-1.5 text-[var(--danger)] hover:bg-[var(--danger)] hover:text-[var(--danger-contrast)] disabled:opacity-40"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {!isObject ? (
        <Field
          label={`上游 DNS 地址 #${index + 1} *`}
          placeholder="如 1.1.1.1、localhost 或 https://1.1.1.1/dns-query"
          value={entry}
          onChange={(e) => onChange(e.target.value.trim())}
        />
      ) : (
        <div className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <Field
              label={`上游 DNS 地址 #${index + 1} *`}
              placeholder="如 解锁 DNS IP 或 https://..."
              value={entry.address ?? ''}
              onChange={(e) => patchObject({ address: e.target.value.trim() })}
            />
            <Field
              label="端口（可选，仅 IP/TCP 适用）"
              type="number"
              placeholder="53"
              value={entry.port ?? ''}
              onChange={(e) => {
                const n = Number(e.target.value)
                patchObject({ port: n > 0 ? n : undefined })
              }}
            />
            <SelectField
              label="单服务器解析策略"
              value={entry.queryStrategy ?? ''}
              onChange={(qs) =>
                patchObject({ queryStrategy: (qs || undefined) as DNSQueryStrategy | undefined })
              }
              options={[
                { value: '', label: '继承全局 queryStrategy' },
                { value: 'UseIP', label: 'UseIP（双栈）' },
                { value: 'UseIPv4', label: 'UseIPv4（仅 A 记录）' },
                { value: 'UseIPv6', label: 'UseIPv6（仅 AAAA 记录）' },
              ]}
            />
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <label className="block text-[12px] font-medium text-soft">
                指定匹配域名（domains，每行一项）
              </label>
              <textarea
                rows={3}
                aria-label={`DNS #${index + 1} 匹配域名`}
                placeholder={'geosite:netflix\ndomain:openai.com'}
                className={controlTextarea}
                value={domainsDraft ?? (entry.domains ?? []).join('\n')}
                onChange={(e) => {
                  setDomainsDraft(e.target.value)
                  patchObject({ domains: domainLines(e.target.value) })
                }}
                onBlur={() => setDomainsDraft(null)}
              />
              <div className="flex flex-wrap gap-1.5">
                {QUICK_DNS_DOMAINS.map((tok) => (
                  <button
                    key={tok}
                    type="button"
                    disabled={disabled}
                    onClick={() => {
                      const existing = entry.domains ?? []
                      if (existing.includes(tok)) return
                      setDomainsDraft(null)
                      patchObject({ domains: [...existing, tok] })
                    }}
                    className="rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2 py-0.5 font-mono text-[11px] text-soft hover:border-[var(--accent)] hover:text-[var(--accent)]"
                  >
                    + {tok}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="block text-[12px] font-medium text-soft">
                期望返回 IP 范围（expectIPs，可选，每行一项）
              </label>
              <textarea
                rows={3}
                aria-label={`DNS #${index + 1} 期望 IP`}
                placeholder={'geoip:cn\n203.0.113.0/24'}
                className={controlTextarea}
                value={expectDraft ?? (entry.expectIPs ?? []).join('\n')}
                onChange={(e) => {
                  setExpectDraft(e.target.value)
                  patchObject({ expectIPs: domainLines(e.target.value) })
                }}
                onBlur={() => setExpectDraft(null)}
              />
              <div className="flex items-center justify-between gap-2 rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2.5 py-1.5 text-xs">
                <span className="text-soft">回退轮询时跳过此服务器（skipFallback）</span>
                <Switch
                  checked={!!entry.skipFallback}
                  onChange={(checked) => patchObject({ skipFallback: checked })}
                />
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

export function DnsEditor({
  dns,
  disabled = false,
  hideEnableToggle = false,
  onChange,
}: {
  dns: DNSConfigModel | null
  disabled?: boolean
  hideEnableToggle?: boolean
  onChange: (next: DNSConfigModel | null) => void
}) {
  const [cachedDraft, setCachedDraft] = useState<DNSConfigModel>(() =>
    dns && dns.enabled ? dns : createDefaultDNSConfig(),
  )
  const [hostDraft, setHostDraft] = useState<{
    rows: { key: string; value: string }[]
    source: string
  } | null>(null)

  const enabled = !!dns?.enabled
  const activeDns: DNSConfigModel = dns && dns.enabled ? dns : cachedDraft
  const servers = activeDns.servers ?? []

  const syncedHostRows: { key: string; value: string }[] =
    hostDraft?.source === JSON.stringify(activeDns.hosts ?? {}) ? hostDraft.rows :
    Object.entries(activeDns.hosts ?? {}).map(([k, v]) => ({
      key: k,
      value: Array.isArray(v) ? v.join(', ') : String(v),
    }))

  const commitDns = (next: DNSConfigModel) => {
    setCachedDraft(next)
    onChange(next)
  }

  const toggleEnabled = (nextEnabled: boolean) => {
    if (nextEnabled) {
      const restored: DNSConfigModel = {
        ...cachedDraft,
        enabled: true,
        queryStrategy: cachedDraft.queryStrategy ?? 'UseIP',
        servers:
          cachedDraft.servers && cachedDraft.servers.length > 0
            ? cachedDraft.servers
            : ['https://1.1.1.1/dns-query'],
      }
      commitDns(restored)
    } else {
      if (dns) setCachedDraft(dns)
      setHostDraft(null)
      onChange(null)
    }
  }

  const appendServer = (entry: DNSServerEntry) => {
    if (servers.length >= 16) return
    commitDns({ ...activeDns, enabled: true, servers: [...servers, entry] })
  }

  const updateHostRows = (rows: { key: string; value: string }[]) => {
    const map: Record<string, DNSHostValue> = {}
    for (const r of rows) {
      const k = r.key.trim()
      if (!k) continue
      const ips = r.value
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      if (ips.length === 1) map[k] = ips[0]
      else if (ips.length > 1) map[k] = ips
    }
    const next = { ...activeDns, enabled: true }
    if (Object.keys(map).length > 0) next.hosts = map
    else delete next.hosts
    setHostDraft({ rows, source: JSON.stringify(next.hosts ?? {}) })
    commitDns(next)
  }

  return (
    <div className="space-y-4">
      {!hideEnableToggle && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel)] px-4 py-3">
          <div className="space-y-0.5">
            <div className="flex items-center gap-1.5 text-sm font-semibold">
              <Globe className="h-4 w-4 text-[var(--accent)]" />
              启用服务端自定义 DNS
            </div>
            <p className="text-xs text-soft">
              管控节点服务端 Xray 内部域名解析（防污染、分域名流媒体/AI 解锁 DNS、静态 Hosts）；关闭时完全走节点系统默认解析。
            </p>
          </div>
          <Switch checked={enabled} onChange={toggleEnabled} />
        </div>
      )}

      {enabled ? (
        <>
          <FieldGroup
            title="全局解析与缓存策略"
            description="约束 Xray 内置 DNS 解析器的查询地址族偏好与回退行为。"
          >
            <SelectField
              label="全局查询策略（queryStrategy）"
              value={activeDns.queryStrategy ?? 'UseIP'}
              onChange={(qs) =>
                commitDns({
                  ...activeDns,
                  enabled: true,
                  queryStrategy: qs as DNSQueryStrategy,
                })
              }
              options={[
                { value: 'UseIP', label: 'UseIP（默认 · 双栈查询 A + AAAA）' },
                { value: 'UseIPv4', label: 'UseIPv4（仅查询 IPv4 A 记录）' },
                { value: 'UseIPv6', label: 'UseIPv6（仅查询 IPv6 AAAA 记录）' },
              ]}
            />

            <div className="grid gap-2.5 sm:grid-cols-3">
              {[
                {
                  key: 'disableCache' as const,
                  title: '禁用缓存（disableCache）',
                  desc: '每次连接均重新向上游发起解析',
                },
                {
                  key: 'disableFallback' as const,
                  title: '禁用默认回退（disableFallback）',
                  desc: '未命中域名规则时不依次轮询兜底服务器',
                },
                {
                  key: 'disableFallbackIfMatch' as const,
                  title: '命中即停（disableFallbackIfMatch）',
                  desc: '一旦命中某条 domains 服务器即不再回退',
                },
              ].map((opt) => (
                <div
                  key={opt.key}
                  className="flex items-center justify-between gap-2 rounded-md border border-[var(--border)] bg-[var(--panel)] p-3"
                >
                  <div className="space-y-0.5">
                    <p className="text-xs font-medium text-[var(--text)]">{opt.title}</p>
                    <p className="text-[11px] text-soft">{opt.desc}</p>
                  </div>
                  <Switch
                    checked={!!activeDns[opt.key]}
                    onChange={(checked) => {
                      const next = { ...activeDns, enabled: true }
                      if (checked) next[opt.key] = true
                      else delete next[opt.key]
                      commitDns(next)
                    }}
                  />
                </div>
              ))}
            </div>
          </FieldGroup>

          <FieldGroup
            title={`上游 DNS 服务器列表（${servers.length}/16）`}
            description="按从上到下顺序匹配与回退。支持快速插入加密 DoH / 公共 DNS，或针对特定流媒体/AI 域名单独指定解锁 DNS 服务器。"
          >
            <div className="flex flex-wrap gap-2">
              <Btn
                variant="secondary"
                disabled={disabled || servers.length >= 16}
                onClick={() => appendServer('https://1.1.1.1/dns-query')}
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                Cloudflare DoH
              </Btn>
              <Btn
                variant="secondary"
                disabled={disabled || servers.length >= 16}
                onClick={() => appendServer('https://dns.google/dns-query')}
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                Google DoH
              </Btn>
              <Btn
                variant="secondary"
                disabled={disabled || servers.length >= 16}
                onClick={() => appendServer('1.1.1.1')}
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                1.1.1.1 UDP
              </Btn>
              <Btn
                variant="secondary"
                disabled={disabled || servers.length >= 16}
                onClick={() => appendServer('localhost')}
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                localhost
              </Btn>
              <Btn
                variant="accent"
                disabled={disabled || servers.length >= 16}
                onClick={() =>
                  appendServer({
                    address: '',
                    domains: ['geosite:netflix'],
                    skipFallback: true,
                  })
                }
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                添加分域名解锁 DNS 规则
              </Btn>
            </div>

            <div className="space-y-3">
              {servers.map((srv, idx) => (
                <SingleDnsServerCard
                  key={typeof srv === 'object' && srv !== null ? editorItemKey(srv) : `simple-${idx}`}
                  entry={srv}
                  index={idx}
                  total={servers.length}
                  disabled={disabled}
                  onMove={(delta) => {
                    const target = idx + delta
                    if (target < 0 || target >= servers.length) return
                    const nextServers = [...servers]
                    ;[nextServers[idx], nextServers[target]] = [
                      nextServers[target],
                      nextServers[idx],
                    ]
                    commitDns({ ...activeDns, enabled: true, servers: nextServers })
                  }}
                  onDelete={() => {
                    if (servers.length <= 1) return
                    commitDns({
                      ...activeDns,
                      enabled: true,
                      servers: servers.filter((_, i) => i !== idx),
                    })
                  }}
                  onChange={(updated) => {
                    const nextServers = [...servers]
                    nextServers[idx] = typeof srv === 'object' && srv !== null && typeof updated === 'object' && updated !== null
                      ? retainEditorItemKey(srv, updated)
                      : updated
                    commitDns({ ...activeDns, enabled: true, servers: nextServers })
                  }}
                />
              ))}
            </div>
          </FieldGroup>

          <FieldGroup
            title={`静态 Hosts 映射（${syncedHostRows.length}/200）`}
            description="键支持完整域名、domain:example.com、full:example.com 或 geosite: 标签；值支持单个 IP 或用英文逗号分隔的多个 IP。"
          >
            {syncedHostRows.length > 0 && (
              <div className="space-y-2.5">
                {syncedHostRows.map((row, idx) => (
                  <div
                    key={idx}
                    className="grid gap-2.5 rounded-md border border-[var(--border)] bg-[var(--panel)] p-3 sm:grid-cols-[1fr_1fr_auto]"
                  >
                    <Field
                      label={`Hosts 域名键 #${idx + 1}`}
                      placeholder="如 domain:openai.com 或 api.example.com"
                      value={row.key}
                      onChange={(e) => {
                        const nextRows = [...syncedHostRows]
                        nextRows[idx] = { ...row, key: e.target.value }
                        updateHostRows(nextRows)
                      }}
                    />
                    <Field
                      label={`映射目标 IP #${idx + 1}`}
                      placeholder="如 203.0.113.10 或 1.1.1.1, 1.0.0.1"
                      value={row.value}
                      onChange={(e) => {
                        const nextRows = [...syncedHostRows]
                        nextRows[idx] = { ...row, value: e.target.value }
                        updateHostRows(nextRows)
                      }}
                    />
                    <div className="flex items-end">
                      <Btn
                        variant="danger"
                        disabled={disabled}
                        aria-label={`删除 Hosts #${idx + 1}`}
                        onClick={() =>
                          updateHostRows(syncedHostRows.filter((_, i) => i !== idx))
                        }
                        className="h-10 px-3"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Btn>
                    </div>
                  </div>
                ))}
              </div>
            )}

            <div>
              <Btn
                variant="secondary"
                disabled={disabled || syncedHostRows.length >= 200}
                onClick={() => updateHostRows([...syncedHostRows, { key: '', value: '' }])}
                className="h-8 px-3 text-xs"
              >
                <Plus className="h-3.5 w-3.5" />
                添加静态 Hosts 映射
              </Btn>
            </div>
          </FieldGroup>
        </>
      ) : null}

      <div className="flex items-start gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3.5 py-2.5 text-xs text-soft">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-[var(--accent)]" />
        <span>
          启用自定义 DNS 后，默认直连出站将自动使用 Xray 内置解析结果以使分流 DNS 与 Hosts 生效。
        </span>
      </div>
    </div>
  )
}
