import { useQuery } from '@tanstack/react-query'
import { Btn, Field, FieldGroup, SelectField } from '@/components/ui/Form'
import { useAuthStore } from '@/store/auth'
import { getRevisions, previewTemplate } from '@/lib/routing'
import {
  basicRoutingDescription,
  basicRoutingLabel,
  bindingMode,
  domainLines,
  readPolicy,
} from '@/lib/routing-model'
import type { RoutingBinding, RoutingTemplate } from '@/lib/routing-model'

export const routingControl =
  'w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-sm text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]'

export function BindingFields({
  value,
  onChange,
  templates,
}: {
  value: RoutingBinding
  onChange: (b: RoutingBinding) => void
  templates: RoutingTemplate[]
}) {
  const superAdmin = useAuthStore((s) => s.role) === 'super_admin'
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
      'routing-template-preview', value.target_id, value.mode, value.template_id, value.revision,
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
  return (
    <div className="space-y-4">
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
                label: `v${r.revision}${readPolicy(r.payload).schema_version === 2 ? ' · 默认直连' : ' · 旧版 IPv4 兜底'}`,
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
              '请选择模板及固定版本，查看可继承的域名。'
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
                {value.mode === 'template' ? '节点指定' : base.groups.length
                  ? `继承分组：${base.groups.map((g) => g.split(' [')[0]).join('、')}`
                  : '继承全局默认'}
              </p>
            ) : '正在读取模板域名…'}
          </div>
          {base?.policy && base.policy.schema_version !== 2 && (
            <p className="text-xs text-soft">
              当前继承旧版策略，未命中仍使用 IPv4 兜底。
              {base.policy.enable_ipv6_outbound ? '' : '旧版 IPv6 开关已关闭，IPv6 列表中的域名实际使用 IPv4。'}
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
          {(['ipv4_force_domains', 'ipv6_force_domains'] as const).map((key, index) => (
            <DomainOverride
              key={key}
              family={index ? 'IPv6' : 'IPv4'}
              value={overrides[key] as string[] | null | undefined}
              inherited={templatePreview.error ? undefined : base?.policy?.[key]}
              onChange={(v) => override(key, v)}
            />
          ))}
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
      {value.scope === 'node' && (
        <details>
          <summary className="cursor-pointer text-sm">高级设置</summary>
          <div className="mt-3 space-y-4">
            <UDPPolicy
              value={value.udp443_policy}
              onChange={(udp443_policy) => onChange({ ...value, udp443_policy })}
            />
            {superAdmin && (
              <Field
                label="Xray 配置路径（留空使用默认）"
                value={value.config_path ?? ''}
                placeholder="/usr/local/etc/xray/config.json"
                onChange={(e) => onChange({ ...value, config_path: e.target.value })}
              />
            )}
          </div>
        </details>
      )}
    </div>
  )
}

import { useState } from 'react'
import { parseRules } from '@/lib/routing-model'
function DomainOverride({ family, value, inherited, onChange }: {
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
        onChange={(v) => onChange(
          v === 'inherit' ? undefined : v === 'clear' ? [] : inherited?.length ? [...inherited] : [''],
        )}
        options={[
          { value: 'inherit', label: '继承模板' },
          { value: 'custom', label: '自定义列表' },
          { value: 'clear', label: '清空规则' },
        ]}
      />
      {mode === 'inherit' && (inherited ? inherited.length ? (
        <>
          <p className="text-xs text-soft">
            继承模板中的 {inherited.length} 条域名（只读）。需要调整时选择“自定义列表”。
          </p>
          <textarea
            aria-label={`${family} 模板域名`} readOnly rows={Math.min(6, inherited.length)}
            className={routingControl + ' bg-[var(--panel-muted)]'} value={inherited.join('\n')}
          />
        </>
      ) : (
        <p className="text-xs text-soft">模板未配置强制 {family} 域名，无需额外设置。</p>
      ) : (
        <p className="text-xs text-soft">模板域名尚未加载，请先确认上方的路由来源。</p>
      ))}
      {mode === 'custom' && (
        <>
          <p className="text-xs text-soft">仅用于本节点，替换模板中的整组列表；每行一条域名规则。</p>
          <textarea
            aria-label={`${family} 覆盖域名`} aria-invalid={emptyCustom}
            rows={4} className={routingControl} value={value!.join('\n')}
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
export function UDPPolicy({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  return (
    <div className="space-y-2">
      <SelectField
        label="客户端 UDP 443 策略"
        value={value}
        onChange={onChange}
        options={[
          { value: 'inherit', label: '继承上级策略（未指定时阻止）' },
          { value: 'block', label: '阻止 UDP 443' },
          { value: 'allow', label: '允许 UDP 443' },
        ]}
      />
      <p className="text-xs text-soft">
        仅影响 Vision 客户端订阅，不是节点防火墙规则。优先级：用户 → 节点 → 全局；均未指定时阻止。
        阻止使用 xtls-rprx-vision，允许使用 xtls-rprx-vision-udp443（更新订阅后生效）。
        允许时，Clash / sing-box 订阅会排除无法等价表达此策略的 Vision 条目。
      </p>
    </div>
  )
}
