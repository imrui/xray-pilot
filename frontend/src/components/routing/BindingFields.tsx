import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field, FieldGroup, SelectField } from '@/components/ui/Form'
import { useAuthStore } from '@/store/auth'
import { getRevisions, previewTemplate } from '@/lib/routing'
import {
  BUILTIN_OUTBOUNDS,
  basicRoutingDescription,
  basicRoutingLabel,
  bindingMode,
  createDefaultDNSConfig,
  domainLines,
  parseRules,
  revisionSummaryLabel,
} from '@/lib/routing-model'
import type {
  CustomOutbound,
  DNSConfigModel,
  OutboundOverride,
  RoutingBinding,
  RoutingTemplate,
} from '@/lib/routing-model'
import { DnsEditor } from './DnsEditor'
import { SingleOutboundFields } from './OutboundListEditor'

export const routingControl =
  'w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

export function BindingFields({
  value,
  onChange,
  templates,
  disabled = false,
  onPendingErrorChange,
}: {
  value: RoutingBinding
  onChange: (b: RoutingBinding) => void
  templates: RoutingTemplate[]
  disabled?: boolean
  onPendingErrorChange?: (hasError: boolean) => void
}) {
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
  const [outboundErrors, setOutboundErrors] = useState<Record<string, string>>({})
  const hasPendingError = Object.keys(outboundErrors).length > 0
  const updateOutboundError = (tag: string, error: string) => {
    const next = { ...outboundErrors }
    if (error) next[tag] = error
    else delete next[tag]
    setOutboundErrors(next)
    onPendingErrorChange?.(Object.keys(next).length > 0)
  }
  const revisions = useQuery({
    queryKey: ['routing-revisions', value.template_id],
    queryFn: () => getRevisions(value.template_id),
    enabled: value.mode === 'template' && value.template_id > 0,
  })
  const canPreview =
    value.scope === 'node' &&
    (value.mode === 'inherit' ||
      (value.mode === 'template' && value.template_id > 0 && value.revision > 0))
  const templatePreview = useQuery({
    queryKey: [
      'routing-template-preview',
      value.target_id,
      value.mode,
      value.template_id,
      value.revision,
    ],
    queryFn: () => previewTemplate(value),
    enabled: canPreview,
  })
  const base = canPreview ? templatePreview.data : undefined
  const templateName =
    base && (templates.find((t) => t.id === base.template_id)?.name ?? `模板 #${base.template_id}`)
  const overrides = JSON.parse(value.overrides || '{}') as Record<string, unknown>
  const override = (key: string, field: unknown) => {
    const next = { ...overrides }
    if (field === undefined) delete next[key]
    else next[key] = field
    onChange({ ...value, overrides: JSON.stringify(next) })
  }

  const inheritedOutbounds: CustomOutbound[] | undefined = templatePreview.error
    ? undefined
    : base?.policy
      ? (base.policy.custom_outbounds ?? [])
      : undefined

  const outboundOverridesMap = (
    overrides.outbound_overrides &&
    typeof overrides.outbound_overrides === 'object' &&
    !Array.isArray(overrides.outbound_overrides)
      ? overrides.outbound_overrides
      : {}
  ) as Record<string, OutboundOverride>

  const updateOutboundOverride = (tag: string, nextEntry: OutboundOverride | undefined) => {
    const nextMap = { ...outboundOverridesMap }
    if (!nextEntry) delete nextMap[tag]
    else nextMap[tag] = nextEntry
    override('outbound_overrides', Object.keys(nextMap).length > 0 ? nextMap : undefined)
  }

  const staleOutboundTags = inheritedOutbounds
    ? Object.keys(outboundOverridesMap).filter(
        (tag) => !inheritedOutbounds.some((o) => o.tag === tag),
      )
    : []

  return (
    <div className="space-y-4">
      <fieldset disabled={hasPendingError} className="space-y-4">
      <SelectField
        label="路由来源"
        value={value.mode}
        onChange={(mode) => onChange(bindingMode(value, mode as RoutingBinding['mode']))}
        options={[
          ...(value.scope !== 'global'
            ? [
                {
                  value: 'inherit',
                  label: value.scope === 'group' ? '继承全局默认' : '继承分组 / 全局默认',
                },
              ]
            : []),
          { value: 'template', label: '指定模板及版本' },
          ...(value.scope === 'node' ? [{ value: 'legacy', label: basicRoutingLabel }] : []),
        ]}
      />
      <p className="text-xs text-soft">
        {value.scope === 'global'
          ? '全局默认只影响最终继承到全局的节点；节点指定或分组指定的模板优先。保存后仍需节点同步生效。'
          : value.scope === 'group'
            ? '分组模板仅作用于继承该分组的节点；节点单独指定的模板优先。'
            : value.mode === 'legacy'
              ? `${basicRoutingDescription} 保存此来源会清除模板绑定和节点路由覆盖，节点同步后生效。`
              : value.mode === 'template'
                ? '为本节点指定模板及固定版本，不继承分组或全局模板。本节点覆盖仍有效，更换模板不会自动清除覆盖；节点同步后生效。'
                : '优先继承分组指定的模板，分组未指定时采用全局默认；本节点覆盖仍有效。恢复继承仍可能应用分流规则，节点同步后生效。'}
      </p>
      {value.mode === 'template' && (
        <div className="grid gap-4 sm:grid-cols-2">
          <SelectField
            label="路由模板"
            value={value.template_id}
            onChange={(id) => onChange({ ...value, template_id: Number(id), revision: 0 })}
            options={[
              { value: 0, label: '请选择模板' },
              ...templates
                .filter((t) => t.latest_revision > 0)
                .map((t) => ({ value: t.id, label: t.name })),
            ]}
          />
          <SelectField
            label="固定版本"
            value={value.revision}
            onChange={(revision) => onChange({ ...value, revision: Number(revision) })}
            options={[
              { value: 0, label: revisions.isLoading ? '加载版本…' : '请选择版本' },
              ...(revisions.data ?? []).map((r) => ({
                value: r.revision,
                label: `v${r.revision} · ${revisionSummaryLabel(r.payload)}`,
              })),
            ]}
          />
          {revisions.error && (
            <p role="alert" className="text-sm text-[var(--danger)]">
              版本加载失败。<button onClick={() => void revisions.refetch()}>重试</button>
            </p>
          )}
          {!!value.revision && (revisions.data?.[0]?.revision ?? 0) > value.revision && (
            <p className="text-xs text-soft">
              有 v{revisions.data![0].revision} 可用，当前仍使用 v{value.revision}。
            </p>
          )}
        </div>
      )}
      </fieldset>
      {hasPendingError && (
        <p role="alert" className="text-xs text-[var(--danger)]">
          节点出站仍有未完成或无效的输入，请修正或放弃修改后再切换来源、保存或预检。
        </p>
      )}
      {value.scope === 'node' && value.mode !== 'legacy' && (
        <FieldGroup
          title="本节点覆盖"
          description="默认继承模板，仅需特殊设置时才自定义。自定义会替换整组列表；清空规则仅移除本节点的该组域名规则，不修改模板。已有覆盖会保留。"
        >
          <div
            className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-xs"
            aria-live="polite"
          >
            {!canPreview ? (
              '请选择模板及固定版本，查看可继承的域名与出站。'
            ) : templatePreview.error ? (
              <div role="alert" className="space-y-2 text-[var(--danger)]">
                <p>模板内容加载失败：{(templatePreview.error as Error).message}</p>
                <Btn variant="secondary" onClick={() => void templatePreview.refetch()}>
                  重试加载模板
                </Btn>
              </div>
            ) : base ? (
              <p>
                当前模板：{templateName} v{base.revision} ·{' '}
                {value.mode === 'template'
                  ? '节点指定'
                  : base.groups.length
                    ? `继承分组：${base.groups.map((g) => g.split(' [')[0]).join('、')}`
                    : '继承全局默认'}
              </p>
            ) : (
              '正在读取模板域名…'
            )}
          </div>
          {base?.policy && (base.policy.schema_version ?? 1) < 2 && (
            <p className="text-xs text-soft">
              当前继承旧版策略，未命中仍使用 IPv4 兜底。
              {base.policy.enable_ipv6_outbound
                ? ''
                : '旧版 IPv6 开关已关闭，IPv6 列表中的域名实际使用 IPv4。'}
            </p>
          )}
          <p className="text-xs text-soft">
            新版策略中，未命中规则使用默认直连；强制 IPv6 需要节点有 IPv6
            出口且不会回退。两组域名重叠时 IPv4 优先。
          </p>
          {overrides.enable_ipv6_outbound != null && (
            <div className="space-y-2 rounded border border-[var(--warning-border)] p-3 text-xs">
              <p>
                此节点仍有旧版 IPv6
                开关覆盖。使用新版模板前，请移除该覆盖，并用下面两组域名列表明确选择出口。
              </p>
              <Btn variant="secondary" onClick={() => override('enable_ipv6_outbound', undefined)}>
                移除旧版开关覆盖
              </Btn>
            </div>
          )}

          {staleOutboundTags.length > 0 && (
            <div className="space-y-2 rounded border border-[var(--warning-border)] bg-[var(--warning-soft)] p-3 text-xs">
              <p className="text-[var(--warning)]">
                检测到失效的节点出站覆盖：{staleOutboundTags.join('、')} 在当前模板版本中已不存在，保存前请清理。
              </p>
              {superAdmin && (
                <Btn
                  variant="secondary"
                  onClick={() => {
                    const cleaned = { ...outboundOverridesMap }
                    for (const t of staleOutboundTags) delete cleaned[t]
                    const remainingErrors = { ...outboundErrors }
                    for (const t of staleOutboundTags) delete remainingErrors[t]
                    setOutboundErrors(remainingErrors)
                    onPendingErrorChange?.(Object.keys(remainingErrors).length > 0)
                    override('outbound_overrides', Object.keys(cleaned).length ? cleaned : undefined)
                  }}
                >
                  清理失效出站覆盖
                </Btn>
              )}
            </div>
          )}

          {inheritedOutbounds && inheritedOutbounds.length > 0 && (
            <div className="space-y-3 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3.5">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <h5 className="text-xs font-semibold text-[var(--text)]">
                    本节点自定义出站覆盖（{inheritedOutbounds.length} 个模板出站）
                  </h5>
                  <p className="mt-0.5 text-xs text-soft">
                    可按出站 Tag 为当前节点单独替换落地地址/协议参数，或将规则重映射到其他出站。
                  </p>
                </div>
                {!superAdmin && <Badge label="仅超级管理员可修改" variant="gray" />}
              </div>

              <div className="space-y-3">
                {inheritedOutbounds.map((tmplOutbound) => {
                  const currentOv = outboundOverridesMap[tmplOutbound.tag] as OutboundOverride | undefined
                  const ovMode: 'inherit' | 'replace' | 'remap' = currentOv?.mode ?? 'inherit'
                  const peerTags = inheritedOutbounds
                    .map((o) => o.tag)
                    .filter((t) => outboundOverridesMap[t]?.mode !== 'remap')

                  const remapOptions = [
                    ...BUILTIN_OUTBOUNDS.map((b) => ({ value: b.tag, label: b.label })),
                    ...inheritedOutbounds
                      .filter(
                        (o) =>
                          o.tag !== tmplOutbound.tag &&
                          outboundOverridesMap[o.tag]?.mode !== 'remap',
                      )
                      .map((o) => ({
                        value: o.tag,
                        label: `${o.tag}（模板自定义出站 · ${o.protocol}）`,
                      })),
                  ]

                  return (
                    <fieldset
                      key={tmplOutbound.tag}
                      disabled={!superAdmin}
                      className="space-y-3 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-3 disabled:opacity-75"
                    >
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex items-center gap-2 text-xs">
                          <span className="font-mono font-semibold text-[var(--text)]">
                            {tmplOutbound.tag}
                          </span>
                          <Badge label={tmplOutbound.protocol.toUpperCase()} variant="blue" />
                          {tmplOutbound.remark && (
                            <span className="text-soft">{tmplOutbound.remark}</span>
                          )}
                        </div>
                        <Badge
                          label={
                            ovMode === 'inherit'
                              ? '继承模板'
                              : ovMode === 'remap'
                                ? `重映射 → ${currentOv?.target_tag || ''}`
                                : '本节点参数替换'
                          }
                          variant={ovMode === 'inherit' ? 'gray' : 'yellow'}
                        />
                      </div>

                      <SelectField
                        label={`出站覆盖模式 · ${tmplOutbound.tag}`}
                        value={ovMode}
                        onChange={(nextMode) => {
                          updateOutboundError(tmplOutbound.tag, '')
                          if (nextMode === 'inherit') {
                            updateOutboundOverride(tmplOutbound.tag, undefined)
                          } else if (nextMode === 'remap') {
                            updateOutboundOverride(tmplOutbound.tag, {
                              mode: 'remap',
                              target_tag: 'direct',
                            })
                          } else {
                            updateOutboundOverride(tmplOutbound.tag, {
                              mode: 'replace',
                              outbound: { ...tmplOutbound, tag: tmplOutbound.tag },
                            })
                          }
                        }}
                        options={[
                          { value: 'inherit', label: '继承模板参数' },
                          { value: 'remap', label: '重映射到其他出站（remap）' },
                          { value: 'replace', label: '自定义本节点参数（replace）' },
                        ]}
                      />

                      {ovMode === 'remap' && (
                        <SelectField
                          label={`重映射目标出站 · ${tmplOutbound.tag}`}
                          value={currentOv?.target_tag ?? 'direct'}
                          onChange={(targetTag) =>
                            updateOutboundOverride(tmplOutbound.tag, {
                              mode: 'remap',
                              target_tag: targetTag,
                            })
                          }
                          options={remapOptions}
                        />
                      )}

                      {ovMode === 'replace' && (
                        <div className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3">
                          <SingleOutboundFields
                            outbound={currentOv?.outbound ?? { ...tmplOutbound }}
                            lockedTag={tmplOutbound.tag}
                            peerOutboundTags={peerTags}
                            disabled={disabled || !superAdmin}
                            onRawErrorChange={(error) => updateOutboundError(tmplOutbound.tag, error)}
                            onChange={(updatedOutbound) =>
                              updateOutboundOverride(tmplOutbound.tag, {
                                mode: 'replace',
                                outbound: { ...updatedOutbound, tag: tmplOutbound.tag },
                              })
                            }
                          />
                        </div>
                      )}
                    </fieldset>
                  )
                })}
              </div>
            </div>
          )}

          {(['ipv4_force_domains', 'ipv6_force_domains'] as const).map((key, index) => (
            <DomainOverride
              key={key}
              family={index ? 'IPv6' : 'IPv4'}
              value={overrides[key] as string[] | null | undefined}
              inherited={templatePreview.error ? undefined : base?.policy?.[key]}
              onChange={(v) => override(key, v)}
            />
          ))}

          <DnsOverrideSection
            value={overrides.dns_override as DNSConfigModel | null | undefined}
            inherited={templatePreview.error ? undefined : base?.policy?.dns}
            superAdmin={superAdmin}
            onChange={(v) => override('dns_override', v)}
          />

          {superAdmin && (
            <details>
              <summary className="cursor-pointer text-sm">高级自定义规则</summary>
              <RulesOverride
                value={overrides.custom_rules}
                onChange={(v) => override('custom_rules', v)}
              />
            </details>
          )}
        </FieldGroup>
      )}
      {value.scope === 'node' && superAdmin && (
        <details>
          <summary className="cursor-pointer text-sm">高级设置</summary>
          <div className="mt-3 space-y-4">
            <Field
              label="Xray 配置路径（留空使用默认）"
              value={value.config_path ?? ''}
              placeholder="/usr/local/etc/xray/config.json"
              onChange={(e) => onChange({ ...value, config_path: e.target.value })}
            />
          </div>
        </details>
      )}
    </div>
  )
}

