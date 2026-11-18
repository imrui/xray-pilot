import { Activity, Plus, Scale, Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, FieldGroup, SelectField, Switch } from '@/components/ui/Form'
import { editorItemKey, retainEditorItemKey } from '@/lib/routing-editor'
import {
  BUILTIN_OUTBOUNDS,
  countBalancerReferences,
  domainLines,
  validateOutboundTag,
} from '@/lib/routing-model'
import type {
  BalancerItem,
  BalancerStrategyType,
  CustomOutbound,
  ObservatoryConfig,
  RoutingRuleItem,
} from '@/lib/routing-model'

const STRATEGY_LABELS: Record<BalancerStrategyType, string> = {
  random: '随机分摊（random）',
  roundRobin: '轮询分发（roundRobin）',
  leastPing: '最低延迟择优（leastPing）',
}

function collectRequiredSelectors(balancers: BalancerItem[]): string[] {
  const set = new Set<string>()
  for (const b of balancers) {
    const st = b.strategy?.type ?? 'random'
    if (st === 'leastPing' || !!b.fallbackTag) {
      for (const s of b.selector ?? []) {
        if (s.trim()) set.add(s.trim())
      }
    }
  }
  return [...set]
}

export function BalancerListEditor({
  balancers,
  observatory,
  outbounds,
  rules,
  disabled = false,
  onBalancersChange,
  onObservatoryChange,
}: {
  balancers: BalancerItem[]
  observatory: ObservatoryConfig | null
  outbounds: CustomOutbound[]
  rules: RoutingRuleItem[]
  disabled?: boolean
  onBalancersChange: (next: BalancerItem[]) => void
  onObservatoryChange: (next: ObservatoryConfig | null) => void
}) {
  const outboundTags = outbounds.map((o) => o.tag).filter(Boolean)
  const needsObservatory = balancers.some(
    (b) => (b.strategy?.type ?? 'random') === 'leastPing' || !!b.fallbackTag,
  )
  const requiredSelectors = collectRequiredSelectors(balancers)

  const addBalancer = () => {
    let idx = balancers.length + 1
    let tag = `pool-${idx}`
    const usedTags = new Set([...outboundTags, ...balancers.map((b) => b.tag)])
    while (usedTags.has(tag)) {
      idx += 1
      tag = `pool-${idx}`
    }
    const defaultSelector = outboundTags.length > 0 ? [outboundTags[0]] : []
    onBalancersChange([
      ...balancers,
      {
        tag,
        remark: '',
        selector: defaultSelector,
        strategy: { type: 'leastPing' },
      },
    ])
  }

  const updateBalancer = (index: number, nextItem: BalancerItem) => {
    const next = [...balancers]
    next[index] = retainEditorItemKey(balancers[index], nextItem)
    onBalancersChange(next)
  }

  const toggleCandidate = (index: number, item: BalancerItem, candidateTag: string) => {
    const current = item.selector ?? []
    const nextSelector = current.includes(candidateTag)
      ? current.filter((s) => s !== candidateTag)
      : [...current, candidateTag]
    updateBalancer(index, { ...item, selector: nextSelector })
  }

  const fallbackBaseOptions = [
    { value: '', label: '不设置故障回退（默认）' },
    ...BUILTIN_OUTBOUNDS.filter((b) => b.tag !== 'block').map((b) => ({
      value: b.tag,
      label: b.label,
    })),
    ...outbounds
      .filter((o) => o.tag)
      .map((o) => ({
        value: o.tag,
        label: `${o.tag}（${o.protocol}${o.remark ? ` · ${o.remark}` : ''}）`,
      })),
  ]

  const missingSelectors =
    observatory != null
      ? requiredSelectors.filter((req) => !(observatory.subjectSelector ?? []).includes(req))
      : []

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel)] px-4 py-3">
        <div className="space-y-0.5">
          <div className="flex items-center gap-1.5 text-sm font-semibold">
            <Scale className="h-4 w-4 text-[var(--accent)]" />
            负载均衡组（Balancers）
          </div>
          <p className="text-xs text-soft">
            将多个自定义落地出站编组，按随机、轮询或实时最低延迟自动选路；支持配置全部故障时的兜底直连/备用出站。
          </p>
        </div>
        <Btn
          variant="secondary"
          disabled={disabled || balancers.length >= 16}
          onClick={addBalancer}
        >
          <Plus className="h-4 w-4" />
          添加负载均衡组（{balancers.length}/16）
        </Btn>
      </div>

      {balancers.length === 0 ? (
        <div className="rounded-lg border border-dashed border-[var(--border-strong)] bg-[var(--panel-muted)] p-6 text-center text-sm text-soft">
          当前模板未定义负载均衡组。先在「自定义出站」添加至少 1 个落地出站后，可在此创建自动择优或轮询均衡池，并在路由规则中直接选用。
        </div>
      ) : (
        <div className="space-y-4">
          {balancers.map((item, idx) => {
            const strategyType: BalancerStrategyType =
              item.strategy?.type === 'roundRobin' || item.strategy?.type === 'leastPing'
                ? item.strategy.type
                : 'random'
            const refCount = countBalancerReferences(item.tag, rules)
            const baseTagErr = validateOutboundTag(item.tag)
            const collidesWithOutbound = outboundTags.includes(item.tag)
            const duplicateBalancer =
              balancers.filter((b) => b.tag && b.tag === item.tag).length > 1
            const tagError =
              baseTagErr ||
              (collidesWithOutbound
                ? `Tag "${item.tag}" 已在自定义出站中使用，均衡组与出站不可重名`
                : duplicateBalancer
                  ? `Tag "${item.tag}" 重复`
                  : '')

            const extraSelectors = (item.selector ?? []).filter((s) => !outboundTags.includes(s))
            const matchedCount = (item.selector ?? []).filter((sel) =>
              [
                'direct',
                'direct-v4',
                'direct-v6',
                ...outboundTags,
              ].some((cand) => cand && sel && cand.startsWith(sel)),
            ).length

            return (
              <div
                key={editorItemKey(item)}
                className="space-y-3 rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4"
              >
                <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] pb-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-semibold text-[var(--text)]">
                      {item.tag || '未命名均衡组'}
                    </span>
                    <Badge label={STRATEGY_LABELS[strategyType]} variant="blue" />
                    <Badge
                      label={refCount > 0 ? `被规则引用 ${refCount} 次` : '未被引用'}
                      variant={refCount > 0 ? 'green' : 'gray'}
                    />
                    {item.fallbackTag && (
                      <Badge label={`兜底: ${item.fallbackTag}`} variant="yellow" />
                    )}
                  </div>
                  <Btn
                    variant="danger"
                    disabled={disabled || refCount > 0}
                    title={
                      refCount > 0
                        ? '该负载均衡组正被路由规则引用，请先修改规则目标后再删除'
                        : '删除此负载均衡组'
                    }
                    onClick={() => onBalancersChange(balancers.filter((_, i) => i !== idx))}
                    className="h-8 px-3 text-xs"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {refCount > 0 ? '使用中禁删' : '删除'}
                  </Btn>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <Field
                    label="均衡组 Tag *"
                    placeholder="如 hk-pool 或 ai-auto"
                    value={item.tag}
                    error={tagError}
                    onChange={(e) => {
                      const nextTag = e.target.value.trim()
                      updateBalancer(idx, { ...item, tag: nextTag })
                    }}
                  />
                  <Field
                    label="备注（仅面板展示）"
                    placeholder="如 香港多线自动择优池"
                    value={item.remark ?? ''}
                    onChange={(e) => {
                      const next = { ...item }
                      if (e.target.value) next.remark = e.target.value
                      else delete next.remark
                      updateBalancer(idx, next)
                    }}
                  />
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  <SelectField
                    label="调度策略（strategy.type）"
                    value={strategyType}
                    onChange={(st) =>
                      updateBalancer(idx, {
                        ...item,
                        strategy: { type: st as BalancerStrategyType },
                      })
                    }
                    options={[
                      { value: 'leastPing', label: STRATEGY_LABELS.leastPing },
                      { value: 'roundRobin', label: STRATEGY_LABELS.roundRobin },
                      { value: 'random', label: STRATEGY_LABELS.random },
                    ]}
                  />
                  <SelectField
                    label="故障回退兜底出站（fallbackTag，可选）"
                    value={item.fallbackTag ?? ''}
                    onChange={(fb) => {
                      const next = { ...item }
                      if (fb) next.fallbackTag = fb
                      else delete next.fallbackTag
                      updateBalancer(idx, next)
                    }}
                    options={fallbackBaseOptions}
                  />
                </div>

                <div className="space-y-2">
                  <span className="block text-[12px] font-medium text-soft">
                    勾选候选自定义出站（selector） *
                  </span>
                  {outbounds.length === 0 ? (
                    <p className="rounded border border-[var(--warning-border)] bg-[var(--warning-soft)] px-3 py-2 text-xs text-[var(--warning)]">
                      尚未添加任何自定义出站，请先切换到「自定义出站」面板创建候选节点，或在下方填写选择器前缀。
                    </p>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {outbounds
                        .filter((o) => o.tag)
                        .map((o) => {
                          const checked = (item.selector ?? []).includes(o.tag)
                          return (
                            <button
                              key={o.tag}
                              type="button"
                              aria-pressed={checked}
                              disabled={disabled}
                              onClick={() => toggleCandidate(idx, item, o.tag)}
                              className={`inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-medium transition ${
                                checked
                                  ? 'border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--accent)]'
                                  : 'border-[var(--border)] bg-[var(--panel-strong)] text-soft hover:text-[var(--text)]'
                              }`}
                            >
                              <span className="font-mono">{o.tag}</span>
                              <span className="text-[11px] opacity-80">
                                ({o.protocol}
                                {o.remark ? ` · ${o.remark}` : ''})
                              </span>
                            </button>
                          )
                        })}
                    </div>
                  )}

                  <Field
                    label="附加前缀选择器（可选，逗号分隔，兼容 JSON 前缀匹配如 hk-）"
                    placeholder="如 hk-, sg-"
                    value={extraSelectors.join(', ')}
                    onChange={(e) => {
                      const checkedExact = (item.selector ?? []).filter((s) =>
                        outboundTags.includes(s),
                      )
                      const extras = e.target.value
                        .split(',')
                        .map((s) => s.trim())
                        .filter(Boolean)
                      updateBalancer(idx, {
                        ...item,
                        selector: [...new Set([...checkedExact, ...extras])],
                      })
                    }}
                  />

                  {matchedCount === 0 && (
                    <p className="text-xs text-[var(--danger)]">
                      当前选择器未匹配到任何有效出站，保存时将被拦截。请至少勾选 1 个候选出站。
                    </p>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}

      {(needsObservatory || observatory != null) && (
        <FieldGroup
          title="健康探测（Observatory）设置"
          description="当任一均衡组使用最低延迟（leastPing）或设置故障回退（fallbackTag）时，Xray 需要顶层 Observatory 持续探测候选出站连通性与 RTT。"
        >
          <div className="flex items-center gap-2 text-xs text-soft">
            <Activity className="h-4 w-4 text-[var(--accent)]" />
            <span>
              当前需要观测的选择器：{requiredSelectors.length > 0 ? requiredSelectors.join('、') : '暂无'}
            </span>
          </div>

          <SelectField
            label="探测参数模式"
            value={observatory == null ? 'auto' : 'custom'}
            onChange={(mode) => {
              if (mode === 'auto') {
                onObservatoryChange(null)
              } else {
                onObservatoryChange({
                  subjectSelector:
                    requiredSelectors.length > 0
                      ? requiredSelectors
                      : outboundTags.length > 0
                        ? [outboundTags[0]]
                        : [''],
                  probeURL: 'https://1.1.1.1/',
                  probeInterval: '1m',
                  enableConcurrency: true,
                })
              }
            }}
            options={[
              {
                value: 'auto',
                label: '自动默认（推荐 · 自动汇总候选集，使用公网 IP 探测地址，每 1m 并发探测）',
              },
              {
                value: 'custom',
                label: '自定义探测参数（单独指定探测 URL、周期与观测范围）',
              },
            ]}
          />

          {observatory != null && (
            <div className="space-y-3 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3.5">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field
                  label="探测目标 URL（probeURL，仅限公网 IP，HTTPS 443）"
                  placeholder="https://1.1.1.1/"
                  value={observatory.probeURL ?? ''}
                  onChange={(e) => {
                    const next = { ...observatory }
                    const v = e.target.value.trim()
                    if (v) next.probeURL = v
                    else delete next.probeURL
                    onObservatoryChange(next)
                  }}
                />
                <Field
                  label="探测周期（probeInterval，范围 10s ~ 1h）"
                  placeholder="1m（支持如 30s、1m、5m）"
                  value={observatory.probeInterval ?? ''}
                  onChange={(e) => {
                    const next = { ...observatory }
                    const v = e.target.value.trim()
                    if (v) next.probeInterval = v
                    else delete next.probeInterval
                    onObservatoryChange(next)
                  }}
                />
              </div>

              <p className="text-xs text-soft">
                请使用明确的公网 IP，防止域名解析变化将健康探测导向内网。
              </p>
              <div className="flex items-center justify-between gap-3 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-2">
                <div>
                  <p className="text-xs font-medium text-[var(--text)]">
                    并发执行健康探测（enableConcurrency）
                  </p>
                  <p className="text-[11px] text-soft">
                    开启后同时探测多个候选落地出站，缩短首轮选路收敛时间。
                  </p>
                </div>
                <Switch
                  checked={!!observatory.enableConcurrency}
                  onChange={(checked) =>
                    onObservatoryChange({
                      ...observatory,
                      enableConcurrency: checked,
                    })
                  }
                />
              </div>

              <div className="space-y-1.5">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <label
                    htmlFor="observatory-subject-selector"
                    className="text-[12px] font-medium text-soft"
                  >
                    观测目标选择器（subjectSelector，每行一项） *
                  </label>
                  {requiredSelectors.length > 0 && missingSelectors.length > 0 && (
                    <button
                      type="button"
                      onClick={() =>
                        onObservatoryChange({
                          ...observatory,
                          subjectSelector: [
                            ...new Set([...(observatory.subjectSelector ?? []), ...requiredSelectors]),
                          ],
                        })
                      }
                      className="text-xs font-medium text-[var(--accent)] hover:underline"
                    >
                      补齐缺失候选（+{missingSelectors.length}）
                    </button>
                  )}
                </div>
                <textarea
                  id="observatory-subject-selector"
                  rows={3}
                  className="w-full rounded-md border border-[var(--border)] bg-[var(--panel)] p-2.5 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
                  value={(observatory.subjectSelector ?? []).join('\n')}
                  onChange={(e) =>
                    onObservatoryChange({
                      ...observatory,
                      subjectSelector: domainLines(e.target.value),
                    })
                  }
                />
                {missingSelectors.length > 0 && (
                  <p className="text-xs text-[var(--warning)]">
                    注意：均衡器依赖的候选「{missingSelectors.join('、')}」尚未包含在观测范围内，保存时将被拦截。
                  </p>
                )}
              </div>
            </div>
          )}
        </FieldGroup>
      )}
    </div>
  )
}
