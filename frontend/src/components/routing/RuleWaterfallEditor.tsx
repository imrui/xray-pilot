import { useId, useState } from 'react'
import {
  ArrowDown,
  ArrowUp,
  Code2,
  Plus,
  SlidersHorizontal,
  Sparkles,
  Trash2,
} from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, SelectField, Switch } from '@/components/ui/Form'
import { Modal } from '@/components/ui/Modal'
import { JsonEditor } from '@/components/ui/JsonEditor'
import { RULE_SNIPPETS } from '@/lib/routing-snippets'
import { editorItemKey, retainEditorItemKey } from '@/lib/routing-editor'
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
      <label htmlFor={id} className="text-sm font-medium text-soft">
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
  'w-full min-w-0 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-sm leading-6 text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

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
  const [rawDraft, setRawDraft] = useState<{ text: string; source: string } | null>(null)
  const serialized = JSON.stringify(rule)
  const rawText = rawDraft?.source === serialized ? rawDraft.text : JSON.stringify(rule, null, 2)
  const [rawError, setRawError] = useState('')
  const [domainDraft, setDomainDraft] = useState<string | null>(null)
  const [ipDraft, setIpDraft] = useState<string | null>(null)
  const fieldId = useId()

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
    if (target === mode) return
    if (target === 'json') {
      setRawDraft(null)
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
        <fieldset disabled={disabled || mode === 'json'} className="flex flex-wrap items-center gap-2">
          <Badge label={`#${index + 1}`} variant={enabled ? 'blue' : 'gray'} />
          <div className="flex items-center gap-1.5 text-sm">
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
            placeholder="规则备注"
            value={rule.remark ?? ''}
            onChange={(e) => {
              const next = { ...rule }
              if (e.target.value) next.remark = e.target.value
              else delete next.remark
              onChange(next)
            }}
            className="h-9 w-48 max-w-full rounded border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none"
          />
        </fieldset>

        <div className="flex items-center gap-1.5">
          <div className="inline-flex rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-0.5 text-sm">
            <button
              type="button"
              onClick={() => switchMode('form')}
              aria-pressed={mode === 'form'}
              className={`inline-flex items-center gap-1 rounded px-2 py-1 font-medium transition ${
                mode === 'form'
                  ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
                  : 'text-soft hover:text-[var(--text)]'
              }`}
            >
              <SlidersHorizontal className="h-3 w-3" />
              表单
            </button>
            <button
              type="button"
              onClick={() => switchMode('json')}
              aria-pressed={mode === 'json'}
              className={`inline-flex items-center gap-1 rounded px-2 py-1 font-medium transition ${
                mode === 'json'
                  ? 'bg-[var(--accent)] text-[var(--accent-contrast)]'
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
            className="rounded border border-[var(--danger-border)] bg-[var(--danger-soft)] p-1.5 text-[var(--danger)] hover:bg-[var(--danger)] hover:text-[var(--danger-contrast)] disabled:opacity-40"
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
              label="传输协议"
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
              label="目标端口"
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
            <div className="min-w-0 space-y-2">
              <label htmlFor={fieldId + '-domain'} className="block text-sm font-medium">
                域名匹配
              </label>
              <textarea
                id={fieldId + '-domain'}
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
              <details>
                <summary className="cursor-pointer text-sm text-soft">域名格式与常用条目</summary>
                <p className="my-2 text-sm leading-6 text-soft">每行一项，支持 domain、full、keyword、regexp 和 geosite 前缀。</p>
                <div className="flex flex-wrap gap-2">
                  {QUICK_DOMAINS.map((tok) => (
                    <button
                      key={tok}
                      type="button"
                      onClick={() => appendQuickToken('domain', tok)}
                      className="rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2 py-1 font-mono text-sm text-soft hover:border-[var(--accent)] hover:text-[var(--accent)]"
                    >
                      + {tok}
                    </button>
                  ))}
                </div>
              </details>
            </div>

            <div className="min-w-0 space-y-2">
              <label htmlFor={fieldId + '-ip'} className="block text-sm font-medium">
                IP 匹配
              </label>
              <textarea
                id={fieldId + '-ip'}
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
              <details>
                <summary className="cursor-pointer text-sm text-soft">IP 格式与常用条目</summary>
                <p className="my-2 text-sm leading-6 text-soft">每行一项，支持 IP、CIDR、geoip: 和 !geoip:。</p>
                <div className="flex flex-wrap gap-2">
                  {QUICK_IPS.map((tok) => (
                    <button
                      key={tok}
                      type="button"
                      onClick={() => appendQuickToken('ip', tok)}
                      className="rounded border border-[var(--border)] bg-[var(--panel-muted)] px-2 py-1 font-mono text-sm text-soft hover:border-[var(--accent)] hover:text-[var(--accent)]"
                    >
                      + {tok}
                    </button>
                  ))}
                </div>
              </details>
            </div>
          </div>

          <div className="space-y-1.5">
            <span className="block text-sm font-medium">
              嗅探协议（多选）
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
                    className={`rounded-md border px-2.5 py-1 font-mono text-sm transition ${
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
            <p className="text-sm text-[var(--warning)]">
              保存前请至少设置一项匹配条件：域名、目标 IP、端口或嗅探协议。
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
              try {
                const parsed = parseSingleObject<RoutingRuleItem>(text, '路由规则')
                const next = { type: 'field', ...parsed }
                setRawDraft({ text, source: JSON.stringify(next) })
                setRawError('')
                onRawErrorChange('')
                onChange(next)
              } catch (e) {
                setRawDraft({ text, source: serialized })
                const msg = (e as Error).message
                setRawError(msg)
                onRawErrorChange(msg)
              }
            }}
          />
          {rawError && (
            <p role="alert" className="text-sm text-[var(--danger)]">
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
  const v6DomainCount = domainLines(v6Text).length

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
    onRulesChange(next)
  }

  return (
    <div className="space-y-5">
      <section className="space-y-4 rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-base font-semibold">域名分流</h3>
            <p className="mt-1 text-sm text-soft">每行填写一项。未命中自定义规则时按域名选择出口。</p>
          </div>
          <Btn
            variant="secondary"
            disabled={disabled || !hasReferencePreset}
            aria-expanded={importOpen}
            onClick={() => setImportOpen(!importOpen)}
          >
            导入参考域名
          </Btn>
        </div>
        {importOpen && (
          <div className="space-y-3 break-words rounded border border-[var(--border)] bg-[var(--panel-strong)] p-4 text-sm">
            <p>起始列表不代表完整服务域名库。google.com 覆盖范围较广，默认不包含，可按需添加。</p>
            <p>IPv4：{preset4.join('、')}</p>
            <p>IPv6：{preset6.join('、')}</p>
            <div className="flex flex-wrap gap-2">
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
        {legacyV1 && (
          <p className="text-sm text-[var(--warning)]">
            正在转换旧版草稿：原先实际走 IPv4 的域名已归入 IPv4
            列表。保存并发布新版本、重新绑定后生效，旧版本不变。
          </p>
        )}
        <div className="grid gap-4 sm:grid-cols-2">
          <label className="block min-w-0 space-y-2 text-sm font-medium">
            <span className="flex items-baseline gap-2">
              强制 IPv4 域名
              <span className="font-normal text-soft">{domainLines(v4Text).length} 条</span>
            </span>
            <textarea
              rows={10}
              placeholder="domain:example.com"
              className={controlTextarea + ' font-mono font-normal'}
              value={v4Text}
              onChange={(e) => onV4Change(e.target.value)}
            />
          </label>
          <label className="block min-w-0 space-y-2 text-sm font-medium">
            <span className="flex items-baseline gap-2">
              强制 IPv6 域名
              <span className="font-normal text-soft">{v6DomainCount} 条</span>
            </span>
            <textarea
              rows={10}
              placeholder="geosite:netflix"
              className={controlTextarea + ' font-mono font-normal'}
              value={v6Text}
              onChange={(e) => onV6Change(e.target.value)}
            />
          </label>
        </div>
        {v6DomainCount > 0 && (
          <p className="text-sm text-[var(--warning)]">节点须有 IPv6 出口才能同步，连接失败时不会回退到 IPv4。</p>
        )}
      </section>

      <section className="space-y-4 rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-baseline gap-2">
            <h3 className="text-base font-semibold">自定义分流规则</h3>
            <span className="text-sm text-soft">{rules.length} 条</span>
          </div>
          <div className="flex flex-wrap gap-2">
            <Btn
              variant="secondary"
              disabled={disabled || rules.length >= 200}
              aria-expanded={snippetsOpen}
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
                <span className="text-sm font-semibold">{snippet.name}</span>
                <span className="text-sm leading-5 text-soft">{snippet.description}</span>
              </button>
            ))}
          </div>
        )}

        {rules.length > 0 && (
          <div className="space-y-3">
            <p className="text-sm text-soft">从上到下匹配，跳过已暂停的规则。</p>
            {rules.map((rule, idx) => (
              <SingleRuleCard
                key={editorItemKey(rule)}
                rule={rule}
                index={idx}
                total={rules.length}
                outbounds={outbounds}
                balancers={balancers}
                disabled={disabled}
                onMove={(delta) => moveRule(idx, delta)}
                onDelete={() => {
                  updateRuleError(editorItemKey(rule), '')
                  onRulesChange(rules.filter((_, i) => i !== idx))
                }}
                onRawErrorChange={(err) => updateRuleError(editorItemKey(rule), err)}
                onChange={(updated) => {
                  const next = [...rules]
                  next[idx] = retainEditorItemKey(rule, updated)
                  onRulesChange(next)
                }}
              />
            ))}
          </div>
        )}
      </section>

      <details className="rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] px-4 sm:px-5">
        <summary className="cursor-pointer py-4 text-base font-semibold">
          解析策略与保护规则
          {domainStrategy !== 'IPIfNonMatch' && (
            <span className="ml-2 text-sm font-normal text-soft">{domainStrategy}</span>
          )}
        </summary>
        <div className="space-y-4 border-t border-[var(--border)] py-4 text-sm">
          <SelectField
            label="路由域名解析策略（routing.domainStrategy）"
            value={domainStrategy}
            onChange={(v) => onDomainStrategyChange(v as DomainStrategy)}
            options={[
              { value: 'IPIfNonMatch', label: '域名未命中时解析 IP（IPIfNonMatch，默认）' },
              { value: 'AsIs', label: '使用原始域名或 IP（AsIs）' },
              { value: 'IPOnDemand', label: '遇到 IP 规则时解析（IPOnDemand）' },
            ]}
          />
          <p className="leading-6 text-soft">先执行系统保护规则，再匹配自定义规则、IPv4 域名和 IPv6 域名。全部未命中时使用默认直连。</p>
          <div className="border-t border-[var(--border)] pt-3">
            <h4 className="font-medium">系统保护规则（只读）</h4>
            <ol className="mt-2 list-decimal space-y-1 pl-5 leading-6 text-soft">
              <li>允许 <code>api-inbound → api</code>，用于本地 gRPC 热更新和流量统计。</li>
              <li>允许已开启 <code>allow_private</code> 的受控运维代理直连。</li>
              <li>通过 <code>geoip:private → block</code> 阻止访问节点私网。</li>
              <li>对普通 HTTP / SOCKS 运维代理执行源 IP 白名单和兜底拦截。</li>
            </ol>
          </div>
        </div>
      </details>

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
          <p className="text-sm leading-6 text-soft">{pendingSnippet?.description}</p>
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
