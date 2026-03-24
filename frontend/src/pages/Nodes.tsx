import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCw, Wifi } from 'lucide-react'
import { nodeApi } from '@/lib/api'
import type { Node, SyncStatus } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
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
  name: string; region: string; ip: string; port: string
  public_key: string; short_id: string; sni: string
  ssh_port: string; ssh_user: string; ssh_key_path: string; remark: string
}

const emptyForm = (): FormState => ({
  name: '', region: '', ip: '', port: '443',
  public_key: '', short_id: '', sni: '',
  ssh_port: '22', ssh_user: 'root', ssh_key_path: '', remark: '',
})

export default function Nodes() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; node?: Node }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page],
    queryFn: () => nodeApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['nodes'] })

  const save = useMutation({
    mutationFn: () => {
      const payload = {
        ...form,
        port: Number(form.port) || 443,
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
  const remove = useMutation({ mutationFn: (id: number) => nodeApi.remove(id), onSuccess: invalidate })
  const sync = useMutation({ mutationFn: (id: number) => nodeApi.sync(id), onSuccess: invalidate })
  const syncDrifted = useMutation({ mutationFn: () => nodeApi.syncDrifted(), onSuccess: invalidate })
  const testSSH = useMutation({ mutationFn: (id: number) => nodeApi.testSSH(id), onSuccess: invalidate })

  const keygen = useMutation({
    mutationFn: () => nodeApi.keygen(),
    onSuccess: (res) => {
      const kp = res.data.data
      if (kp) setForm(p => ({ ...p, public_key: kp.public_key }))
    },
  })

  const openCreate = () => { setForm(emptyForm()); setErr(''); setModal({ open: true }) }
  const openEdit = (n: Node) => {
    setForm({
      name: n.name, region: n.region, ip: n.ip, port: String(n.port),
      public_key: n.public_key, short_id: n.short_id, sni: n.sni,
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
          <span className="font-medium">{n.name}</span>
          {n.region && <span className="ml-1.5 text-xs text-gray-400">{n.region}</span>}
        </div>
      ),
    },
    { key: 'ip', label: '地址', render: (n: Node) => `${n.ip}:${n.port}` },
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
        : <span className="text-xs text-gray-400">未检测</span>,
    },
    { key: 'active', label: '状态', render: (n: Node) => <Badge label={n.active ? '启用' : '禁用'} variant={n.active ? 'green' : 'gray'} /> },
    {
      key: 'actions', label: '操作',
      render: (n: Node) => (
        <div className="flex gap-2 flex-wrap">
          <button onClick={() => openEdit(n)} className="text-xs text-gray-600 hover:text-gray-900">编辑</button>
          <button onClick={() => sync.mutate(n.id)} className="text-xs text-blue-500 hover:text-blue-700 flex items-center gap-0.5">
            <RefreshCw className="h-3 w-3" />同步
          </button>
          <button onClick={() => testSSH.mutate(n.id)} className="text-xs text-indigo-500 hover:text-indigo-700 flex items-center gap-0.5">
            <Wifi className="h-3 w-3" />SSH
          </button>
          <button onClick={() => toggle.mutate(n.id)} className="text-xs text-amber-600 hover:text-amber-800">
            {n.active ? '禁用' : '启用'}
          </button>
          <button onClick={() => { if (confirm(`删除节点「${n.name}」？`)) remove.mutate(n.id) }}
            className="text-xs text-red-500 hover:text-red-700">删除</button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">节点管理</h2>
          <p className="text-sm text-gray-500 mt-0.5">管理 Xray Reality 节点 · 共 {data?.total ?? 0} 个</p>
        </div>
        <div className="flex gap-2">
          <Btn variant="secondary" loading={syncDrifted.isPending} onClick={() => syncDrifted.mutate()}>
            <RefreshCw className="h-3.5 w-3.5" />同步漂移节点
          </Btn>
          <Btn onClick={openCreate}>新增节点</Btn>
        </div>
      </div>

      {/* 图例 */}
      <div className="flex gap-2 mb-4">
        {(Object.entries(statusBadge) as [SyncStatus, typeof statusBadge[SyncStatus]][]).map(([, s]) => (
          <Badge key={s.label} label={s.label} variant={s.variant} />
        ))}
      </div>

      <div className="bg-white rounded-lg border border-gray-200">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
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
          <Field label="IP *" value={form.ip} onChange={f('ip')} />
          <Field label="端口" value={form.port} onChange={f('port')} type="number" />
          <Field label="SNI" value={form.sni} onChange={f('sni')} placeholder="www.microsoft.com" />
          <div className="col-span-2 flex gap-2 items-end">
            <Field label="PublicKey" value={form.public_key} onChange={f('public_key')} className="flex-1" />
            <Btn variant="secondary" loading={keygen.isPending} onClick={() => keygen.mutate()} className="shrink-0 mb-0">
              生成密钥对
            </Btn>
          </div>
          <Field label="ShortID" value={form.short_id} onChange={f('short_id')} />
          <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
          <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
          <Field label="SSH 密钥路径" value={form.ssh_key_path} onChange={f('ssh_key_path')} placeholder="/root/.ssh/id_ed25519" className="col-span-2" />
          <Field label="备注" value={form.remark} onChange={f('remark')} className="col-span-2" />
          {err && <p className="col-span-2 text-sm text-red-500">{err}</p>}
        </div>
      </Modal>
    </div>
  )
}
