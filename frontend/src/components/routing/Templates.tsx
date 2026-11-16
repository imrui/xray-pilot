import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Drawer } from '@/components/ui/Drawer'
import { Btn, Field, FieldGroup, SelectField } from '@/components/ui/Form'
import { useConfirm } from '@/lib/confirm-context'
import { useAuthStore } from '@/store/auth'
import { routingControl } from './BindingFields'
import {
  domainLines,
  nodesLink,
  parseRules,
  readPolicy,
  upgradeRoutingPolicy,
  routingLabel,
  templateBinding,
} from '@/lib/routing-model'
import type { RoutingPolicy, RoutingTemplate } from '@/lib/routing-model'
import {
  getBinding,
  getRevisions,
  invalidateRouting,
  routingGet,
  saveBinding,
  useRoutingOverview,
} from '@/lib/routing'
import request from '@/lib/axios'
import { PreviewConfigModal } from '@/components/nodes/ConfigPreview'
import { NodeRoutingPreview } from './NodeRoutingPreview'

const preset4 = [
  'anthropic.com',
  'claude.ai',
  'openai.com',
  'chatgpt.com',
  'x.ai',
  'grok.com',
  'gemini.google.com',
  'aistudio.google.com',
].map((x) => 'domain:' + x)
const preset6 = ['youtube.com', 'googlevideo.com', 'google.com', 'netflix.com'].map(
  (x) => 'domain:' + x,
)

