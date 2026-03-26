import { useState, useEffect, useRef } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Wifi, ChevronDown, ChevronUp, FileCode } from 'lucide-react'
import { nodeApi } from '@/lib/api'
import type { Node, SyncStatus } from '@/types'
import { Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn } from '@/components/ui/Form'

const PAGE_SIZE = 20

const statusBadge: Record<SyncStatus, { label: string; variant: 'green' | 'yellow' | 'red' | 'gray' }> = {
  synced:  { label: '已同步', variant: 'green' },
  drifted: { label: '配置漂移', variant: 'yellow' },
  failed:  { label: '同步失败', variant: 'red' },
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
  name: '', region: '', ip: '', domain: '',
  ssh_port: '22', ssh_user: 'root', ssh_key_path: '', remark: '',
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

  // 成功提示 3 秒后自动消失
  useEffect(() => {
    if (!syncOk) return
    if (syncOkTimer.current) clearTimeout(syncOkTimer.current)
    syncOkTimer.current = setTimeout(() => setSyncOk(''), 3000)
    return () => { if (syncOkTimer.current) clearTimeout(syncOkTimer.current) }
  }, [syncOk])

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page],
    queryFn: () => nodeApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['nodes'] })

  const save = useMutation({
    mutationFn: () => {
      const payload = {
        ...form,
        ssh_port: Number(form.ssh_port) || 22,
      }
      return modal.node
        ? nodeApi.update(modal.node.id, payload)
        : nodeApi.create(payload)
    },
    onSuccess: () => { invalidate(); closeModal() },
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

  const openCreate = () => { setForm(emptyForm()); setErr(''); setModal({ open: true }) }
  const openEdit = (n: Node) => {
    setForm({
      name: n.name, region: n.region, ip: n.ip, domain: n.domain,
      ssh_port: String(n.ssh_port), ssh_user: n.ssh_user,
      ssh_key_path: n.ssh_key_path, remark: n.remark,
    })
    setErr('')
    setModal({ open: true, node: n })
  }
  const closeModal = () => setModal({ open: false })

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [k]: e.target.value }))

  const columns = [
    { key: 'id', label: 'ID' },
    {
      key: 'name', label: '节点',
      render: (n: Node) => (
        <div>
          <span className="font-medium text-slate-900">{n.name}</span>
          {n.region && <span className="ml-1.5 text-xs text-slate-400">{n.region}</span>}
          <p className="text-xs text-slate-400 mt-0.5">{n.domain || n.ip}</p>
        </div>
      ),
    },
    {
      key: 'xray_active', label: 'Xray',
      render: (n: Node) => n.xray_version
        ? <Badge label={n.xray_version} variant={n.xray_active ? 'green' : 'gray'} />
        : <span className="text-xs text-slate-400">未知</span>,
    },
    {
      key: 'sync_status', label: '同步状态',
      render: (n: Node) => {
        const s = statusBadge[n.sync_status] ?? statusBadge.pending
        return <Badge label={s.label} variant={s.variant} />
      },
    },
    {
      key: 'last_check_ok', label: '健康',
      render: (n: Node) => n.last_check_at
        ? <Badge label={n.last_check_ok ? `${n.last_latency_ms}ms` : '不可达'} variant={n.last_check_ok ? 'green' : 'red'} />
        : <span className="text-xs text-slate-400">未检测</span>,
    },
    { key: 'active', label: '状态', render: (n: Node) => <Badge label={n.active ? '启用' : '禁用'} variant={n.active ? 'green' : 'gray'} /> },
    {
      key: 'actions', label: '操作',
      render: (n: Node) => (
        <div className="flex gap-2 flex-wrap items-center">
          <button onClick={() => openEdit(n)} className="text-xs text-slate-500 hover:text-slate-900">编辑</button>
          <button
            onClick={() => sync.mutate(n.id)}
            disabled={sync.isPending && sync.variables === n.id}
            className="text-xs text-indigo-500 hover:text-indigo-700 flex items-center gap-0.5 disabled:opacity-50"
          >
            <RefreshCw className={`h-3 w-3${sync.isPending && sync.variables === n.id ? ' animate-spin' : ''}`} />同步
          </button>
          <button
            onClick={() => testSSH.mutate(n.id)}
            disabled={testSSH.isPending && testSSH.variables === n.id}
            className="text-xs text-cyan-500 hover:text-cyan-700 flex items-center gap-0.5 disabled:opacity-50"
          >
            <Wifi className={`h-3 w-3${testSSH.isPending && testSSH.variables === n.id ? ' animate-pulse' : ''}`} />SSH
          </button>
          <button
            onClick={() => toggle.mutate(n.id)}
            className="text-xs text-slate-500 hover:text-slate-800"
          >
            {n.active ? '禁用' : '启用'}
          </button>
          <button
            onClick={() => { if (confirm(`删除节点「${n.name}」？`)) remove.mutate(n.id) }}
            className="text-xs text-red-500 hover:text-red-700"
          >删除</button>
          <button
            onClick={() => setPreviewNode(n)}
            className="text-xs text-slate-400 hover:text-slate-600 flex items-center gap-0.5"
            title="预览生成配置"
          >
            <FileCode className="h-3.5 w-3.5" />
          </button>
          <button
            onClick={() => setExpanded(expanded === n.id ? null : n.id)}
            className="text-xs text-slate-400 hover:text-slate-600"
          >
            {expanded === n.id ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">节点管理</h1>
          <p className="text-sm text-slate-500 mt-0.5">管理 Xray 节点 · 共 {data?.total ?? 0} 个</p>
        </div>
        <div className="flex gap-2">
          <Btn variant="secondary" loading={syncDrifted.isPending} onClick={() => syncDrifted.mutate()}>
            <RefreshCw className="h-3.5 w-3.5" />同步漂移节点
          </Btn>
          <Btn onClick={openCreate}>新增节点</Btn>
        </div>
      </div>

      {syncOk && (
        <div className="bg-green-50 border border-green-200 text-green-700 text-sm px-4 py-2 rounded-lg flex items-center justify-between">
          <span>{syncOk}</span>
          <button onClick={() => setSyncOk('')} className="ml-3 text-green-400 hover:text-green-600 text-base leading-none">✕</button>
        </div>
      )}
      {syncErr && (
        <div className="bg-red-50 border border-red-200 text-red-600 text-sm px-4 py-2 rounded-lg flex items-center justify-between">
          <span>{syncErr}</span>
          <button onClick={() => setSyncErr('')} className="ml-3 text-red-400 hover:text-red-600 text-base leading-none">✕</button>
        </div>
      )}

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 border-b border-slate-200">
              <tr>
                {columns.map(col => (
                  <th key={col.key} className="px-4 py-3 text-left text-xs font-medium text-slate-500 uppercase tracking-wide">
                    {col.label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {isLoading ? (
                <tr><td colSpan={columns.length} className="px-4 py-8 text-center text-slate-400 text-sm">加载中…</td></tr>
              ) : (data?.list ?? []).length === 0 ? (
                <tr><td colSpan={columns.length} className="px-4 py-8 text-center text-slate-400 text-sm">暂无数据</td></tr>
              ) : (
                (data?.list ?? []).map((node) => (
                  <>
                    <tr key={node.id} className="hover:bg-slate-50 transition-colors">
                      {columns.map(col => (
                        <td key={col.key} className="px-4 py-3 text-slate-700">
                          {col.render ? col.render(node) : String((node as unknown as Record<string, unknown>)[col.key] ?? '')}
                        </td>
                      ))}
                    </tr>
                    {/* 展开行：显示节点详细信息 */}
                    {expanded === node.id && (
                      <tr key={`${node.id}-expand`} className="bg-slate-50">
                        <td colSpan={columns.length} className="px-4 py-3">
                          <div className="grid grid-cols-3 gap-3 text-xs text-slate-600">
                            <div><span className="font-medium text-slate-500">IP：</span>{node.ip}</div>
                            <div><span className="font-medium text-slate-500">Domain：</span>{node.domain || '—'}</div>
                            <div><span className="font-medium text-slate-500">SSH：</span>{node.ssh_user}@{node.ip}:{node.ssh_port}</div>
                            <div><span className="font-medium text-slate-500">SSH Key：</span>{node.ssh_key_path || '默认'}</div>
                            <div><span className="font-medium text-slate-500">最后同步：</span>{node.last_sync_at ? new Date(node.last_sync_at).toLocaleString() : '—'}</div>
                            <div><span className="font-medium text-slate-500">最后检测：</span>{node.last_check_at ? new Date(node.last_check_at).toLocaleString() : '—'}</div>
                            {node.remark && <div className="col-span-3"><span className="font-medium text-slate-500">备注：</span>{node.remark}</div>}
                          </div>
                        </td>
                      </tr>
                    )}
                  </>
                ))
              )}
            </tbody>
          </table>
        </div>
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </div>

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
        <div className="grid grid-cols-2 gap-3">
          <Field label="节点名 *" value={form.name} onChange={f('name')} className="col-span-2" />
          <Field label="地区" value={form.region} onChange={f('region')} placeholder="如：香港" />
          <Field label="IP *" value={form.ip} onChange={f('ip')} placeholder="服务器 IP" />
          <Field label="连接域名" value={form.domain} onChange={f('domain')} placeholder="CDN/中转域名（留空用 IP）" className="col-span-2" />
          <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
          <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
          <Field label="SSH 密钥路径" value={form.ssh_key_path} onChange={f('ssh_key_path')} placeholder="/root/.ssh/id_ed25519" className="col-span-2" />
          <Field label="备注" value={form.remark} onChange={f('remark')} className="col-span-2" />
          {err && <p className="col-span-2 text-sm text-red-500">{err}</p>}
        </div>
      </Modal>

      {/* 预览配置弹窗 */}
      {previewNode && (
        <PreviewConfigModal node={previewNode} onClose={() => setPreviewNode(null)} />
      )}
    </div>
  )
}

// PreviewConfigModal 预览节点生成的 Xray 配置
function PreviewConfigModal({ node, onClose }: { node: Node; onClose: () => void }) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['previewConfig', node.id],
    queryFn: () => nodeApi.previewConfig(node.id).then(r => r.data.data!),
    retry: false,
  })

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-3xl max-h-[85vh] flex flex-col">
        <div className="flex items-center justify-between px-6 py-4 border-b border-gray-200 shrink-0">
          <div>
            <h3 className="text-base font-semibold text-gray-900">预览生成配置 · {node.name}</h3>
            <p className="text-xs text-slate-400 mt-0.5">private_key 已脱敏 · 此配置将被同步至节点</p>
          </div>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600 text-lg leading-none">✕</button>
        </div>
        <div className="flex-1 overflow-auto p-4">
          {isLoading && <p className="text-sm text-slate-400 text-center py-8">生成中…</p>}
          {error && (
            <div className="bg-red-50 border border-red-200 rounded-lg p-4">
              <p className="text-sm text-red-600 font-medium">生成失败</p>
              <p className="text-xs text-red-500 mt-1">{(error as Error).message}</p>
            </div>
          )}
          {data && (
            <>
              {(data.warnings ?? []).length > 0 && (
                <div className="mb-3 bg-yellow-50 border border-yellow-200 rounded-lg p-3">
                  <p className="text-xs font-medium text-yellow-700 mb-1">部分协议生成失败（不影响其他协议）：</p>
                  {data.warnings.map((w, i) => (
                    <p key={i} className="text-xs text-yellow-600">• {w}</p>
                  ))}
                </div>
              )}
              <pre className="text-xs font-mono bg-slate-950 text-slate-200 rounded-lg p-4 overflow-auto whitespace-pre">
                {data.config}
              </pre>
            </>
          )}
        </div>
        <div className="px-6 py-3 border-t border-gray-200 shrink-0 flex justify-end">
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
        </div>
      </div>
    </div>
  )
}