function DomainOverride({
  family,
  value,
  inherited,
  onChange,
}: {
  family: string
  value: string[] | null | undefined
  inherited: string[] | undefined
  onChange: (value: string[] | undefined) => void
}) {
  const mode = value == null ? 'inherit' : value.length === 0 ? 'clear' : 'custom'
  const emptyCustom = mode === 'custom' && domainLines(value!.join('\n')).length === 0
  return (
    <div className="space-y-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3">
      <SelectField
        label={`强制 ${family} 域名`}
        value={mode}
        onChange={(v) =>
          onChange(
            v === 'inherit'
              ? undefined
              : v === 'clear'
                ? []
                : inherited?.length
                  ? [...inherited]
                  : [''],
          )
        }
        options={[
          { value: 'inherit', label: '继承模板' },
          { value: 'custom', label: '自定义列表' },
          { value: 'clear', label: '清空规则' },
        ]}
      />
      {mode === 'inherit' &&
        (inherited ? (
          inherited.length ? (
            <>
              <p className="text-xs text-soft">
                继承模板中的 {inherited.length} 条域名（只读）。需要调整时选择“自定义列表”。
              </p>
              <textarea
                aria-label={`${family} 模板域名`}
                readOnly
                rows={Math.min(6, inherited.length)}
                className={routingControl + ' bg-[var(--panel-muted)]'}
                value={inherited.join('\n')}
              />
            </>
          ) : (
            <p className="text-xs text-soft">模板未配置强制 {family} 域名，无需额外设置。</p>
          )
        ) : (
          <p className="text-xs text-soft">模板域名尚未加载，请先确认上方的路由来源。</p>
        ))}
      {mode === 'custom' && (
        <>
          <p className="text-xs text-soft">
            仅用于本节点，替换模板中的整组列表；每行一条域名规则。
          </p>
          <textarea
            aria-label={`${family} 覆盖域名`}
            aria-invalid={emptyCustom}
            rows={4}
            className={routingControl}
            value={value!.join('\n')}
            onChange={(e) => onChange(e.target.value.split('\n'))}
          />
          {emptyCustom && (
            <p role="alert" className="text-xs text-[var(--danger)]">
              请填写域名；如需移除该组域名规则，请选择“清空规则”。
            </p>
          )}
        </>
      )}
      {mode === 'clear' && (
        <p className="text-xs text-soft">
          本节点不使用强制 {family} 域名列表，包括模板中的该组域名。
          其他路由规则仍有效；恢复使用请选择“继承模板”。
        </p>
      )}
    </div>
  )
}

