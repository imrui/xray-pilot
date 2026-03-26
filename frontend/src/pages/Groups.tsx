import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { groupApi, nodeApi } from '@/lib/api'
import type { Group } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

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
    queryFn: () => groupApi.list({ page, page_size: PAGE_SIZE }).then((r) => r.data.data!),
  })

  const { data: allNodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 200 }).then((r) => r.data.data?.list ?? []),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['groups'] })

  const save = useMutation({
    mutationFn: () => (modal.group ? groupApi.update(modal.group.id, form) : groupApi.create(form)),
    onSuccess: () => {
      invalidate()
      closeModal()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({
    mutationFn: (id: number) => groupApi.remove(id),
    onSuccess: invalidate,
  })

  const openCreate = () => {
    setForm(emptyForm())
    setErr('')
    setModal({ open: true })
  }

  const openEdit = (g: Group) => {
    setForm({ name: g.name, description: g.description, node_ids: g.node_ids ?? [] })
    setErr('')
    setModal({ open: true, group: g })
  }

  const closeModal = () => setModal({ open: false })

  const f = (k: 'name' | 'description') => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const toggleNode = (nodeId: number) =>
    setForm((prev) => ({
      ...prev,
      node_ids: prev.node_ids.includes(nodeId) ? prev.node_ids.filter((id) => id !== nodeId) : [...prev.node_ids, nodeId],
    }))

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: '分组名', render: (g: Group) => <span className="font-semibold">{g.name}</span> },
    { key: 'description', label: '描述', render: (g: Group) => g.description || <span className="text-soft">未填写</span> },
    { key: 'node_count', label: '节点数', render: (g: Group) => <Badge label={String(g.node_count)} variant="blue" /> },
    { key: 'active', label: '状态', render: (g: Group) => <Badge label={g.active ? '启用' : '禁用'} variant={g.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (g: Group) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(g)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
          <button
            onClick={() => {
              if (confirm(`确认删除分组「${g.name}」？`)) remove.mutate(g.id)
            }}
            className="text-xs text-rose-500 transition hover:brightness-110"
          >
            删除
          </button>
        </div>
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="分组管理"
        description="为用户集和节点集建立稳定的编排关系，减少手工配置带来的维护噪音。"
        actions={<Btn onClick={openCreate}>新增分组</Btn>}
        stats={[
          { label: '总分组数', value: data?.total ?? 0 },
          { label: '已发现节点', value: allNodes?.length ?? 0 },
          { label: '分页尺寸', value: PAGE_SIZE },
        ]}
      />

      <SurfaceCard className="p-4">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

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
        <div className="space-y-4">
          <Field label="分组名 *" value={form.name} onChange={f('name')} placeholder="如：高速节点" />
          <Field label="描述" value={form.description} onChange={f('description')} />
          <div>
            <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">
              关联节点
              <span className="ml-2 text-[11px] normal-case tracking-normal text-soft">已选 {form.node_ids.length} 个</span>
            </label>
            {(allNodes ?? []).length === 0 ? (
              <p className="mt-2 px-1 text-xs text-soft">暂无节点，请先添加节点</p>
            ) : (
              <div className="mt-2 max-h-56 overflow-y-auto rounded-2xl border bg-[var(--panel-muted)]">
                {(allNodes ?? []).map((n) => (
                  <label key={n.id} className="flex items-center gap-3 border-b border-[var(--border)] px-4 py-3 last:border-b-0">
                    <input
                      type="checkbox"
                      checked={form.node_ids.includes(n.id)}
                      onChange={() => toggleNode(n.id)}
                      className="h-4 w-4 rounded border-[var(--border-strong)]"
                    />
                    <span className="text-sm font-medium">{n.name}</span>
                    {n.region && <span className="text-xs text-soft">{n.region}</span>}
                    <span className="ml-auto text-xs text-soft">{n.ip}</span>
                  </label>
                ))}
              </div>
            )}
          </div>
          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Modal>
    </PageShell>
  )
}
