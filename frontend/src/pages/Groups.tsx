import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers2, PencilLine, Plus } from 'lucide-react'
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

const DEFAULT_PAGE_SIZE = 10
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

interface FormState {
  name: string
  description: string
  node_ids: number[]
}

const emptyForm = (): FormState => ({ name: '', description: '', node_ids: [] })

function Switch({ checked, onChange }: { checked: boolean; onChange: (next: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative h-6 w-11 rounded-full border transition ${
        checked ? 'border-emerald-500 bg-emerald-500' : 'border-[var(--border-strong)] bg-slate-200 dark:border-[var(--border)] dark:bg-white/10'
      }`}
    >
      <span
        className={`absolute top-0.5 h-4.5 w-4.5 rounded-full bg-white shadow transition ${
          checked ? 'left-[22px]' : 'left-0.5'
        }`}
      />
    </button>
  )
}

export default function Groups() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [drawer, setDrawer] = useState<{ open: boolean; group?: Group }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['groups', page, pageSize],
    queryFn: () => groupApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
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
  const toggle = useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) => groupApi.update(id, { active }),
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
        <div className="space-y-1.5">
          <div className="font-semibold">{g.name}</div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge label={`${g.node_count} 个节点`} variant="blue" />
            <span className="text-xs text-soft">{g.description || '未填写描述'}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'active',
      label: '状态',
      render: (g: Group) => <Switch checked={g.active} onChange={(next) => toggle.mutate({ id: g.id, active: next })} />,
    },
    {
      key: 'actions',
      label: '操作',
      render: (g: Group) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => openEdit(g)}
            className="inline-flex h-9 items-center gap-1 rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-semibold text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
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
        description="把分组视为节点池和分发策略容器，而不是单纯的命名项。编辑时在右侧直接维护关联节点。"
        actions={
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增分组
          </Btn>
        }
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

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <SurfaceCard className="p-4">
          <Table columns={columns} data={filteredGroups} loading={isLoading} />
          <div className="mt-4 flex flex-col gap-3 border-t border-[var(--border)] pt-4 md:flex-row md:items-center md:justify-between">
            <label className="inline-flex items-center gap-2 text-sm text-soft">
              分页
              <select
                value={pageSize}
                onChange={(e) => {
                  setPageSize(Number(e.target.value))
                  setPage(1)
                }}
                className="h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"
              >
                {PAGE_SIZE_OPTIONS.map((size) => (
                  <option key={size} value={size}>
                    {size} / 页
                  </option>
                ))}
              </select>
            </label>
            <Pagination page={page} pageSize={pageSize} total={data?.total ?? 0} onChange={setPage} />
          </div>
        </SurfaceCard>

        <SurfaceCard className="p-5">
          <div className="mb-4">
            <div className="text-sm font-semibold">分组说明</div>
            <p className="mt-2 text-sm leading-6 text-soft">
              建议把分组用于表达线路池、地区池或特定分发策略。这样用户和订阅逻辑在后续配置里会更清晰。
            </p>
          </div>
          <div className="space-y-3">
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Node Pool</div>
              <div className="mt-2 text-sm font-semibold">节点按职责归类</div>
              <p className="mt-2 text-xs leading-5 text-soft">比如海外中转、高可用入口、实验线路，不需要把这些语义散落在节点备注里。</p>
            </div>
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Distribution</div>
              <div className="mt-2 text-sm font-semibold">为订阅分发做准备</div>
              <p className="mt-2 text-xs leading-5 text-soft">后续用户绑定分组后，订阅生成会更自然，排障时也更容易回溯。</p>
            </div>
          </div>
        </SurfaceCard>
      </div>

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
                          ? 'border-[var(--accent)]/30 bg-[var(--accent-soft)]'
                          : 'border-[var(--border)] bg-[var(--panel-strong)] hover:bg-[var(--panel-muted)]'
                      }`}
                    >
                      <div className={`h-2.5 w-2.5 rounded-full ${checked ? 'bg-[var(--accent)]' : 'bg-slate-400/60'}`} />
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