function RulesOverride({ value, onChange }: { value: unknown; onChange: (v: unknown) => void }) {
  const [raw, setRaw] = useState<string | null>(null)
  const [error, setError] = useState('')
  return (
    <div className="mt-3 space-y-2">
      <SelectField
        label="自定义规则"
        value={value == null ? 'inherit' : 'override'}
        onChange={(v) => {
          setRaw(null)
          setError('')
          onChange(v === 'inherit' ? undefined : [])
        }}
        options={[
          { value: 'inherit', label: '继承模板' },
          { value: 'override', label: '本节点覆盖' },
        ]}
      />
      {value != null && (
        <textarea
          aria-label="自定义规则 JSON"
          rows={6}
          className={routingControl + ' font-mono'}
          value={raw ?? JSON.stringify(value, null, 2)}
          onChange={(e) => {
            setRaw(e.target.value)
            try {
              onChange(parseRules(e.target.value))
              setError('')
            } catch (error) {
              onChange(e.target.value)
              setError((error as Error).message)
            }
          }}
        />
      )}
      {error && (
        <p role="alert" className="text-sm text-[var(--danger)]">
          {error}（请修正后再保存）
        </p>
      )}
    </div>
  )
}

function DnsOverrideSection({
  value,
  inherited,
  superAdmin,
  onChange,
}: {
  value: DNSConfigModel | null | undefined
  inherited: DNSConfigModel | null | undefined
  superAdmin: boolean
  onChange: (next: DNSConfigModel | undefined) => void
}) {
  const mode: 'inherit' | 'disable' | 'custom' =
    value == null ? 'inherit' : value.enabled === false ? 'disable' : 'custom'

  return (
    <fieldset
      disabled={!superAdmin}
      className="space-y-3 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 disabled:opacity-75"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h5 className="text-xs font-semibold text-[var(--text)]">本节点服务端 DNS 覆盖</h5>
          <p className="mt-0.5 text-xs text-soft">
            可针对不同地区节点单独关闭自定义 DNS，或替换为当前节点授权的流媒体解锁 DNS IP。
          </p>
        </div>
        {!superAdmin && <Badge label="仅超级管理员可修改" variant="gray" />}
      </div>

      <SelectField
        label="本节点 DNS 模式"
        value={mode}
        onChange={(nextMode) => {
          if (nextMode === 'inherit') {
            onChange(undefined)
          } else if (nextMode === 'disable') {
            onChange({ enabled: false })
          } else {
            const seed =
              inherited && inherited.enabled
                ? (JSON.parse(JSON.stringify(inherited)) as DNSConfigModel)
                : createDefaultDNSConfig()
            onChange({ ...seed, enabled: true })
          }
        }}
        options={[
          { value: 'inherit', label: '继承模板 DNS' },
          { value: 'disable', label: '本节点关闭自定义 DNS（回退系统解析）' },
          { value: 'custom', label: '自定义本节点 DNS（独立替换）' },
        ]}
      />

      {mode === 'inherit' && (
        <p className="text-xs text-soft">
          {inherited?.enabled
            ? `当前继承模板已启用的自定义 DNS（共 ${inherited.servers?.length ?? 0} 个上游服务器，策略 ${inherited.queryStrategy ?? 'UseIP'}）。`
            : '当前继承模板未启用自定义 DNS，本节点使用系统默认解析。'}
        </p>
      )}

      {mode === 'disable' && (
        <p className="text-xs text-soft">
          已为本节点显式关闭服务端自定义 DNS，将回退使用节点系统默认解析，不影响模板与其他节点。
        </p>
      )}

      {mode === 'custom' && value && (
        <div className="rounded-md border border-[var(--border)] bg-[var(--panel-muted)] p-3">
          <DnsEditor
            dns={value}
            disabled={!superAdmin}
            hideEnableToggle
            onChange={(nextDns) => {
              if (nextDns) onChange({ ...nextDns, enabled: true })
            }}
          />
        </div>
      )}
    </fieldset>
  )
}
