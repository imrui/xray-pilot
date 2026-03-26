import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers2, PencilLine } from 'lucide-react'
import { groupApi, nodeApi } from '@/lib/api'
import type { Group } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { BulkBar, FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { useConfirm } from '@/components/ui/ConfirmProvider'

const PAGE_SIZE = 20

interface FormState {
  name: string
  description: string
  node_ids: number[]
}

const emptyForm = (): FormState => ({ name: '', description: '', node_ids: [] })

export default function Groups() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [drawer, setDrawer] = useState<{ open: boolean; group?: Group }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

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
    mutationFn: () => (drawer.group ? groupApi.update(drawer.group.id, form) : groupApi.create(form)),
    onSuccess: () => {
      invalidate()
      closeDrawer()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({
    mutationFn: (id: number) => groupApi.remove(id),
    onSuccess: invalidate,
  })

  const openCreate = () => {
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true })
  }

  const openEdit = (g: Group) => {
    const next = { name: g.name, description: g.description, node_ids: g.node_ids ?? [] }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true, group: g })
  }

  const closeDrawer = () => setDrawer({ open: false })
  const confirmCloseDrawer = async () => {
    if (!dirty) return true
    return confirm({
      title: '放弃未保存的分组修改？',
      description: '关闭后，当前分组名称和节点关联更改将不会保存。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    })
  }

  const f = (k: 'name' | 'description') => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const toggleNode = (nodeId: number) =>
    setForm((prev) => ({
      ...prev,
      node_ids: prev.node_ids.includes(nodeId) ? prev.node_ids.filter((id) => id !== nodeId) : [...prev.node_ids, nodeId],
    }))

  const filteredGroups = (data?.list ?? []).filter((g) => {
    const keyword = search.trim().toLowerCase()
    const matchesKeyword =
      keyword === '' || g.name.toLowerCase().includes(keyword) || g.description.toLowerCase().includes(keyword)
    const matchesStatus = statusFilter === 'all' || (statusFilter === 'active' && g.active) || (statusFilter === 'inactive' && !g.active)
    return matchesKeyword && matchesStatus
  })
  const allVisibleSelected = filteredGroups.length > 0 && filteredGroups.every((g) => selectedIds.includes(g.id))
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm) && drawer.open

  const toggleSelect = (id: number) =>
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))

  const toggleSelectVisible = () =>
    setSelectedIds((prev) => {
      if (allVisibleSelected) return prev.filter((id) => !filteredGroups.some((g) => g.id === id))
      return Array.from(new Set([...prev, ...filteredGroups.map((g) => g.id)]))
    })

  const selectedGroups = filteredGroups.filter((g) => selectedIds.includes(g.id))

  const columns = [
    {
      key: 'select',
      label: (
        <input type="checkbox" checked={allVisibleSelected} onChange={toggleSelectVisible} aria-label="选择当前页可见分组" className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ) as unknown as string,
      render: (g: Group) => (
        <input type="checkbox" checked={selectedIds.includes(g.id)} onChange={() => toggleSelect(g.id)} aria-label={`选择分组 ${g.name}`} className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ),
    },
    {
      key: 'name',
      label: '分组',
      render: (g: Group) => (
        <div className="space-y-1">
          <div className="font-semibold">{g.name}</div>
          <div className="flex items-center gap-2">
            <Badge label={`${g.node_count} 个节点`} variant="blue" />
            <span className="text-xs text-soft">{g.description || '未填写描述'}</span>
          </div>
        </div>
      ),
    },
    { key: 'active', label: '状态', render: (g: Group) => <Badge label={g.active ? '启用' : '禁用'} variant={g.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (g: Group) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => openEdit(g)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-soft transition hover:bg-[var(--panel)] hover:text-[var(--text)]"
          >
            <PencilLine className="h-3.5 w-3.5" />
            编辑
          </button>
          <ActionMenu
            items={[
              {
                label: '删除分组',
                danger: true,
                onSelect: async () => {
                  const ok = await confirm({
                    title: `删除分组「${g.name}」？`,
                    description: '删除后，该分组与节点的关联会被移除。',
                    confirmText: '删除分组',
                    cancelText: '取消',
                    tone: 'danger',
                  })
                  if (ok) remove.mutate(g.id)
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
        title="分组管理"
        description="分组不再是单纯的表格项，编辑时直接进入抽屉配置关联节点，减少来回切换。"
        actions={<Btn onClick={openCreate}>新增分组</Btn>}
        stats={[
          { label: '总分组数', value: data?.total ?? 0 },
          { label: '已发现节点', value: allNodes?.length ?? 0 },
          { label: '分页尺寸', value: PAGE_SIZE },
        ]}
      />

      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索分组名或描述"
        onSearchChange={setSearch}
        filters={
          <>
            <FilterChip active={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>全部</FilterChip>
            <FilterChip active={statusFilter === 'active'} onClick={() => setStatusFilter('active')}>启用</FilterChip>
            <FilterChip active={statusFilter === 'inactive'} onClick={() => setStatusFilter('inactive')}>禁用</FilterChip>
          </>
        }
        meta={`当前页匹配 ${filteredGroups.length} / ${(data?.list ?? []).length} 条`}
        bulkBar={
          selectedGroups.length > 0 ? (
            <BulkBar>
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="text-sm text-soft">已选择 <span className="font-semibold text-[var(--text)]">{selectedGroups.length}</span> 个分组</div>
                <div className="flex flex-wrap gap-2">
                  <Btn
                    variant="danger"
                    onClick={async () => {
                      const ok = await confirm({
                        title: `删除已选中的 ${selectedGroups.length} 个分组？`,
                        description: '批量删除会一并移除这些分组和节点之间的关联。',
                        confirmText: '批量删除',
                        cancelText: '取消',
                        tone: 'danger',
                      })
                      if (!ok) return
                      await Promise.all(selectedGroups.map((g) => groupApi.remove(g.id)))
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

      <SurfaceCard className="p-4">
        <Table columns={columns} data={filteredGroups} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.group ? `编辑分组 · ${drawer.group.name}` : '新增分组'}
        description="在右侧集中维护分组名称、说明和节点关联关系。"
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
          <FieldGroup title="分组信息" description="用于区分不同节点池和用户分发策略。">
            <Field label="分组名 *" value={form.name} onChange={f('name')} placeholder="如：高速节点" />
            <Field label="描述" value={form.description} onChange={f('description')} placeholder="例如：高优先级节点池 / 海外节点池" />
          </FieldGroup>

          <FieldGroup title="关联节点" description={`已选择 ${form.node_ids.length} 个节点，可直接在列表里切换。`}>
            {(allNodes ?? []).length === 0 ? (
              <p className="text-xs text-soft">暂无节点，请先添加节点。</p>
            ) : (
              <div className="grid gap-3">
                {(allNodes ?? []).map((n) => {
                  const checked = form.node_ids.includes(n.id)
                  return (
                    <button
                      key={n.id}
                      type="button"
                      onClick={() => toggleNode(n.id)}
                      className={`flex items-center gap-3 rounded-2xl border px-4 py-3 text-left transition ${
                        checked
                          ? 'border-emerald-500/30 bg-emerald-500/10'
                          : 'border-[var(--border)] bg-[var(--panel)] hover:bg-[var(--panel-muted)]'
                      }`}
                    >
                      <div className={`h-2.5 w-2.5 rounded-full ${checked ? 'bg-emerald-400' : 'bg-slate-400/60'}`} />
                      <div className="min-w-0 flex-1">
                        <div className="font-medium">{n.name}</div>
                        <div className="mt-1 text-xs text-soft">{n.region ? `${n.region} · ${n.ip}` : n.ip}</div>
                      </div>
                      <Layers2 className="h-4 w-4 text-soft" />
                    </button>
                  )
                })}
              </div>
            )}
          </FieldGroup>

          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Drawer>
    </PageShell>
  )
}
