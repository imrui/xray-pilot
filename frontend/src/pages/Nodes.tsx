import { Fragment, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FileCode, PencilLine, RefreshCw, Wifi } from 'lucide-react'
import { nodeApi } from '@/lib/api'
import type { Node, SyncStatus } from '@/types'
import { Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { BulkBar, FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { useConfirm } from '@/components/ui/ConfirmProvider'

const PAGE_SIZE = 20

const statusBadge: Record<SyncStatus, { label: string; variant: 'green' | 'yellow' | 'red' | 'gray' }> = {
  synced: { label: '已同步', variant: 'green' },
  drifted: { label: '配置漂移', variant: 'yellow' },
  failed: { label: '同步失败', variant: 'red' },
  pending: { label: '待同步', variant: 'gray' },
}

interface FormState {
  name: string
  region: string
  ip: string
  domain: string
  ssh_port: string
  ssh_user: string
  ssh_key_path: string
  remark: string
}

const emptyForm = (): FormState => ({
  name: '',
  region: '',
  ip: '',
  domain: '',
  ssh_port: '22',
  ssh_user: 'root',
  ssh_key_path: '',
  remark: '',
})

export default function Nodes() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [drawer, setDrawer] = useState<{ open: boolean; node?: Node }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [syncErr, setSyncErr] = useState('')
  const [syncOk, setSyncOk] = useState('')
  const syncOkTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)
  const [previewNode, setPreviewNode] = useState<Node | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'synced' | 'drifted' | 'failed' | 'pending'>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  useEffect(() => {
    if (!syncOk) return
    if (syncOkTimer.current) clearTimeout(syncOkTimer.current)
    syncOkTimer.current = setTimeout(() => setSyncOk(''), 3000)
    return () => {
      if (syncOkTimer.current) clearTimeout(syncOkTimer.current)
    }
  }, [syncOk])

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page],
    queryFn: () => nodeApi.list({ page, page_size: PAGE_SIZE }).then((r) => r.data.data!),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['nodes'] })

  const save = useMutation({
    mutationFn: () => {
      const payload = { ...form, ssh_port: Number(form.ssh_port) || 22 }
      return drawer.node ? nodeApi.update(drawer.node.id, payload) : nodeApi.create(payload)
    },
    onSuccess: () => {
      invalidate()
      closeDrawer()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const toggle = useMutation({ mutationFn: (id: number) => nodeApi.toggle(id), onSuccess: invalidate })
  const remove = useMutation({
    mutationFn: (id: number) => nodeApi.remove(id),
    onSuccess: invalidate,
    onError: (e: Error) => setSyncErr(e.message),
  })
  const sync = useMutation({
    mutationFn: (id: number) => nodeApi.sync(id),
    onSuccess: (res) => {
      setSyncErr('')
      setSyncOk(res.data.data?.message ?? '同步成功')
      invalidate()
    },
    onError: (e: Error) => setSyncErr(e.message),
  })
  const syncDrifted = useMutation({ mutationFn: () => nodeApi.syncDrifted(), onSuccess: invalidate })
  const testSSH = useMutation({
    mutationFn: (id: number) => nodeApi.testSSH(id),
    onSuccess: invalidate,
    onError: (e: Error) => setSyncErr(e.message),
  })

  const openCreate = () => {
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true })
  }

  const openEdit = (n: Node) => {
    const next = {
      name: n.name,
      region: n.region,
      ip: n.ip,
      domain: n.domain,
      ssh_port: String(n.ssh_port),
      ssh_user: n.ssh_user,
      ssh_key_path: n.ssh_key_path,
      remark: n.remark,
    }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true, node: n })
  }

  const closeDrawer = () => setDrawer({ open: false })
  const confirmCloseDrawer = async () => {
    if (!dirty) return true
    return confirm({
      title: '放弃未保存的节点修改？',
      description: '当前连接地址和 SSH 参数更改尚未保存，关闭后会丢失。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    })
  }

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const filteredNodes = (data?.list ?? []).filter((n) => {
    const keyword = search.trim().toLowerCase()
    const matchesKeyword =
      keyword === '' ||
      n.name.toLowerCase().includes(keyword) ||
      n.ip.toLowerCase().includes(keyword) ||
      n.domain.toLowerCase().includes(keyword) ||
      n.region.toLowerCase().includes(keyword)
    const matchesStatus = statusFilter === 'all' || n.sync_status === statusFilter
    return matchesKeyword && matchesStatus
  })
  const allVisibleSelected = filteredNodes.length > 0 && filteredNodes.every((n) => selectedIds.includes(n.id))
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm) && drawer.open

  const toggleSelect = (id: number) =>
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))

  const toggleSelectVisible = () =>
    setSelectedIds((prev) => {
      if (allVisibleSelected) return prev.filter((id) => !filteredNodes.some((n) => n.id === id))
      return Array.from(new Set([...prev, ...filteredNodes.map((n) => n.id)]))
    })

  const selectedNodes = filteredNodes.filter((n) => selectedIds.includes(n.id))

  const columns = [
    {
      key: 'select',
      label: (
        <input type="checkbox" checked={allVisibleSelected} onChange={toggleSelectVisible} aria-label="选择当前页可见节点" className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ) as unknown as string,
      render: (n: Node) => (
        <input type="checkbox" checked={selectedIds.includes(n.id)} onChange={() => toggleSelect(n.id)} aria-label={`选择节点 ${n.name}`} className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ),
    },
    {
      key: 'name',
      label: '节点',
      render: (n: Node) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{n.name}</span>
            {n.region && <span className="text-xs text-soft">{n.region}</span>}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {n.xray_version ? <Badge label={n.xray_version} variant={n.xray_active ? 'green' : 'gray'} /> : <span className="text-xs text-soft">版本未知</span>}
            <Badge label={(statusBadge[n.sync_status] ?? statusBadge.pending).label} variant={(statusBadge[n.sync_status] ?? statusBadge.pending).variant} />
            <span className="text-xs text-soft">{n.domain || n.ip}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'health',
      label: '健康',
      render: (n: Node) =>
        n.last_check_at ? <Badge label={n.last_check_ok ? `${n.last_latency_ms}ms` : '不可达'} variant={n.last_check_ok ? 'green' : 'red'} /> : <span className="text-xs text-soft">未检测</span>,
    },
    { key: 'active', label: '状态', render: (n: Node) => <Badge label={n.active ? '启用' : '禁用'} variant={n.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (n: Node) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => sync.mutate(n.id)}
            disabled={sync.isPending && sync.variables === n.id}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-[var(--accent)] transition hover:bg-[var(--panel)] disabled:opacity-50"
          >
            <RefreshCw className={`h-3.5 w-3.5${sync.isPending && sync.variables === n.id ? ' animate-spin' : ''}`} />
            同步
          </button>
          <button
            onClick={() => openEdit(n)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-soft transition hover:bg-[var(--panel)] hover:text-[var(--text)]"
          >
            <PencilLine className="h-3.5 w-3.5" />
            编辑
          </button>
          <ActionMenu
            items={[
              {
                label: '测试 SSH',
                onSelect: () => testSSH.mutate(n.id),
                disabled: testSSH.isPending && testSSH.variables === n.id,
              },
              {
                label: '预览生成配置',
                onSelect: () => setPreviewNode(n),
              },
              {
                label: expanded === n.id ? '收起详情' : '展开详情',
                onSelect: () => setExpanded(expanded === n.id ? null : n.id),
              },
              {
                label: n.active ? '禁用节点' : '启用节点',
                onSelect: () => toggle.mutate(n.id),
              },
              {
                label: '删除节点',
                danger: true,
                onSelect: async () => {
                  const ok = await confirm({
                    title: `删除节点「${n.name}」？`,
                    description: '此操作不可撤销，节点配置和关联状态会被永久移除。',
                    confirmText: '删除节点',
                    cancelText: '取消',
                    tone: 'danger',
                  })
                  if (ok) remove.mutate(n.id)
                },
              },
            ]}
          />
        </div>
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="节点管理"
        description="把高频节点动作集中在同步和编辑，扩展操作收纳进菜单，减少表格里大段文字按钮。"
        actions={
          <>
            <Btn variant="secondary" loading={syncDrifted.isPending} onClick={() => syncDrifted.mutate()}>
              <RefreshCw className="h-4 w-4" />
              同步漂移节点
            </Btn>
            <Btn onClick={openCreate}>新增节点</Btn>
          </>
        }
        stats={[
          { label: '总节点数', value: data?.total ?? 0 },
          { label: '分页尺寸', value: PAGE_SIZE },
          { label: '展开节点', value: expanded ?? '无' },
        ]}
      />

      {syncOk && (
        <div className="panel rounded-[24px] border-emerald-500/20 bg-emerald-500/8 p-4 text-sm text-emerald-500">
          <div className="flex items-center justify-between gap-3">
            <span>{syncOk}</span>
            <button onClick={() => setSyncOk('')} className="text-emerald-400 transition hover:text-emerald-300">关闭</button>
          </div>
        </div>
      )}

      {syncErr && (
        <div className="panel rounded-[24px] border-rose-500/20 bg-rose-500/8 p-4 text-sm text-rose-500">
          <div className="flex items-center justify-between gap-3">
            <span>{syncErr}</span>
            <button onClick={() => setSyncErr('')} className="text-rose-400 transition hover:text-rose-300">关闭</button>
          </div>
        </div>
      )}

      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索节点名、IP、域名或地区"
        onSearchChange={setSearch}
        filters={
          <>
            <FilterChip active={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>全部</FilterChip>
            <FilterChip active={statusFilter === 'synced'} onClick={() => setStatusFilter('synced')}>已同步</FilterChip>
            <FilterChip active={statusFilter === 'drifted'} onClick={() => setStatusFilter('drifted')}>漂移</FilterChip>
            <FilterChip active={statusFilter === 'failed'} onClick={() => setStatusFilter('failed')}>失败</FilterChip>
            <FilterChip active={statusFilter === 'pending'} onClick={() => setStatusFilter('pending')}>待同步</FilterChip>
          </>
        }
        meta={`当前页匹配 ${filteredNodes.length} / ${(data?.list ?? []).length} 条`}
        bulkBar={
          selectedNodes.length > 0 ? (
            <BulkBar>
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="text-sm text-soft">已选择 <span className="font-semibold text-[var(--text)]">{selectedNodes.length}</span> 个节点</div>
                <div className="flex flex-wrap gap-2">
                  <Btn variant="secondary" onClick={async () => {
                    await Promise.all(selectedNodes.map((n) => nodeApi.sync(n.id)))
                    setSelectedIds([])
                    invalidate()
                  }}>批量同步</Btn>
                  <Btn variant="secondary" onClick={async () => {
                    await Promise.all(selectedNodes.map((n) => nodeApi.testSSH(n.id)))
                    setSelectedIds([])
                    invalidate()
                  }}>批量测试 SSH</Btn>
                  <Btn variant="secondary" onClick={async () => {
                    await Promise.all(selectedNodes.map((n) => nodeApi.toggle(n.id)))
                    setSelectedIds([])
                    invalidate()
                  }}>批量切换启用状态</Btn>
                  <Btn
                    variant="danger"
                    onClick={async () => {
                      const ok = await confirm({
                        title: `删除已选中的 ${selectedNodes.length} 个节点？`,
                        description: '批量删除不可撤销，建议先确认筛选条件和勾选范围。',
                        confirmText: '批量删除',
                        cancelText: '取消',
                        tone: 'danger',
                      })
                      if (!ok) return
                      await Promise.all(selectedNodes.map((n) => nodeApi.remove(n.id)))
                      setSelectedIds([])
                      invalidate()
                    }}
                  >
                    批量删除
                  </Btn>
                </div>
              </div>
            </BulkBar>
          ) : null
        }
      />

      <SurfaceCard className="overflow-hidden p-4">
        <div className="hidden overflow-x-auto rounded-[24px] border border-[var(--border)] bg-[var(--panel-muted)] md:block">
          <table className="w-full text-left text-sm">
            <thead className="border-b border-[var(--border)] bg-white/30 dark:bg-white/2">
              <tr>
                {columns.map((col) => (
                  <th key={col.key} className="px-4 py-4 text-[11px] font-semibold uppercase tracking-[0.2em] text-faint">
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr><td colSpan={columns.length} className="px-4 py-10 text-center text-soft">加载中…</td></tr>
              ) : filteredNodes.length === 0 ? (
                <tr><td colSpan={columns.length} className="px-4 py-10 text-center text-soft">暂无数据</td></tr>
              ) : (
                filteredNodes.map((node) => (
                  <Fragment key={node.id}>
                    <tr className="transition-colors hover:bg-white/30 dark:hover:bg-white/2">
                      {columns.map((col) => (
                        <td key={col.key} className="px-4 py-4 align-top text-[var(--text)]">
                          {col.render ? col.render(node) : String((node as unknown as Record<string, unknown>)[col.key] ?? '')}
                        </td>
                      ))}
                    </tr>
                    {expanded === node.id && (
                      <tr className="bg-white/30 dark:bg-white/2">
                        <td colSpan={columns.length} className="px-4 py-4">
                          <div className="grid gap-3 text-xs text-soft md:grid-cols-3">
                            <div><span className="font-semibold text-[var(--text)]">IP：</span>{node.ip}</div>
                            <div><span className="font-semibold text-[var(--text)]">Domain：</span>{node.domain || '—'}</div>
                            <div><span className="font-semibold text-[var(--text)]">SSH：</span>{node.ssh_user}@{node.ip}:{node.ssh_port}</div>
                            <div><span className="font-semibold text-[var(--text)]">SSH Key：</span>{node.ssh_key_path || '默认'}</div>
                            <div><span className="font-semibold text-[var(--text)]">最后同步：</span>{node.last_sync_at ? new Date(node.last_sync_at).toLocaleString() : '—'}</div>
                            <div><span className="font-semibold text-[var(--text)]">最后检测：</span>{node.last_check_at ? new Date(node.last_check_at).toLocaleString() : '—'}</div>
                            {node.remark && <div className="md:col-span-3"><span className="font-semibold text-[var(--text)]">备注：</span>{node.remark}</div>}
                          </div>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))
              )}
            </tbody>
          </table>
        </div>

        <div className="space-y-3 md:hidden">
          {(data?.list ?? []).map((node) => (
            <div key={node.id} className="panel-muted rounded-[22px] p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-base font-semibold">{node.name}</div>
                  <div className="mt-1 text-xs text-soft">{node.region || node.domain || node.ip}</div>
                </div>
                <Badge label={node.active ? '启用' : '禁用'} variant={node.active ? 'green' : 'gray'} />
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                <button onClick={() => sync.mutate(node.id)} className="text-xs text-[var(--accent)]">同步</button>
                <button onClick={() => openEdit(node)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
                <button onClick={() => setPreviewNode(node)} className="text-xs text-soft transition hover:text-[var(--text)]">配置</button>
              </div>
            </div>
          ))}
        </div>

        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.node ? `编辑节点 · ${drawer.node.name}` : '新增节点'}
        description="连接信息和 SSH 参数放入右侧抽屉，避免大型表单打断表格浏览。"
        width="lg"
        dirty={dirty}
        saving={save.isPending}
        onBeforeClose={confirmCloseDrawer}
        footer={
          <>
            <Btn variant="secondary" onClick={closeDrawer}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="space-y-4">
          <FieldGroup title="节点标识" description="基础信息用于列表展示和节点识别。">
            <Field label="节点名 *" value={form.name} onChange={f('name')} />
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="地区" value={form.region} onChange={f('region')} placeholder="如：香港" />
              <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="例如：高可用入口节点" />
            </div>
          </FieldGroup>

          <FieldGroup title="连接地址" description="域名为空时使用 IP 直连。">
            <Field label="IP *" value={form.ip} onChange={f('ip')} placeholder="服务器 IP" />
            <Field label="连接域名" value={form.domain} onChange={f('domain')} placeholder="CDN/中转域名（留空用 IP）" />
          </FieldGroup>

          <FieldGroup title="SSH 参数" description="用于后续同步、健康检查和配置下发。">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
              <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
            </div>
            <Field label="SSH 密钥路径" value={form.ssh_key_path} onChange={f('ssh_key_path')} placeholder="/root/.ssh/id_ed25519" />
          </FieldGroup>

          {drawer.node && (
            <FieldGroup title="快捷动作" description="编辑抽屉内直接做连通性和配置确认。">
              <div className="flex flex-wrap gap-2">
                <Btn
                  variant="secondary"
                  loading={testSSH.isPending && testSSH.variables === drawer.node.id}
                  onClick={() => testSSH.mutate(drawer.node!.id)}
                >
                  <Wifi className="h-4 w-4" />
                  测试 SSH
                </Btn>
                <Btn variant="secondary" onClick={() => setPreviewNode(drawer.node!)}><FileCode className="h-4 w-4" />预览配置</Btn>
              </div>
            </FieldGroup>
          )}

          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Drawer>

      {previewNode && <PreviewConfigModal node={previewNode} onClose={() => setPreviewNode(null)} />}
    </PageShell>
  )
}

function PreviewConfigModal({ node, onClose }: { node: Node; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['previewConfig', node.id],
    queryFn: () => nodeApi.previewConfig(node.id).then((r) => r.data.data!),
    retry: false,
  })

  return (
    <Modal
      open
      onClose={onClose}
      title={`预览生成配置 · ${node.name}`}
      size="lg"
      footer={<Btn variant="secondary" onClick={onClose}>关闭</Btn>}
    >
      <div className="space-y-4">
        <p className="text-xs text-soft">仅预览，不会同步至节点；`private_key` 已脱敏。</p>
        {isLoading && <p className="py-8 text-center text-sm text-soft">生成中…</p>}
        {error && (
          <div className="rounded-2xl border border-rose-500/20 bg-rose-500/8 p-4">
            <p className="text-sm font-medium text-rose-500">生成失败</p>
            <p className="mt-1 text-xs text-rose-400">{(error as Error).message}</p>
          </div>
        )}
        {data && (
          <>
            {(data.warnings ?? []).length > 0 && (
              <div className="rounded-2xl border border-amber-500/20 bg-amber-500/8 p-4">
                <p className="mb-2 text-xs font-medium text-amber-400">部分协议生成失败，不影响其他协议：</p>
                {data.warnings.map((w, i) => (
                  <p key={i} className="text-xs text-amber-300">{w}</p>
                ))}
              </div>
            )}
            <pre className="overflow-auto rounded-[24px] border border-[var(--border)] bg-slate-950 p-4 font-mono text-xs text-slate-100 whitespace-pre-wrap">
              {data.config}
            </pre>
          </>
        )}
      </div>
    </Modal>
  )
}