export function TemplateEditor({
  template,
  onClose,
  onConfigure,
}: {
  template: RoutingTemplate
  onClose: () => void
  onConfigure: (t: RoutingTemplate) => void
}) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const writable = useAuthStore((s) => s.role) === 'super_admin'
  const [base, setBase] = useState(template)
  const [name, setName] = useState(template.name)
  const [description, setDescription] = useState(template.description)
  const initial = upgradeRoutingPolicy(template.edit_payload ?? template.draft)
  const [v4, setV4] = useState(initial.ipv4_force_domains.join('\n'))
  const [v6, setV6] = useState(initial.ipv6_force_domains.join('\n'))
  const [rules, setRules] = useState(JSON.stringify(initial.custom_rules, null, 2))
  const [importOpen, setImportOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [message, setMessage] = useState('')
  const original = upgradeRoutingPolicy(base.draft)
  const legacy = readPolicy(base.draft).schema_version !== 2
  const dirty =
    name !== base.name ||
    description !== base.description ||
    legacy ||
    v4 !== original.ipv4_force_domains.join('\n') ||
    v6 !== original.ipv6_force_domains.join('\n') ||
    rules !== JSON.stringify(original.custom_rules, null, 2)
  const close = async () =>
    !dirty ||
    (await confirm({
      title: '放弃未保存的模板修改？',
      description: '已发布的版本不会改变。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    }))
  const save = async (publish: boolean) => {
    setBusy(true)
    setError('')
    setMessage('')
    try {
      if (!name.trim()) throw new Error('请填写模板名称。')
      const policy: RoutingPolicy = {
        schema_version: 2,
        ipv4_force_domains: domainLines(v4),
        ipv6_force_domains: domainLines(v6),
        ipv6_failure_policy: 'strict',
        custom_rules: parseRules(rules),
      }
      const payload = { name: name.trim(), description, draft: policy, version: base.version }
      const res = base.id
        ? await request.patch(`/routing-templates/${base.id}`, payload)
        : await request.post('/routing-templates', payload)
      let saved = { ...base, ...res.data.data } as RoutingTemplate
      setBase(saved)
      setName(saved.name)
      setV4(policy.ipv4_force_domains.join('\n'))
      setV6(policy.ipv6_force_domains.join('\n'))
      setRules(JSON.stringify(policy.custom_rules, null, 2))
      setMessage('草稿已保存。')
      if (publish) {
        const r = await request.post(`/routing-templates/${saved.id}/publish`, {
          version: saved.version,
        })
        saved = {
          ...saved,
          version: saved.version + 1,
          latest_revision: r.data.data.revision,
          draft_changed: false,
        }
        setBase(saved)
        setMessage(`已发布 v${saved.latest_revision}。现有节点仍保持原绑定版本，尚未同步任何节点。`)
      }
      await invalidateRouting(qc)
    } catch (e) {
      setError((e as Error).message)
      await invalidateRouting(qc)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Drawer
      open
      width="lg"
      onClose={onClose}
      title={template.id ? `编辑路由模板 · ${template.name}` : '新增路由模板'}
      description="保存草稿用于继续编辑；发布版本后才能被节点选用。"
      dirty={dirty}
      saving={busy}
      onBeforeClose={close}
      footer={
        <>
          <Btn
            variant="secondary"
            disabled={busy}
            onClick={() =>
              void close().then((ok) => {
                if (ok) onClose()
              })
            }
          >
            取消
          </Btn>
          {writable && (
            <>
              <Btn variant="secondary" loading={busy} onClick={() => void save(false)}>
                保存草稿
              </Btn>
              <Btn loading={busy} onClick={() => void save(true)}>
                保存并发布版本
              </Btn>
            </>
          )}{' '}
        </>
      }
    >
      <fieldset disabled={!writable || busy} className="space-y-4">
        <FieldGroup title="基本信息">
          <Field label="模板名称 *" value={name} onChange={(e) => setName(e.target.value)} />
          <Field
            label="说明"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </FieldGroup>
        <FieldGroup
          title="域名路由"
          description="未命中规则保持默认直连。每行一条，例如 domain:example.com；自定义规则优先于域名列表，两组域名重叠时 IPv4 优先。"
        >
          <p className="text-xs text-soft">
            默认出站：正常直连，不强制 IPv4 或 IPv6。两组列表留空时保持默认行为。
          </p>
          {legacy && (
            <p className="text-xs text-[var(--warning)]">
              正在转换旧版草稿：原先实际走 IPv4 的域名已归入 IPv4
              列表。保存并发布新版本、重新绑定后生效，旧版本不变。
            </p>
          )}
          <label className="block space-y-2 text-sm">
            <span>强制 IPv4 域名 · {domainLines(v4).length} 条</span>
            <textarea
              rows={5}
              className={routingControl}
              value={v4}
              onChange={(e) => setV4(e.target.value)}
            />
          </label>
          <label className="block space-y-2 text-sm">
            <span>强制 IPv6 域名 · {domainLines(v6).length} 条</span>
            <textarea
              rows={5}
              className={routingControl}
              value={v6}
              onChange={(e) => setV6(e.target.value)}
            />
          </label>
          <p className="text-xs text-soft">
            强制 IPv6 需要节点有 IPv6 出口，失败不回退
            IPv4；无出口的节点会在同步前被拦截。策略与客户端使用 IPv4 或 IPv6 连接节点无关。
          </p>
          <Btn variant="secondary" onClick={() => setImportOpen(!importOpen)}>
            导入参考域名
          </Btn>
          {importOpen && (
            <div className="space-y-3 rounded border border-[var(--border)] p-3 text-xs">
              <p>IPv4：{preset4.join('、')}</p>
              <p>IPv6：{preset6.join('、')}</p>
              <div className="flex gap-2">
                {[false, true].map((replace) => (
                  <Btn
                    key={String(replace)}
                    variant="secondary"
                    onClick={() => {
                      setV4(
                        domainLines(
                          [...(replace ? [] : domainLines(v4)), ...preset4].join('\n'),
                        ).join('\n'),
                      )
                      setV6(
                        domainLines(
                          [...(replace ? [] : domainLines(v6)), ...preset6].join('\n'),
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
        <details>
          <summary className="cursor-pointer text-sm">高级自定义规则</summary>
          <label className="mt-3 block space-y-2 text-sm">
            <span>路由规则 JSON 数组</span>
            <textarea
              rows={8}
              className={routingControl + ' font-mono'}
              value={rules}
              onChange={(e) => setRules(e.target.value)}
            />
          </label>
        </details>
      </fieldset>
      {error && (
        <p role="alert" className="mt-4 text-sm text-[var(--danger)]">
          {error} 输入已保留。
        </p>
      )}
      {message && (
        <div role="status" className="mt-4 space-y-3 text-sm">
          <p>{message}</p>
          {base.latest_revision > 0 && !dirty && (
            <Btn variant="secondary" onClick={() => onConfigure(base)}>
              配置节点
            </Btn>
          )}
        </div>
      )}
    </Drawer>
  )
}

export function TemplateHistory({
  template,
  onClose,
  onEdit,
}: {
  template: RoutingTemplate
  onClose: () => void
  onEdit: (t: RoutingTemplate) => void
}) {
  const query = useQuery({
    queryKey: ['routing-revisions', template.id],
    queryFn: () => getRevisions(template.id),
  })
  const overview = useRoutingOverview()
  const confirm = useConfirm()
  const writable = useAuthStore((s) => s.role) === 'super_admin'
  return (
    <Drawer
      open
      width="lg"
      onClose={onClose}
      title={`版本历史与引用 · ${template.name}`}
      footer={
        <Btn variant="secondary" onClick={onClose}>
          关闭
        </Btn>
      }
    >
      <p className="mb-4 text-sm text-soft">
        已发布版本只读。旧节点保持固定版本，发布新版本不会自动切换绑定。
      </p>
      {query.isLoading && <p>加载中…</p>}
      {query.error && (
        <Btn variant="secondary" onClick={() => void query.refetch()}>
          加载失败，重试
        </Btn>
      )}
      <div className="space-y-3">
        {query.data?.map((r) => (
          <details key={r.revision} className="rounded-md border border-[var(--border)] p-3">
            <summary className="cursor-pointer text-sm">
              v{r.revision} ·{' '}
              {readPolicy(r.payload).schema_version === 2 ? '默认直连' : '旧版 IPv4 兜底'} ·{' '}
              {new Date(r.created_at).toLocaleString()} · {r.actor}
            </summary>
            <pre className="my-3 max-h-72 overflow-auto text-xs">
              {JSON.stringify(JSON.parse(r.payload), null, 2)}
            </pre>
            {writable && (
              <Btn
                variant="secondary"
                onClick={async () => {
                  if (
                    await confirm({
                      title: `以 v${r.revision} 编辑草稿？`,
                      description: '保存时会替换现有草稿，已发布版本保持不变。',
                      confirmText: '编辑草稿',
                      cancelText: '取消',
                    })
                  ) {
                    const latest = await routingGet<RoutingTemplate>(
                      `/routing-templates/${template.id}`,
                    )
                    onEdit({ ...template, ...latest, edit_payload: r.payload })
                  }
                }}
              >
                以此版本编辑草稿
              </Btn>
            )}
          </details>
        ))}
      </div>
      <h4 className="mt-5 text-sm font-semibold">直接绑定</h4>
      <div className="mt-2 space-y-2">
        {overview.data?.references
          ?.filter((b) => b.template_id === template.id)
          .map((b) => (
            <p key={`${b.scope}:${b.target_id}`} className="text-sm">
              {({ global: '全局', group: '分组', node: '节点' } as Record<string, string>)[
                b.scope
              ] ?? b.scope}{' '}
              · {b.target_name || '已删除对象'} · v{b.revision}
            </p>
          ))}
      </div>
      <h4 className="mt-5 text-sm font-semibold">实际使用节点</h4>
      {overview.error && <p role="alert">引用加载失败。</p>}
      <div className="mt-2 space-y-2">
        {overview.data?.nodes
          .filter((n) => n.routing?.template_id === template.id)
          .map((n) => (
            <Link className="block text-sm text-[var(--accent)]" key={n.id} to={nodesLink([n.id])}>
              {n.name} · {routingLabel(n, overview.data!.templates)}
            </Link>
          ))}
      </div>
    </Drawer>
  )
}

export function TemplateNodes({
  template,
  onClose,
}: {
  template: RoutingTemplate
  onClose: () => void
}) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const overview = useRoutingOverview()
  const revisions = useQuery({
    queryKey: ['routing-revisions', template.id],
    queryFn: () => getRevisions(template.id),
  })
  const [selected, setSelected] = useState<number[]>([])
  const [activeID, setActiveID] = useState<number | null>(null)
  const [preview, setPreview] = useState(false)
  const [revision, setRevision] = useState(0)
  const [search, setSearch] = useState('')
  const [busy, setBusy] = useState(false)
  const [results, setResults] = useState<{ id: number; error?: string; changed: number[] }[]>([])
  const rows = (overview.data?.nodes ?? []).filter((n) =>
    `${n.name} ${n.ip}`.toLowerCase().includes(search.toLowerCase()),
  )
  const activeNode = overview.data?.nodes.find((n) => n.id === activeID)
  const targets = selected.length ? selected : activeNode ? [activeNode.id] : []
  const dirty = targets.length > 0 && revision > 0
  const close = async () =>
    !dirty ||
    (await confirm({
      title: '放弃未保存的路由绑定？',
      description: '已保存的节点路由不会改变。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
    }))
  const viewNode = async (id: number) => {
    if (activeID === id && selected.length === 0) return
    if (!(await close())) return
    setActiveID(id)
    setSelected([])
    setRevision(0)
    setPreview(false)
  }
  const toggleNode = (id: number) => {
    const next = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]
    setSelected(next)
    if (next.length === 1) setActiveID(next[0])
  }
  const save = async () => {
    setBusy(true)
    setResults([])
    const out: typeof results = []
    for (const id of targets) {
      try {
        const binding =
          overview.data?.nodes.find((n) => n.id === id)?.routing?.binding ??
          (await getBinding('node', id))
        const impact = await saveBinding(templateBinding(binding, template.id, revision))
        out.push({
          id,
          changed: impact.changed_nodes,
          error: Object.values(impact.conflicts).join('；') || undefined,
        })
      } catch (e) {
        out.push({ id, error: (e as Error).message, changed: [] })
      }
      setResults([...out])
    }
    setSelected(out.filter((r) => r.error).map((r) => r.id))
    if (out.every((r) => !r.error)) setRevision(0)
    await invalidateRouting(qc)
    setBusy(false)
  }
  const changed = [...new Set(results.flatMap((r) => r.changed))]
  return (
    <>
      <Drawer
        open
        width="xl"
        saving={busy}
        dirty={dirty}
        onBeforeClose={close}
        onClose={onClose}
        title={`配置节点 · ${template.name}`}
        description="点击节点查看并配置单个节点；勾选用于批量配置。每个节点只使用一份路由模板，保存会替换原选择，保留节点覆盖。"
        footer={
          <>
            <Btn
              variant="secondary"
              disabled={busy}
              onClick={() =>
                void close().then((ok) => {
                  if (ok) onClose()
                })
              }
            >
              关闭
            </Btn>
            <Btn
              loading={busy}
              disabled={!targets.length || !revision || !!overview.error || !!revisions.error}
              onClick={() => void save()}
            >
              {selected.length > 1
                ? `批量保存（${selected.length}）`
                : selected.length
                  ? '保存绑定'
                  : '保存当前节点'}
            </Btn>
          </>
        }
      >
        {(overview.error || revisions.error) && (
          <div role="alert">
            数据加载失败。
            <Btn
              variant="secondary"
              onClick={() => {
                void overview.refetch()
                void revisions.refetch()
              }}
            >
              重试
            </Btn>
          </div>
        )}
        <fieldset
          disabled={busy || !!overview.error || !!revisions.error}
          className="grid gap-5 md:grid-cols-[280px_minmax(0,1fr)]"
        >
          <div className="space-y-3">
            <Field label="搜索节点" value={search} onChange={(e) => setSearch(e.target.value)} />
            <button
              className="text-xs text-[var(--accent)]"
              onClick={() =>
                setSelected(
                  rows.every((n) => selected.includes(n.id))
                    ? selected.filter((id) => !rows.some((n) => n.id === id))
                    : [...new Set([...selected, ...rows.map((n) => n.id)])],
                )
              }
            >
              全选 / 取消当前可见节点
            </button>
            <div className="max-h-[55vh] space-y-2 overflow-auto">
              {rows.map((n) => (
                <div
                  key={n.id}
                  className={`flex items-start gap-2 rounded border p-3 text-sm ${activeID === n.id ? 'border-[var(--accent)] bg-[var(--accent-soft)]' : 'border-[var(--border)]'}`}
                >
                  <input
                    className="mt-1"
                    type="checkbox"
                    aria-label={`选择 ${n.name}`}
                    checked={selected.includes(n.id)}
                    onChange={() => toggleNode(n.id)}
                  />
                  <button
                    type="button"
                    aria-label={`查看 ${n.name} 的路由配置`}
                    aria-pressed={activeID === n.id}
                    onClick={() => void viewNode(n.id)}
                    className="min-w-0 flex-1 cursor-pointer rounded text-left outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
                  >
                    {n.name}
                    <span className="mt-1 block text-xs text-soft">{n.ip}</span>
                    <span className="mt-1 block text-xs text-soft">
                      {routingLabel(n, overview.data?.templates ?? [])}
                    </span>
                  </button>
                </div>
              ))}
            </div>
          </div>
          <div className="space-y-4">
            {activeNode ? (
              <NodeRoutingPreview
                key={activeNode.id}
                node={activeNode}
                templates={overview.data?.templates ?? []}
                onPreview={() => setPreview(true)}
              />
            ) : (
              <p className="rounded-md bg-[var(--panel-muted)] p-4 text-sm text-soft">
                点击左侧节点查看已保存的路由配置；勾选多个节点可批量指定同一个模板版本。
              </p>
            )}
            <h4 className="text-sm font-semibold">
              {selected.length
                ? `为勾选的 ${selected.length} 个节点配置路由`
                : activeNode
                  ? `设置 ${activeNode.name} 的路由`
                  : '指定路由模板版本'}
            </h4>
            <SelectField
              label="为所选节点指定固定版本"
              value={revision}
              onChange={(v) => setRevision(Number(v))}
              options={[
                { value: 0, label: '请选择已发布版本' },
                ...(revisions.data ?? []).map((r) => ({
                  value: r.revision,
                  label: `v${r.revision}${readPolicy(r.payload).schema_version === 2 ? ' · 默认直连' : ' · 旧版 IPv4 兜底'}`,
                })),
              ]}
            />
            <p className="text-sm text-soft">
              保存将把目标节点原来的继承或指定模板替换为「{template.name}
              {revision ? ` v${revision}` : ''}」。
              原模板规则不会叠加；各节点的覆盖项、配置路径和订阅策略保持不变。
              {overview.data?.auto_sync
                ? '自动同步已开启，变化节点将自动同步。'
                : '保存后请回节点列表同步。'}
            </p>
            {targets.map((id) => {
              const n = overview.data?.nodes.find((n) => n.id === id)
              return (
                <div key={id} className="rounded border border-[var(--border)] p-3 text-xs">
                  <strong>{n?.name}</strong>
                  <p className="mt-1 text-soft">
                    {n && routingLabel(n, overview.data?.templates ?? [])} → {template.name}{' '}
                    {revision ? `v${revision}` : '未选择版本'}
                  </p>
                </div>
              )
            })}
            {results.map((r) => (
              <p
                role="status"
                key={r.id}
                className={`text-sm ${r.error ? 'text-[var(--danger)]' : 'text-soft'}`}
              >
                {overview.data?.nodes.find((n) => n.id === r.id)?.name}：
                {r.error ?? (r.changed.length ? '已保存，配置已变化' : '已保存，有效配置未变化')}
              </p>
            ))}
            {!!changed.length && (
              <Link
                to={nodesLink(changed)}
                className="inline-block text-sm text-[var(--accent)] underline"
              >
                查看受影响的 {changed.length} 个节点
              </Link>
            )}
          </div>
        </fieldset>
      </Drawer>
      {preview && activeNode && (
        <PreviewConfigModal
          key={activeNode.id}
          node={activeNode}
          onClose={() => setPreview(false)}
        />
      )}
    </>
  )
}
