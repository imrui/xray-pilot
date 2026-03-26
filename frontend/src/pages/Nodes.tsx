import { Fragment, useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, FileCode, RefreshCw, Wifi } from 'lucide-react'
import { nodeApi } from '@/lib/api'
import type { Node, SyncStatus } from '@/types'
import { Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

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
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; node?: Node }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [syncErr, setSyncErr] = useState('')
  const [syncOk, setSyncOk] = useState('')
  const syncOkTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)
  const [previewNode, setPreviewNode] = useState<Node | null>(null)

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
      return modal.node ? nodeApi.update(modal.node.id, payload) : nodeApi.create(payload)
    },
    onSuccess: () => {
      invalidate()
      closeModal()
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
    setForm(emptyForm())
    setErr('')
    setModal({ open: true })
  }

  const openEdit = (n: Node) => {
    setForm({
      name: n.name,
      region: n.region,
      ip: n.ip,
      domain: n.domain,
      ssh_port: String(n.ssh_port),
      ssh_user: n.ssh_user,
      ssh_key_path: n.ssh_key_path,
      remark: n.remark,
    })
    setErr('')
    setModal({ open: true, node: n })
  }

  const closeModal = () => setModal({ open: false })

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const columns = [
    { key: 'id', label: 'ID' },
    {
      key: 'name',
      label: '节点',
      render: (n: Node) => (
        <div>
          <span className="font-semibold">{n.name}</span>
          {n.region && <span className="ml-2 text-xs text-soft">{n.region}</span>}
          <p className="mt-1 text-xs text-soft">{n.domain || n.ip}</p>
        </div>
      ),
    },
    {
      key: 'xray_active',
      label: 'Xray',
      render: (n: Node) => (n.xray_version ? <Badge label={n.xray_version} variant={n.xray_active ? 'green' : 'gray'} /> : <span className="text-xs text-soft">未知</span>),
    },
    {
      key: 'sync_status',
      label: '同步状态',
      render: (n: Node) => {
        const s = statusBadge[n.sync_status] ?? statusBadge.pending
        return <Badge label={s.label} variant={s.variant} />
      },
    },
    {
      key: 'last_check_ok',
      label: '健康',
      render: (n: Node) =>
        n.last_check_at ? <Badge label={n.last_check_ok ? `${n.last_latency_ms}ms` : '不可达'} variant={n.last_check_ok ? 'green' : 'red'} /> : <span className="text-xs text-soft">未检测</span>,
    },
    { key: 'active', label: '状态', render: (n: Node) => <Badge label={n.active ? '启用' : '禁用'} variant={n.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (n: Node) => (
        <div className="flex flex-wrap items-center gap-2">
          <button onClick={() => openEdit(n)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
          <button
            onClick={() => sync.mutate(n.id)}
            disabled={sync.isPending && sync.variables === n.id}
            className="flex items-center gap-1 text-xs text-[var(--accent)] transition hover:brightness-110 disabled:opacity-50"
          >
            <RefreshCw className={`h-3 w-3${sync.isPending && sync.variables === n.id ? ' animate-spin' : ''}`} />
            同步
          </button>
          <button
            onClick={() => testSSH.mutate(n.id)}
            disabled={testSSH.isPending && testSSH.variables === n.id}
            className="flex items-center gap-1 text-xs text-cyan-500 transition hover:brightness-110 disabled:opacity-50"
          >
            <Wifi className={`h-3 w-3${testSSH.isPending && testSSH.variables === n.id ? ' animate-pulse' : ''}`} />
            SSH
          </button>
          <button onClick={() => toggle.mutate(n.id)} className="text-xs text-soft transition hover:text-[var(--text)]">
            {n.active ? '禁用' : '启用'}
          </button>
          <button
            onClick={() => {
              if (confirm(`删除节点「${n.name}」？`)) remove.mutate(n.id)
            }}
            className="text-xs text-rose-500 transition hover:brightness-110"
          >
            删除
          </button>
          <button onClick={() => setPreviewNode(n)} className="flex items-center gap-1 text-xs text-soft transition hover:text-[var(--text)]" title="预览生成配置">
            <FileCode className="h-3.5 w-3.5" />
            配置
          </button>
          <button onClick={() => setExpanded(expanded === n.id ? null : n.id)} className="text-xs text-soft transition hover:text-[var(--text)]">
            {expanded === n.id ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </button>
        </div>
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="节点管理"
        description="统一管理 Xray 节点、SSH 连接信息与同步状态，优先突出健康检测和漂移恢复。"
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
              ) : (data?.list ?? []).length === 0 ? (
                <tr><td colSpan={columns.length} className="px-4 py-10 text-center text-soft">暂无数据</td></tr>
              ) : (
                (data?.list ?? []).map((node) => (
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
                <button onClick={() => openEdit(node)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
                <button onClick={() => sync.mutate(node.id)} className="text-xs text-[var(--accent)]">同步</button>
                <button onClick={() => setPreviewNode(node)} className="text-xs text-soft transition hover:text-[var(--text)]">配置</button>
              </div>
            </div>
          ))}
        </div>

        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Modal
        open={modal.open}
        onClose={closeModal}
        title={modal.node ? '编辑节点' : '新增节点'}
        size="lg"
        footer={
          <>
            <Btn variant="secondary" onClick={closeModal}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="grid gap-4 md:grid-cols-2">
          <Field label="节点名 *" value={form.name} onChange={f('name')} className="md:col-span-2" />
          <Field label="地区" value={form.region} onChange={f('region')} placeholder="如：香港" />
          <Field label="IP *" value={form.ip} onChange={f('ip')} placeholder="服务器 IP" />
          <Field label="连接域名" value={form.domain} onChange={f('domain')} placeholder="CDN/中转域名（留空用 IP）" className="md:col-span-2" />
          <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
          <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
          <Field label="SSH 密钥路径" value={form.ssh_key_path} onChange={f('ssh_key_path')} placeholder="/root/.ssh/id_ed25519" className="md:col-span-2" />
          <Field label="备注" value={form.remark} onChange={f('remark')} className="md:col-span-2" />
          {err && <p className="text-sm text-rose-500 md:col-span-2">{err}</p>}
        </div>
      </Modal>

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
