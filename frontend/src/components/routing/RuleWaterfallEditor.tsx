import { useId, useState } from 'react'
import {
  ArrowDown,
  ArrowUp,
  Code2,
  Lock,
  Plus,
  SlidersHorizontal,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, FieldGroup, SelectField, Switch } from '@/components/ui/Form'
import { Modal } from '@/components/ui/Modal'
import { JsonEditor } from '@/components/ui/JsonEditor'
import { RULE_SNIPPETS } from '@/lib/routing-snippets'
import type { RuleSnippet } from '@/lib/routing-snippets'
import {
  BUILTIN_OUTBOUNDS,
  domainLines,
  parseSingleObject,
} from '@/lib/routing-model'
import type {
  BalancerItem,
  CustomOutbound,
  DomainStrategy,
  RoutingRuleItem,
} from '@/lib/routing-model'

const BALANCER_VALUE_PREFIX = 'balancer:'

function RuleTargetSelect({
  label,
  value,
  outbounds,
  balancers,
  disabled,
  onChange,
}: {
  label: string
  value: string
  outbounds: CustomOutbound[]
  balancers: BalancerItem[]
  disabled?: boolean
  onChange: (nextVal: string) => void
}) {
  const id = useId()
  const outboundOptions = [
    ...BUILTIN_OUTBOUNDS.map((b) => ({ value: b.tag, label: b.label })),
    ...outbounds
      .filter((o) => o.tag)
      .map((o) => ({
        value: o.tag,
        label: `${o.tag}（${o.protocol}${o.remark ? ` · ${o.remark}` : ''}）`,
      })),
  ]
  const balancerOptions = balancers
    .filter((b) => b.tag)
    .map((b) => ({
      value: `${BALANCER_VALUE_PREFIX}${b.tag}`,
      label: `${b.tag}（${b.strategy?.type ?? 'random'}${b.remark ? ` · ${b.remark}` : ''}）`,
    }))

  const isBalancerVal = value.startsWith(BALANCER_VALUE_PREFIX)
  const rawTag = isBalancerVal ? value.slice(BALANCER_VALUE_PREFIX.length) : value
  const isKnown = isBalancerVal
    ? balancerOptions.some((b) => b.value === value)
    : outboundOptions.some((o) => o.value === value)

  return (
    <div className="flex flex-col space-y-1.5">
      <label htmlFor={id} className="text-[12px] font-medium text-soft">
        {label}
      </label>
      <select
        id={id}
        value={value}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
      >
        {!isKnown && (
          <option value={value}>
            {rawTag || '未选择'}（{isBalancerVal ? '未定义均衡组' : '未定义出站'}）
          </option>
        )}
        <optgroup label="出站（Outbounds）">
          {outboundOptions.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </optgroup>
        {balancerOptions.length > 0 && (
          <optgroup label="负载均衡组（Balancers）">
            {balancerOptions.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </optgroup>
        )}
      </select>
    </div>
  )
}

const controlTextarea =
  'w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-2.5 text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

const QUICK_DOMAINS = [
  'geosite:openai',
  'geosite:anthropic',
  'geosite:netflix',
  'geosite:category-ads-all',
  'geosite:cn',
]

const QUICK_IPS = ['geoip:telegram', '!geoip:cn', 'geoip:cloudflare']

const PROTOCOLS = ['http', 'tls', 'quic', 'bittorrent'] as const

function SingleRuleCard({
  rule,
  index,
  total,
  outbounds,
  balancers,
  disabled,
  onMove,
  onDelete,
  onChange,
  onRawErrorChange,
}: {
  rule: RoutingRuleItem
  index: number
  total: number
  outbounds: CustomOutbound[]
  balancers: BalancerItem[]
  disabled?: boolean
  onMove: (delta: -1 | 1) => void
  onDelete: () => void
  onChange: (next: RoutingRuleItem) => void
  onRawErrorChange: (err: string) => void
}) {
  const [mode, setMode] = useState<'form' | 'json'>('form')
  const [rawText, setRawText] = useState(() => JSON.stringify(rule, null, 2))
  const [rawError, setRawError] = useState('')
  const [domainDraft, setDomainDraft] = useState<string | null>(null)
  const [ipDraft, setIpDraft] = useState<string | null>(null)

  const enabled = !rule.disabled
  const hasTargetFilter =
    (rule.domain?.length ?? 0) > 0 ||
    (rule.ip?.length ?? 0) > 0 ||
    !!rule.port?.trim() ||
    (rule.protocol?.length ?? 0) > 0

  const selectedTargetValue = rule.balancerTag
    ? `${BALANCER_VALUE_PREFIX}${rule.balancerTag}`
    : (rule.outboundTag ?? 'direct')

  const switchMode = (target: 'form' | 'json') => {
    if (target === 'json') {
      setRawText(JSON.stringify(rule, null, 2))
      setRawError('')
      onRawErrorChange('')
      setMode('json')
      return
    }
    try {
      const parsed = parseSingleObject<RoutingRuleItem>(rawText, '路由规则')
      onChange({ type: 'field', ...parsed })
      setDomainDraft(null)
      setIpDraft(null)
      setRawError('')
      onRawErrorChange('')
      setMode('form')
    } catch (e) {
      const msg = (e as Error).message
      setRawError(msg)
      onRawErrorChange(msg)
    }
  }

  const patchListField = (key: 'domain' | 'ip', items: string[]) => {
    const next = { ...rule, type: 'field' }
    if (items.length > 0) next[key] = items
    else delete next[key]
    onChange(next)
  }

  const toggleProtocol = (proto: string) => {
    const current = rule.protocol ?? []
    const nextList = current.includes(proto)
      ? current.filter((p) => p !== proto)
      : [...current, proto]
    const next = { ...rule, type: 'field' }
    if (nextList.length > 0) next.protocol = nextList
    else delete next.protocol
    onChange(next)
  }

  const appendQuickToken = (key: 'domain' | 'ip', token: string) => {
    const existing = rule[key] ?? []
    if (existing.includes(token)) return
    const updated = [...existing, token]
    if (key === 'domain') setDomainDraft(null)
    if (key === 'ip') setIpDraft(null)
    patchListField(key, updated)
  }

  return (
    <div
      className={`space-y-3 rounded-lg border p-4 transition ${
        enabled
          ? 'border-[var(--border)] bg-[var(--panel)]'
          : 'border-dashed border-[var(--border-strong)] bg-[var(--panel-muted)] opacity-80'
      }`}
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] pb-3">
        <div className="flex flex-wrap items-center gap-2">
          <Badge label={`#${index + 1}`} variant={enabled ? 'blue' : 'gray'} />
          <div className="flex items-center gap-1.5 text-xs">
            <Switch
              checked={enabled}
              onChange={(nextEnabled) => {
                const next = { ...rule }
                if (nextEnabled) delete next.disabled
                else next.disabled = true
                onChange(next)
              }}
            />
            <span className="text-soft">{enabled ? '启用' : '已暂停'}</span>
          </div>
          <input
            type="text"
            aria-label={`规则 #${index + 1} 备注`}
            placeholder="填写规则备注（如 AI 分流 / 屏蔽 BT）"
            value={rule.remark ?? ''}
            onChange={(e) => {
              const next = { ...rule }
              if (e.target.value) next.remark = e.target.value
              else delete next.remark
              onChange(next)
            }}
            className="h-8 w-52 rounded border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 text-xs text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none"
          />
        </div>

        <div className="flex items-center gap-1.5">
          <div className="inline-flex rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-0.5 text-xs">
            <button
              type="button"
              onClick={() => switchMode('form')}
              className={`inline-flex items-center gap-1 rounded px-2 py-1 font-medium transition ${
                mode === 'form'
                  ? 'bg-[var(--accent)] text-white'
                  : 'text-soft hover:text-[var(--text)]'
              }`}
            >
              <SlidersHorizontal className="h-3 w-3" />
              表单
            </button>
            <button
              type="button"
              onClick={() => switchMode('json')}
              className={`inline-flex items-center gap-1 rounded px-2 py-1 font-medium transition ${
                mode === 'json'
                  ? 'bg-[var(--accent)] text-white'
                  : 'text-soft hover:text-[var(--text)]'
              }`}
            >
              <Code2 className="h-3 w-3" />
              JSON
            </button>
          </div>
          <button
            type="button"
            aria-label={`上移规则 #${index + 1}`}
            disabled={disabled || index === 0}
            onClick={() => onMove(-1)}
            className="rounded border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 text-soft hover:text-[var(--text)] disabled:opacity-40"
          >
            <ArrowUp className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            aria-label={`下移规则 #${index + 1}`}
            disabled={disabled || index === total - 1}
            onClick={() => onMove(1)}
            className="rounded border border-[var(--border)] bg-[var(--panel-strong)] p-1.5 text-soft hover:text-[var(--text)] disabled:opacity-40"
          >
            <ArrowDown className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            aria-label={`删除规则 #${index + 1}`}
            disabled={disabled}
            onClick={onDelete}
            className="rounded border border-[var(--danger-border)] bg-[var(--danger-soft)] p-1.5 text-[var(--danger)] hover:bg-[var(--danger)] hover:text-white disabled:opacity-40"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {mode === 'form' ? (
        <div className="space-y-3">
          <div className="grid gap-3 sm:grid-cols-3">
            <RuleTargetSelect
              label="目标出站 / 均衡组 *"
              value={selectedTargetValue}
              outbounds={outbounds}
              balancers={balancers}
              disabled={disabled}
              onChange={(nextVal) => {
                const next = { ...rule, type: 'field' }
                if (nextVal.startsWith(BALANCER_VALUE_PREFIX)) {
                  next.balancerTag = nextVal.slice(BALANCER_VALUE_PREFIX.length)
                  delete next.outboundTag
                } else {
                  next.outboundTag = nextVal
                  delete next.balancerTag
                }
                onChange(next)
              }}
            />
            <SelectField
              label="传输协议（network）"
              value={rule.network ?? ''}
              onChange={(net) => {
                const next = { ...rule, type: 'field' }
                if (net) next.network = net
                else delete next.network
                onChange(next)
              }}
              options={[
                { value: '', label: '不限（TCP + UDP）' },
                { value: 'tcp', label: '仅 TCP' },
                { value: 'udp', label: '仅 UDP' },
                { value: 'tcp,udp', label: '显式 tcp,udp' },
              ]}
            />
            <Field
              label="目标端口（port，可选）"
              placeholder="如 80,443 或 1000-2000"
              value={rule.port ?? ''}
              onChange={(e) => {
                const next = { ...rule, type: 'field' }
                const v = e.target.value.trim()
                if (v) next.port = v
                else delete next.port
                onChange(next)
              }}
            />
          </div>

          <div className="grid gap-3 md:grid-cols-2">
            <div className="space-y-1.5">
              <label className="block text-[12px] font-medium text-soft">
                域名匹配（每行一项，支持 geosite: / domain: / full: / keyword: / regexp:）
              </label>
              <textarea
                rows={3}
                aria-label={`规则 #${index + 1} 域名匹配`}
                placeholder={'geosite:openai\ndomain:example.com'}
                className={controlTextarea + ' font-mono'}
                value={domainDraft ?? (rule.domain ?? []).join('\n')}
                onChange={(e) => {
                  setDomainDraft(e.target.value)
                  patchListField('domain', domainLines(e.target.value))
                }}
                onBlur={() => setDomainDraft(null)}
              />
              <div className="flex flex-wrap gap-1.5">
                {QUICK_DOMAINS.map((tok) => (
                  <button
                    key={tok}
                    type="button"
                    onClick={() => appendQuickToken('domain', tok)}
                    className="rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2 py-0.5 font-mono text-[11px] text-soft hover:border-[var(--accent)] hover:text-[var(--accent)]"
                  >
                    + {tok}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <label className="block text-[12px] font-medium text-soft">
                目标 IP / CIDR / GeoIP（每行一项，支持 geoip: 与 !geoip:）
              </label>
              <textarea
                rows={3}
                aria-label={`规则 #${index + 1} IP 匹配`}
                placeholder={'geoip:telegram\n!geoip:cn\n203.0.113.0/24'}
                className={controlTextarea + ' font-mono'}
                value={ipDraft ?? (rule.ip ?? []).join('\n')}
                onChange={(e) => {
                  setIpDraft(e.target.value)
                  patchListField('ip', domainLines(e.target.value))
                }}
                onBlur={() => setIpDraft(null)}
              />
              <div className="flex flex-wrap gap-1.5">
                {QUICK_IPS.map((tok) => (
                  <button
                    key={tok}
                    type="button"
                    onClick={() => appendQuickToken('ip', tok)}
                    className="rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2 py-0.5 font-mono text-[11px] text-soft hover:border-[var(--accent)] hover:text-[var(--accent)]"
                  >
                    + {tok}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="space-y-1.5">
            <span className="block text-[12px] font-medium text-soft">
              嗅探应用层协议匹配（protocol，多选）
            </span>
            <div className="flex flex-wrap gap-2">
              {PROTOCOLS.map((proto) => {
                const active = (rule.protocol ?? []).includes(proto)
                return (
                  <button
                    key={proto}
                    type="button"
                    aria-pressed={active}
                    onClick={() => toggleProtocol(proto)}
                    className={`rounded-md border px-2.5 py-1 font-mono text-xs transition ${
                      active
                        ? 'border-[var(--accent)] bg-[var(--accent-soft)] font-medium text-[var(--accent)]'
                        : 'border-[var(--border)] bg-[var(--panel-strong)] text-soft hover:text-[var(--text)]'
                    }`}
                  >
                    {proto}
                  </button>
                )
              })}
            </div>
          </div>

          {!hasTargetFilter && (
            <p className="text-xs text-[var(--warning)]">
              提示：请至少限定域名、目标 IP、端口或应用层协议中的一项（面板禁止保存无过滤条件的全量兜底规则）。
            </p>
          )}
        </div>
      ) : (
        <div className="space-y-2">
          <JsonEditor
            ariaLabel={`规则 #${index + 1} JSON`}
            value={rawText}
            readOnly={disabled}
            minHeight="160px"
            onChange={(text) => {
              setRawText(text)
              try {
                const parsed = parseSingleObject<RoutingRuleItem>(text, '路由规则')
                setRawError('')
                onRawErrorChange('')
                onChange({ type: 'field', ...parsed })
              } catch (e) {
                const msg = (e as Error).message
                setRawError(msg)
                onRawErrorChange(msg)
              }
            }}
          />
          {rawError && (
            <p role="alert" className="text-xs text-[var(--danger)]">
              {rawError}
            </p>
          )}
        </div>
      )}
    </div>
  )
}

export function RuleWaterfallEditor({
  domainStrategy,
  onDomainStrategyChange,
  rules,
  onRulesChange,
  outbounds,
  balancers = [],
  v4Text,
  onV4Change,
  v6Text,
  onV6Change,
  legacyV1,
  preset4,
  preset6,
  hasReferencePreset,
  disabled = false,
  onPendingErrorChange,
}: {
  domainStrategy: DomainStrategy
  onDomainStrategyChange: (next: DomainStrategy) => void
  rules: RoutingRuleItem[]
  onRulesChange: (next: RoutingRuleItem[]) => void
  outbounds: CustomOutbound[]
  balancers?: BalancerItem[]
  v4Text: string
  onV4Change: (next: string) => void
  v6Text: string
  onV6Change: (next: string) => void
  legacyV1: boolean
  preset4: string[]
  preset6: string[]
  hasReferencePreset: boolean
  disabled?: boolean
  onPendingErrorChange?: (hasError: boolean) => void
}) {
  const [importOpen, setImportOpen] = useState(false)
  const [snippetsOpen, setSnippetsOpen] = useState(false)
  const [pendingSnippet, setPendingSnippet] = useState<RuleSnippet | null>(null)
  const [snippetTargetTag, setSnippetTargetTag] = useState('block')
  const [ruleErrors, setRuleErrors] = useState<Record<number, string>>({})

  const updateRuleError = (idx: number, err: string) => {
    const next = { ...ruleErrors }
    if (err) next[idx] = err
    else delete next[idx]
    setRuleErrors(next)
    onPendingErrorChange?.(Object.keys(next).length > 0)
  }

  const addRule = () => {
    const defaultTarget = outbounds[0]?.tag || 'block'
    onRulesChange([
      ...rules,
      {
        type: 'field',
        remark: '',
        domain: ['geosite:category-ads-all'],
        outboundTag: defaultTarget,
      },
    ])
  }

  const moveRule = (index: number, delta: -1 | 1) => {
    const target = index + delta
    if (target < 0 || target >= rules.length) return
    const next = [...rules]
    const [moved] = next.splice(index, 1)
    next.splice(target, 0, moved)
    setRuleErrors({})
    onPendingErrorChange?.(false)
    onRulesChange(next)
  }

  return (
    <div className="space-y-4">
      <FieldGroup
        title="全局路由解析策略与系统保护规则"
        description="节点最终路由按「系统置顶保护规则 → 自定义分流规则 → 强制 IPv4 域名 → 强制 IPv6 域名 → 默认首项 direct 兜底」顺序执行。"
      >
        <SelectField
          label="路由域名解析策略（routing.domainStrategy）"
          value={domainStrategy}
          onChange={(v) => onDomainStrategyChange(v as DomainStrategy)}
          options={[
            { value: 'IPIfNonMatch', label: 'IPIfNonMatch（默认：域名未命中时解析 IP 再匹配规则）' },
            { value: 'AsIs', label: 'AsIs（仅按域名或已知目标 IP 匹配，不在路由阶段主动解析）' },
            { value: 'IPOnDemand', label: 'IPOnDemand（存在 IP 规则时按需触发域名解析）' },
          ]}
        />

        <details className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-xs">
          <summary className="flex cursor-pointer items-center gap-2 font-medium text-[var(--text)]">
            <Lock className="h-3.5 w-3.5 text-[var(--accent)]" />
            查看面板强制置顶的系统保护规则（只读，不可覆盖）
          </summary>
          <ol className="mt-2.5 list-decimal space-y-1 pl-5 text-soft">
            <li>
              <code className="font-mono">inboundTag: [&quot;api-inbound&quot;] → api</code>（保障本地 127.0.0.1:10085 gRPC 热更新与流量统计）
            </li>
            <li>
              显式开启 <code className="font-mono">allow_private</code> 的受控运维 HTTP/SOCKS 入站直连规则
            </li>
            <li>
              <code className="font-mono">ip: [&quot;geoip:private&quot;] → block</code>（阻断代理访问节点私网/内网服务）
            </li>
            <li>普通 HTTP/SOCKS 运维代理源 IP 白名单与兜底拦截规则</li>
          </ol>
        </details>
      </FieldGroup>

      <FieldGroup
        title={`自定义分流规则 · ${rules.length} 条`}
        description="按从上到下顺序优先匹配，优先级高于下方强制 IPv4 / IPv6 快捷域名列表。"
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-xs text-soft">
            支持 geosite / geoip / 协议嗅探与自定义出站联动；暂停的规则在节点下发时自动跳过。
          </span>
          <div className="flex flex-wrap gap-2">
            <Btn
              variant="secondary"
              disabled={disabled || rules.length >= 200}
              onClick={() => setSnippetsOpen(!snippetsOpen)}
            >
              <Sparkles className="h-4 w-4" />
              插入规则片段
            </Btn>
            <Btn variant="secondary" disabled={disabled || rules.length >= 200} onClick={addRule}>
              <Plus className="h-4 w-4" />
              添加分流规则
            </Btn>
          </div>
        </div>

        {snippetsOpen && (
          <div className="grid gap-2.5 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 sm:grid-cols-2">
            {RULE_SNIPPETS.map((snippet) => (
              <button
                key={snippet.key}
                type="button"
                disabled={disabled || rules.length >= 200}
                onClick={() => {
                  setPendingSnippet(snippet)
                  setSnippetTargetTag(snippet.defaultTarget(outbounds))
                }}
                className="flex flex-col items-start gap-1 rounded border border-[var(--border)] bg-[var(--panel-muted)] p-3 text-left transition hover:border-[var(--accent)] hover:bg-[var(--panel)]"
              >
                <span className="text-xs font-semibold text-[var(--accent)]">{snippet.name}</span>
                <span className="text-[11px] leading-4 text-soft">{snippet.description}</span>
              </button>
            ))}
          </div>
        )}

        {rules.length === 0 ? (
          <p className="rounded-md border border-dashed border-[var(--border-strong)] bg-[var(--panel-strong)] p-4 text-center text-xs text-soft">
            暂无自定义分流规则。可直接添加规则卡片，或在下方快速填写强制 IPv4 / IPv6 域名列表。
          </p>
        ) : (
          <div className="space-y-3">
            {rules.map((rule, idx) => (
              <SingleRuleCard
                key={idx}
                rule={rule}
                index={idx}
                total={rules.length}
                outbounds={outbounds}
                balancers={balancers}
                disabled={disabled}
                onMove={(delta) => moveRule(idx, delta)}
                onDelete={() => {
                  updateRuleError(idx, '')
                  onRulesChange(rules.filter((_, i) => i !== idx))
                }}
                onRawErrorChange={(err) => updateRuleError(idx, err)}
                onChange={(updated) => {
                  const next = [...rules]
                  next[idx] = updated
                  onRulesChange(next)
                }}
              />
            ))}
          </div>
        )}
      </FieldGroup>

      <FieldGroup
        title="快捷域名地址族分流（强制 IPv4 / IPv6）"
        description="未命中规则保持默认直连。每行一条，例如 domain:example.com 或 geosite:openai；自定义规则优先于域名列表，两组域名重叠时 IPv4 优先。"
      >
        <p className="text-xs text-soft">
          默认出站：正常直连，不强制 IPv4 或 IPv6。两组列表留空时保持默认行为。
        </p>
        {legacyV1 && (
          <p className="text-xs text-[var(--warning)]">
            正在转换旧版草稿：原先实际走 IPv4 的域名已归入 IPv4
            列表。保存并发布新版本、重新绑定后生效，旧版本不变。
          </p>
        )}
        <label className="block space-y-2 text-sm">
          <span>强制 IPv4 域名 · {domainLines(v4Text).length} 条</span>
          <textarea
            rows={4}
            className="w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 font-mono text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
            value={v4Text}
            onChange={(e) => onV4Change(e.target.value)}
          />
        </label>
        <label className="block space-y-2 text-sm">
          <span>强制 IPv6 域名 · {domainLines(v6Text).length} 条</span>
          <textarea
            rows={4}
            className="w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 font-mono text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
            value={v6Text}
            onChange={(e) => onV6Change(e.target.value)}
          />
        </label>
        <p className="text-xs text-soft">
          强制 IPv6 需要节点有 IPv6 出口，失败不回退
          IPv4；无出口的节点会在同步前被拦截。策略与客户端使用 IPv4 或 IPv6 连接节点无关。
        </p>
        <div>
          <Btn
            variant="secondary"
            disabled={!hasReferencePreset}
            onClick={() => setImportOpen(!importOpen)}
          >
            导入参考域名
          </Btn>
        </div>
        {importOpen && (
          <div className="space-y-3 rounded border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-xs">
            <p>起始列表不代表完整服务域名库。google.com 覆盖范围较广，默认不包含，可按需添加。</p>
            <p>IPv4：{preset4.join('、')}</p>
            <p>IPv6：{preset6.join('、')}</p>
            <div className="flex gap-2">
              {[false, true].map((replace) => (
                <Btn
                  key={String(replace)}
                  variant="secondary"
                  onClick={() => {
                    onV4Change(
                      domainLines(
                        [...(replace ? [] : domainLines(v4Text)), ...preset4].join('\n'),
                      ).join('\n'),
                    )
                    onV6Change(
                      domainLines(
                        [...(replace ? [] : domainLines(v6Text)), ...preset6].join('\n'),
                      ).join('\n'),
                    )
                    setImportOpen(false)
                  }}
                >
                  {replace ? '替换现有列表' : '追加到现有列表'}
                </Btn>
              ))}
            </div>
          </div>
        )}
      </FieldGroup>

      <Modal
        open={!!pendingSnippet}
        onClose={() => setPendingSnippet(null)}
        size="sm"
        title={`插入规则片段 · ${pendingSnippet?.name ?? ''}`}
        footer={
          <>
            <Btn variant="secondary" onClick={() => setPendingSnippet(null)}>
              取消
            </Btn>
            <Btn
              onClick={() => {
                if (!pendingSnippet) return
                const isBalancerTarget = snippetTargetTag.startsWith(BALANCER_VALUE_PREFIX)
                const rawTarget = isBalancerTarget
                  ? snippetTargetTag.slice(BALANCER_VALUE_PREFIX.length)
                  : snippetTargetTag
                const builtRules = pendingSnippet.build(rawTarget).map((r) => {
                  if (!isBalancerTarget) return r
                  const copy = { ...r, balancerTag: rawTarget }
                  delete copy.outboundTag
                  return copy
                })
                onRulesChange([...rules, ...builtRules])
                setPendingSnippet(null)
                setSnippetsOpen(false)
              }}
            >
              确认插入规则
            </Btn>
          </>
        }
      >
        <div className="space-y-3">
          <p className="text-xs leading-5 text-soft">{pendingSnippet?.description}</p>
          <RuleTargetSelect
            label="选择片段命中的目标出站 / 均衡组"
            value={snippetTargetTag}
            outbounds={outbounds}
            balancers={balancers}
            onChange={setSnippetTargetTag}
          />
        </div>
      </Modal>
    </div>
  )
}
