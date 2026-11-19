import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Layers2, PencilLine, Plus } from 'lucide-react'
import { groupApi, nodeApi } from '@/lib/api'
import type { Group } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup, Switch } from '@/components/ui/Form'
import { PageShell } from '@/components/ui/Page'
import { HelpPopover } from '@/components/ui/HelpPopover'
import { fetchAllPages } from '@/lib/pagination'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { BulkBar, FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { useConfirm } from '@/lib/confirm-context'
import { Tooltip } from '@/components/ui/Tooltip'
import { BindingEditor } from '@/components/routing/BindingEditor'

const DEFAULT_PAGE_SIZE = 10
const DESCRIPTION_PREVIEW_LIMIT = 24

interface FormState {
  name: string
  description: string
  node_ids: number[]
}

const emptyForm = (): FormState => ({ name: '', description: '', node_ids: [] })

function truncateDescription(text: string, limit = DESCRIPTION_PREVIEW_LIMIT) {
  const normalized = text.trim()
  if (normalized.length <= limit) return normalized
  return `${normalized.slice(0, limit)}...`
}

function GroupNodesTooltip({ names }: { names: string[] }) {
  return (
    <div className="max-h-52 space-y-1 overflow-y-auto pr-1">
      {names.map((name) => (
        <div key={name} className="whitespace-nowrap">
          {name}
        </div>
      ))}
    </div>
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
  const [routingDirty, setRoutingDirty] = useState(false)
  const [routingBusy, setRoutingBusy] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive'>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['groups', page, pageSize, search.trim(), statusFilter],
    queryFn: () => groupApi.list({
      page,
      page_size: pageSize,
      keyword: search.trim() || undefined,
      active: statusFilter === 'all' ? undefined : statusFilter === 'active',
    }).then((r) => r.data.data!),
  })

  const { data: allNodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => fetchAllPages((page, pageSize) =>
      nodeApi.list({ page, page_size: pageSize }).then((r) => r.data.data!)),
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
    setRoutingDirty(false)
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true })
  }

  const openEdit = (g: Group) => {
    setRoutingDirty(false)
    const next = { name: g.name, description: g.description, node_ids: g.node_ids ?? [] }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true, group: g })
  }

  const closeDrawer = () => setDrawer({ open: false })
  const confirmCloseDrawer = async () => {
    if (!dirty && !routingDirty) return true
    return confirm({
      title: '放弃未保存的分组修改？',
      description: '关闭后，未保存的分组信息、成员和路由修改将丢失。',
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

  const filteredGroups = data?.list ?? []
  const changeStatus = (value: typeof statusFilter) => {
    setStatusFilter(value)
    setPage(1)
  }
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
  const nodeNameMap = new Map((allNodes ?? []).map((node) => [node.id, node.name]))

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
      label: '名称',
      render: (g: Group) => (
        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{g.name}</span>
            <span className="text-xs text-faint">#{g.id}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'node_count',
      label: '节点数',
      render: (g: Group) => (
        <Tooltip
          content={
            g.node_ids.length > 0
              ? <GroupNodesTooltip names={g.node_ids.map((id) => nodeNameMap.get(id)).filter((name): name is string => Boolean(name))} />
              : '当前分组没有关联节点'
          }
          side="right"
          className="max-w-[320px] whitespace-normal"
        >
          <button
            type="button"
            onClick={() => openEdit(g)}
            title="点击编辑该分组"
            className="inline-flex rounded-full transition hover:-translate-y-0.5 hover:shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          >
            <Badge label={`${g.node_count} 个节点`} variant="blue" />
          </button>
        </Tooltip>
      ),
    },
    {
      key: 'active',
      label: '状态',
      render: (g: Group) => <Switch checked={g.active} onChange={(next) => toggle.mutate({ id: g.id, active: next })} />,
    },
    {
      key: 'description',
      label: '描述',
      render: (g: Group) => {
        const text = g.description?.trim() || '未填写描述'
        const preview = truncateDescription(text)
        return (
          <Tooltip content={text} side="right" className="max-w-[320px] whitespace-normal">
            <span className="block max-w-[280px] truncate text-xs text-soft">
              {preview}
            </span>
          </Tooltip>
        )
      },
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
      <ListToolbar
        actions={
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增分组
          </Btn>
        }
        searchValue={search}
        searchPlaceholder="搜索分组名或描述"
        onSearchChange={(value) => {
          setSearch(value)
          setPage(1)
        }}
        meta={
          <HelpPopover title="分组说明">
            <p>分组用于组织同一地区、线路或用途的节点，供订阅用户按组使用。</p>
            <p>编辑分组时可以调整成员，并选择继承全局路由或指定一个模板版本。</p>
          </HelpPopover>
        }
        filters={
          <>
            <FilterChip active={statusFilter === 'all'} onClick={() => changeStatus('all')}>全部</FilterChip>
            <FilterChip active={statusFilter === 'active'} onClick={() => changeStatus('active')}>启用</FilterChip>
            <FilterChip active={statusFilter === 'inactive'} onClick={() => changeStatus('inactive')}>禁用</FilterChip>
          </>
        }
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

      <Table columns={columns} data={filteredGroups} loading={isLoading} />
      <Pagination
        page={page}
        pageSize={pageSize}
        total={data?.total}
        onChange={setPage}
        onPageSizeChange={setPageSize}
      />

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.group ? `编辑分组 · ${drawer.group.name}` : '新增分组'}
        description="在右侧集中维护分组名称、说明和节点关联关系。"
        dirty={dirty || routingDirty}
        saving={save.isPending || routingBusy}
        onBeforeClose={confirmCloseDrawer}
        footer={
          <>
            <Btn variant="secondary" disabled={routingBusy} onClick={() => void confirmCloseDrawer().then(ok => { if (ok) closeDrawer() })}>取消</Btn>
            <Btn loading={save.isPending} disabled={routingDirty || routingBusy} title={routingDirty ? '请先保存或放弃路由修改' : undefined} onClick={() => save.mutate()}>保存分组</Btn>
          </>
        }
      >
        <div className="space-y-4">
          <FieldGroup title="分组信息" description="用于区分不同节点池和用户分发策略。">
            <Field label="分组名 *" value={form.name} onChange={f('name')} placeholder="如：高速节点" />
            <Field label="描述" value={form.description} onChange={f('description')} placeholder="例如：高优先级节点池 / 海外节点池" />
          </FieldGroup>

          {drawer.group && <FieldGroup title="路由策略" description="继承全局默认或为该分组指定固定模板版本。"><BindingEditor key={drawer.group.id} scope="group" id={drawer.group.id} onDirtyChange={setRoutingDirty} onBusyChange={setRoutingBusy} /></FieldGroup>}
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
                      <div className={`h-2.5 w-2.5 rounded-full ${checked ? 'bg-[var(--accent)]' : 'bg-[var(--border-strong)]'}`} />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium">{n.name}</span>
                          {n.owner && <Badge label={n.owner} variant="gray" />}
                        </div>
                        <div className="mt-1 text-xs text-soft">
                          {[n.region, n.ip, n.domain].filter(Boolean).join(' · ')}
                        </div>
                      </div>
                      <Layers2 className="h-4 w-4 text-soft" />
                    </button>
                  )
                })}
              </div>
            )}
          </FieldGroup>

          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      </Drawer>
    </PageShell>
  )
}
