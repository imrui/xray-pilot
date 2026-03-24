import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { groupApi } from '@/lib/api'
import type { Group } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn } from '@/components/ui/Form'

const PAGE_SIZE = 20

interface FormState {
  name: string
  description: string
}

const emptyForm = (): FormState => ({ name: '', description: '' })

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
  const openEdit = (g: Group) => { setForm({ name: g.name, description: g.description }); setErr(''); setModal({ open: true, group: g }) }
  const closeModal = () => setModal({ open: false })

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [k]: e.target.value }))

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: '分组名', render: (g: Group) => <span className="font-medium">{g.name}</span> },
    { key: 'description', label: '描述', render: (g: Group) => g.description || <span className="text-gray-400">—</span> },
    { key: 'node_count', label: '节点数', render: (g: Group) => <Badge label={String(g.node_count)} variant="blue" /> },
    { key: 'active', label: '状态', render: (g: Group) => <Badge label={g.active ? '启用' : '禁用'} variant={g.active ? 'green' : 'gray'} /> },
    {
      key: 'actions', label: '操作',
      render: (g: Group) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(g)} className="text-xs text-gray-600 hover:text-gray-900">编辑</button>
          <button onClick={() => { if (confirm(`确认删除分组「${g.name}」？`)) remove.mutate(g.id) }}
            className="text-xs text-red-500 hover:text-red-700">删除</button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">分组管理</h2>
          <p className="text-sm text-gray-500 mt-0.5">管理用户分组和关联节点 · 共 {data?.total ?? 0} 个</p>
        </div>
        <Btn onClick={openCreate}>新增分组</Btn>
      </div>

      <div className="bg-white rounded-lg border border-gray-200">
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
          {err && <p className="text-sm text-red-500">{err}</p>}
        </div>
      </Modal>
    </div>
  )
}
