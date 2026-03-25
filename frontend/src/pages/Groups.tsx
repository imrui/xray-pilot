import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { groupApi, nodeApi } from '@/lib/api'
import type { Group } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn } from '@/components/ui/Form'

const PAGE_SIZE = 20

interface FormState {
  name: string
  description: string
  node_ids: number[]
}

const emptyForm = (): FormState => ({ name: '', description: '', node_ids: [] })

export default function Groups() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; group?: Group }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')

  const { data, isLoading } = useQuery({
    queryKey: ['groups', page],
    queryFn: () => groupApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
  })

  // 所有节点列表（用于关联复选框）
  const { data: allNodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 200 }).then(r => r.data.data?.list ?? []),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['groups'] })

  const save = useMutation({
    mutationFn: () =>
      modal.group
        ? groupApi.update(modal.group.id, form)
        : groupApi.create(form),
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({
    mutationFn: (id: number) => groupApi.remove(id),
    onSuccess: invalidate,
  })

  const openCreate = () => { setForm(emptyForm()); setErr(''); setModal({ open: true }) }
  const openEdit = (g: Group) => {
    setForm({ name: g.name, description: g.description, node_ids: g.node_ids ?? [] })
    setErr('')
    setModal({ open: true, group: g })
  }
  const closeModal = () => setModal({ open: false })

  const f = (k: 'name' | 'description') => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [k]: e.target.value }))

  const toggleNode = (nodeId: number) =>
    setForm(prev => ({
      ...prev,
      node_ids: prev.node_ids.includes(nodeId)
        ? prev.node_ids.filter(id => id !== nodeId)
        : [...prev.node_ids, nodeId],
    }))

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: '分组名', render: (g: Group) => <span className="font-medium text-slate-900">{g.name}</span> },
    { key: 'description', label: '描述', render: (g: Group) => g.description || <span className="text-slate-300">—</span> },
    { key: 'node_count', label: '节点数', render: (g: Group) => <Badge label={String(g.node_count)} variant="blue" /> },
    { key: 'active', label: '状态', render: (g: Group) => <Badge label={g.active ? '启用' : '禁用'} variant={g.active ? 'green' : 'gray'} /> },
    {
      key: 'actions', label: '操作',
      render: (g: Group) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(g)} className="text-xs text-slate-500 hover:text-slate-900">编辑</button>
          <button
            onClick={() => { if (confirm(`确认删除分组「${g.name}」？`)) remove.mutate(g.id) }}
            className="text-xs text-red-500 hover:text-red-700"
          >删除</button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">分组管理</h1>
          <p className="text-sm text-slate-500 mt-0.5">管理用户分组和关联节点 · 共 {data?.total ?? 0} 个</p>
        </div>
        <Btn onClick={openCreate}>新增分组</Btn>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </div>

      <Modal
        open={modal.open}
        onClose={closeModal}
        title={modal.group ? '编辑分组' : '新增分组'}
        size="sm"
        footer={
          <>
            <Btn variant="secondary" onClick={closeModal}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="space-y-3">
          <Field label="分组名 *" value={form.name} onChange={f('name')} placeholder="如：高速节点" />
          <Field label="描述" value={form.description} onChange={f('description')} />
          {/* 关联节点 */}
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-2">
              关联节点 <span className="text-slate-400 font-normal">（已选 {form.node_ids.length} 个）</span>
            </label>
            {(allNodes ?? []).length === 0 ? (
              <p className="text-xs text-slate-400 px-1">暂无节点，请先添加节点</p>
            ) : (
              <div className="border border-slate-200 rounded-lg max-h-48 overflow-y-auto divide-y divide-slate-100">
                {(allNodes ?? []).map(n => (
                  <label key={n.id} className="flex items-center gap-2 px-3 py-2 hover:bg-slate-50 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={form.node_ids.includes(n.id)}
                      onChange={() => toggleNode(n.id)}
                      className="h-4 w-4 text-indigo-600 rounded border-slate-300"
                    />
                    <span className="text-sm text-slate-700">{n.name}</span>
                    {n.region && <span className="text-xs text-slate-400">{n.region}</span>}
                    <span className="ml-auto text-xs text-slate-400">{n.ip}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          {err && <p className="text-sm text-red-500">{err}</p>}
        </div>
      </Modal>
    </div>
  )
}
