import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Copy, FileCode, Filter, PencilLine, Plus, RefreshCw, Search, Sparkles, Terminal, Wifi } from 'lucide-react'
import { OneClickInstallDialog } from '@/components/OneClickInstallDialog'
import { nodeApi, profileApi } from '@/lib/api'
import { generateShortIds } from '@/lib/keygen'
import { copyText } from '@/lib/clipboard'
import { protocolBadgeVariant, protocolLabel } from '@/lib/protocol'
import type { InboundProfile, Node, NodeKey, SyncStatus } from '@/types'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup } from '@/components/ui/Form'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/components/ui/ConfirmProvider'
import { Tooltip } from '@/components/ui/Tooltip'
import { pushToast } from '@/lib/notify'

const DEFAULT_PAGE_SIZE = 10
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

const statusMeta: Record<SyncStatus, { label: string; variant: 'green' | 'yellow' | 'red' | 'gray'; tip: string }> = {
  synced: { label: '已同步', variant: 'green', tip: '配置已是最新' },
  drifted: { label: '待同步', variant: 'yellow', tip: '检测到配置变更或远端配置不一致，建议重新同步' },
  failed: { label: '同步失败', variant: 'red', tip: '同步失败，建议先测试 SSH 连通性' },
  pending: { label: '待同步', variant: 'gray', tip: '节点尚未同步，请点击同步按钮' },
}

const unconfiguredStatusMeta = {
  label: '未配置协议',
  variant: 'gray' as const,
  tip: '当前节点未绑定任何协议，无法生成可用代理配置。请先为节点配置至少一个协议。',
}

interface FormState {
  name: string
  region: string
  owner: string
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
  owner: '',
  ip: '',
  domain: '',
  ssh_port: '22',
  ssh_user: 'root',
  ssh_key_path: '',
  remark: '',
})

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

export default function Nodes() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [drawer, setDrawer] = useState<{ open: boolean; node?: Node }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [previewNode, setPreviewNode] = useState<Node | null>(null)
  const [protocolNode, setProtocolNode] = useState<Node | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'synced' | 'drifted' | 'failed' | 'pending'>('all')
  const [regionFilter, setRegionFilter] = useState<string>('all')
  const [ownerFilter, setOwnerFilter] = useState<string>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [installOpen, setInstallOpen] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page, pageSize],
    queryFn: () => nodeApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
  })
  const { data: profilesData } = useQuery({
    queryKey: ['profiles-for-nodes'],
    queryFn: () => profileApi.list({ page: 1, page_size: 100 }).then((r) => r.data.data!),
  })
  const { data: nodeKeysByNodeId = {} } = useQuery({
    queryKey: ['node-keys-for-list', data?.list?.map((node) => node.id).join(',') ?? ''],
    queryFn: async () => {
      const nodes = data?.list ?? []
      const entries = await Promise.all(
        nodes.map(async (node) => {
          const res = await nodeApi.getKeys(node.id)
          return [node.id, res.data.data ?? []] as const
        }),
      )
      return Object.fromEntries(entries) as Record<number, NodeKey[]>
    },
    enabled: (data?.list?.length ?? 0) > 0,
  })

  const regions = useMemo(() => {
    return Array.from(new Set((data?.list ?? []).map((n) => n.region).filter(Boolean)))
  }, [data?.list])

  const owners = useMemo(() => {
    return Array.from(new Set((data?.list ?? []).map((n) => n.owner).filter(Boolean)))
  }, [data?.list])

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
  })
  const sync = useMutation({
    mutationFn: (id: number) => nodeApi.sync(id),
    onSuccess: (res) => {
      pushToast({
        title: '节点同步成功',
        description: res.data.data?.message ?? '节点配置已同步到远端。',
        variant: 'success',
      })
      invalidate()
    },
  })
  const syncDrifted = useMutation({
    mutationFn: () => nodeApi.syncDrifted(),
    onSuccess: (res) => {
      const summary = res.data.data
      pushToast({
        title: '批量同步已完成',
        description: summary
          ? `总计 ${summary.total} 个节点，成功 ${summary.success} 个，失败 ${summary.failed} 个。`
          : '待处理节点已完成同步。',
        variant: summary?.failed ? 'warning' : 'success',
      })
      invalidate()
    },
  })
  const testSSH = useMutation({
    mutationFn: (id: number) => nodeApi.testSSH(id),
    onSuccess: (res, id) => {
      invalidate()
      const result = res.data.data
      if (result?.ok) {
        pushToast({
          title: 'SSH 连接正常',
          description: `节点 #${id} 延迟 ${result.latency_ms}ms`,
          variant: 'success',
        })
        return
      }
      pushToast({
        title: 'SSH 测试失败',
        description: result?.error || '连接失败，请检查密钥、端口和 known_hosts 配置。',
        variant: 'warning',
      })
    },
    onError: (e: Error) => {
      pushToast({
        title: 'SSH 测试失败',
        description: e.message,
        variant: 'error',
      })
    },
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
      owner: n.owner,
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

  const filteredNodes = useMemo(() => {
    return [...(data?.list ?? [])]
      .sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' }))
      .filter((n) => {
        const keyword = search.trim().toLowerCase()
        const matchesKeyword =
          keyword === '' ||
          n.name.toLowerCase().includes(keyword) ||
          n.ip.toLowerCase().includes(keyword) ||
          n.domain.toLowerCase().includes(keyword) ||
          n.region.toLowerCase().includes(keyword) ||
          n.owner.toLowerCase().includes(keyword)
        const matchesStatus = statusFilter === 'all' || n.sync_status === statusFilter
        const matchesRegion = regionFilter === 'all' || n.region === regionFilter
        const matchesOwner = ownerFilter === 'all' || n.owner === ownerFilter
        return matchesKeyword && matchesStatus && matchesRegion && matchesOwner
      })
  }, [data?.list, search, statusFilter, regionFilter, ownerFilter])

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
  const activeProfiles = (profilesData?.list ?? []).filter((item) => item.active)
  const activeProfileMap = new Map(activeProfiles.map((profile) => [profile.id, profile]))

  const renderGroupCell = (groupNames?: string[]) => {
    if (!groupNames || groupNames.length === 0) return <span className="text-soft">—</span>

    const visible = groupNames.slice(0, 2)
    const hiddenCount = groupNames.length - visible.length

    return (
      <div className="flex flex-wrap items-center gap-1.5">
        {visible.map((groupName) => (
          <Badge key={groupName} label={groupName} variant="blue" />
        ))}
        {hiddenCount > 0 && (
          <Tooltip
            content={
              <div className="space-y-1">
                {groupNames.map((groupName) => (
                  <div key={groupName}>{groupName}</div>
                ))}
              </div>
            }
            className="max-w-[240px] whitespace-normal"
          >
            <span tabIndex={0} className="inline-flex">
              <Badge label={`+${hiddenCount}`} variant="gray" />
            </span>
          </Tooltip>
        )}
      </div>
    )
  }

  const getNodeKeys = (nodeId: number) => nodeKeysByNodeId[nodeId] ?? []
  const getConfiguredProfiles = (nodeId: number) =>
    getNodeKeys(nodeId)
      .map((key) => activeProfileMap.get(key.profile_id))
      .filter((item): item is InboundProfile => Boolean(item))

  const renderProtocolCell = (node: Node) => {
    const matchedProfiles = getConfiguredProfiles(node.id)

    if (matchedProfiles.length === 0) {
      return (
        <button
          type="button"
          onClick={() => setProtocolNode(node)}
          className="inline-flex h-7 items-center rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-2.5 text-xs font-medium text-soft transition hover:bg-[var(--panel-strong)] hover:text-[var(--text)]"
        >
          未配置
        </button>
      )
    }

    const [firstProfile, ...restProfiles] = matchedProfiles

    return (
      <div className="flex flex-wrap items-center gap-1.5">
        <button
          type="button"
          onClick={() => setProtocolNode(node)}
          title="点击管理该节点协议"
          className="inline-flex rounded-full transition hover:-translate-y-0.5 hover:shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
        >
          <Badge label={firstProfile.name} variant={protocolBadgeVariant(firstProfile.protocol)} />
        </button>
        {restProfiles.length > 0 && (
          <Tooltip
            side="right"
            content={
              <div className="space-y-1">
                {matchedProfiles.map((profile) => (
                  <div key={profile.id}>
                    {profile.name} <span className="text-faint">· {protocolLabel(profile.protocol)}</span>
                  </div>
                ))}
              </div>
            }
            className="max-w-[260px] whitespace-normal"
          >
            <button
              type="button"
              onClick={() => setProtocolNode(node)}
              className="inline-flex h-6 items-center rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
            >
              +{restProfiles.length}
            </button>
          </Tooltip>
        )}
      </div>
    )
  }

  return (
    <PageShell className="space-y-6">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex flex-1 flex-col gap-3 md:flex-row">
          <label className="relative min-w-0 flex-1 md:max-w-[380px]">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索节点名称、地址..."
              className="h-11 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] pl-10 pr-3 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
            />
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
              className="w-full bg-transparent text-sm outline-none"
            >
              <option value="all">全部状态</option>
              <option value="synced">已同步</option>
              <option value="drifted">待同步</option>
              <option value="failed">同步失败</option>
              <option value="pending">待同步</option>
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select value={regionFilter} onChange={(e) => setRegionFilter(e.target.value)} className="w-full bg-transparent text-sm outline-none">
              <option value="all">全部地区</option>
              {regions.map((region) => (
                <option key={region} value={region}>
                  {region}
                </option>
              ))}
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select value={ownerFilter} onChange={(e) => setOwnerFilter(e.target.value)} className="w-full bg-transparent text-sm outline-none">
              <option value="all">全部所有者</option>
              {owners.map((owner) => (
                <option key={owner} value={owner}>
                  {owner}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="flex items-center gap-2">
          <Btn variant="secondary" loading={syncDrifted.isPending} onClick={() => syncDrifted.mutate()}>
            <RefreshCw className="h-4 w-4" />
            同步待处理节点
          </Btn>
          <Btn variant="secondary" onClick={() => setInstallOpen(true)}>
            <Terminal className="h-4 w-4" />
            一键接入
          </Btn>
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增节点
          </Btn>
        </div>
      </div>

      <p className="text-xs text-soft">
        协议绑定提示：只有在“配置节点密钥”中保存过的协议，才会参与该节点的配置生成；保存空对象 <code>{'{}'}</code> 表示继承协议默认参数。
      </p>

      {selectedNodes.length > 0 && (
        <SurfaceCard className="p-3">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="text-sm text-soft">
              已选择 <span className="font-semibold text-[var(--text)]">{selectedNodes.length}</span> 个节点
            </div>
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
              }}>批量切换启用</Btn>
              <Btn
                variant="danger"
                onClick={async () => {
                  const ok = await confirm({
                    title: `删除已选中的 ${selectedNodes.length} 个节点？`,
                    description: '该操作不可撤销。',
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
        </SurfaceCard>
      )}

      <SurfaceCard className="overflow-visible">
        <div className="overflow-x-auto overflow-y-visible">
          <table className="w-full min-w-[1480px] text-left text-sm">
            <thead className="border-b border-[var(--border)] bg-[var(--panel-muted)]">
              <tr>
                <th className="w-10 px-4 py-3">
                  <input
                    type="checkbox"
                    checked={allVisibleSelected}
                    onChange={toggleSelectVisible}
                    className="h-4 w-4 rounded border-[var(--border-strong)]"
                    aria-label="选择当前页可见节点"
                  />
                </th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">节点名称</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">连接地址</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">状态</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">分组</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">所有者</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">用户 / Xray</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">协议</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">状态</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">最后同步</th>
                <th className="px-4 py-3 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-faint">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr>
                  <td colSpan={11} className="px-4 py-10 text-center text-soft">加载中…</td>
                </tr>
              ) : filteredNodes.length === 0 ? (
                <tr>
                  <td colSpan={11} className="px-4 py-10 text-center text-soft">暂无节点数据</td>
                </tr>
              ) : (
                filteredNodes.map((n) => (
                  <tr key={n.id} className="transition hover:bg-white/5">
                    <td className="px-4 py-3.5">
                      <input
                        type="checkbox"
                        checked={selectedIds.includes(n.id)}
                        onChange={() => toggleSelect(n.id)}
                        className="h-4 w-4 rounded border-[var(--border-strong)]"
                        aria-label={`选择节点 ${n.name}`}
                      />
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold">{n.name}</span>
                        <span className="text-xs text-faint">#{n.id}</span>
                      </div>
                      <div className="text-xs text-soft">
                        {n.region || '--'}
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="font-medium text-[var(--text)]">{n.ip}</div>
                      <div className="text-xs text-soft">{n.domain || '—'}</div>
                    </td>
                    <td className="px-4 py-3.5">
                      <Tooltip
                        content={(getConfiguredProfiles(n.id).length === 0 ? unconfiguredStatusMeta : (statusMeta[n.sync_status] ?? statusMeta.pending)).tip}
                        side="right"
                        className="max-w-[220px] whitespace-normal"
                      >
                        <span tabIndex={0} className="inline-flex">
                          <Badge
                            label={(getConfiguredProfiles(n.id).length === 0 ? unconfiguredStatusMeta : (statusMeta[n.sync_status] ?? statusMeta.pending)).label}
                            variant={(getConfiguredProfiles(n.id).length === 0 ? unconfiguredStatusMeta : (statusMeta[n.sync_status] ?? statusMeta.pending)).variant}
                          />
                        </span>
                      </Tooltip>
                    </td>
                    <td className="px-4 py-3.5">
                      {renderGroupCell(n.group_names)}
                    </td>
                    <td className="px-4 py-3.5">
                      {n.owner ? (
                        <Badge label={n.owner} variant="gray" />
                      ) : (
                        <span className="text-soft">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="font-medium text-[var(--text)]">{n.online_user_count} 人</div>
                      <div className="mt-1 inline-flex items-center gap-1.5 text-xs text-soft">
                        <span
                          className={`h-2 w-2 rounded-full ${
                            n.xray_version ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-500'
                          }`}
                        />
                        <span>{n.xray_version ? `Xray ${n.xray_version}` : 'Xray —'}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      {renderProtocolCell(n)}
                    </td>
                    <td className="px-4 py-3.5">
                      <Switch checked={n.active} onChange={() => toggle.mutate(n.id)} />
                    </td>
                    <td className="px-4 py-3.5 text-soft">{n.last_sync_at ? new Date(n.last_sync_at).toLocaleString('zh-CN') : '—'}</td>
                    <td className="px-4 py-3.5">
                      <div className="flex flex-col items-end gap-2">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            onClick={() => sync.mutate(n.id)}
                            disabled={getConfiguredProfiles(n.id).length === 0}
                            className={`inline-flex h-9 items-center gap-1 rounded-md border px-3 text-xs font-medium transition ${
                              n.sync_status === 'failed'
                                ? 'border-amber-500/40 bg-amber-500/12 text-amber-300 hover:bg-amber-500/18'
                                : 'border-[var(--border)] bg-[var(--panel-strong)] text-[var(--accent)] hover:bg-[var(--panel-muted)]'
                            } ${getConfiguredProfiles(n.id).length === 0 ? 'cursor-not-allowed opacity-45 hover:bg-inherit' : ''}`}
                          >
                            <RefreshCw className={`h-3.5 w-3.5${sync.isPending && sync.variables === n.id ? ' animate-spin' : ''}`} />
                            同步
                          </button>
                          <button
                            onClick={() => openEdit(n)}
                            className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
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
                                label: '删除节点',
                                danger: true,
                                onSelect: async () => {
                                  const ok = await confirm({
                                    title: `删除节点「${n.name}」？`,
                                    description: '该操作不可撤销。',
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
                        {n.sync_status === 'failed' && (
                          <div className="inline-flex items-center gap-1 text-[11px] text-amber-300">
                            <AlertTriangle className="h-3.5 w-3.5" />
                            建议先测试 SSH
                          </div>
                        )}
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </SurfaceCard>

      <div className="flex items-center justify-between text-sm text-soft">
        <span>共 {data?.total ?? 0} 条</span>
        <div className="flex items-center gap-2">
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
          <Btn variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>上一页</Btn>
          <span>{page}</span>
          <Btn
            variant="secondary"
            disabled={page >= Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))}
            onClick={() => setPage((p) => p + 1)}
          >
            下一页
          </Btn>
        </div>
      </div>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.node ? `编辑节点 · ${drawer.node.name}` : '新增节点'}
        description="连接信息和 SSH 参数放入右侧抽屉，避免大型表单打断列表浏览。"
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
              <Field label="所有者" value={form.owner} onChange={f('owner')} placeholder="标识节点来源，如：供应商A" />
            </div>
            <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="例如：高可用入口节点" />
          </FieldGroup>

          <FieldGroup title="连接地址" description="域名为空时使用 IP 直连。">
            <Field label="IP *" value={form.ip} onChange={f('ip')} placeholder="服务器 IP" />
            <Field label="连接域名" value={form.domain} onChange={f('domain')} placeholder="CDN/中转域名（留空用 IP）" />
          </FieldGroup>

          <FieldGroup title="SSH 参数" description="用于后续同步、健康检查和配置下发。密钥路径可留空，自动使用系统设置中的默认值。">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
              <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
            </div>
            <Field
              label="SSH 密钥路径（可选覆盖）"
              value={form.ssh_key_path}
              onChange={f('ssh_key_path')}
              placeholder="留空使用系统默认（/etc/xray-pilot/ssh/id_ed25519）"
            />
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

      {previewNode && (
        <PreviewConfigModal
          node={previewNode}
          activeProfiles={activeProfiles}
          onClose={() => setPreviewNode(null)}
        />
      )}
      {protocolNode && (
        <NodeProtocolsDrawer
          node={protocolNode}
          profiles={profilesData?.list ?? []}
          onPreview={() => {
            setPreviewNode(protocolNode)
            setProtocolNode(null)
          }}
          onClose={() => setProtocolNode(null)}
        />
      )}
      <OneClickInstallDialog
        open={installOpen}
        onClose={() => setInstallOpen(false)}
        onRegistered={() => invalidate()}
      />
    </PageShell>
  )
}

function PreviewConfigModal({ node, activeProfiles, onClose }: { node: Node; activeProfiles: InboundProfile[]; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const { data, isLoading, error } = useQuery({
    queryKey: ['previewConfig', node.id],
    queryFn: () => nodeApi.previewConfig(node.id).then((r) => r.data.data!),
    retry: false,
  })
  const { data: nodeKeys } = useQuery({
    queryKey: ['nodeKeys', node.id],
    queryFn: () => nodeApi.getKeys(node.id).then((r) => r.data.data ?? []),
    retry: false,
  })

  useEffect(() => {
    if (!copied) return
    const timer = setTimeout(() => setCopied(false), 1800)
    return () => clearTimeout(timer)
  }, [copied])

  const handleCopy = async () => {
    if (!data?.config) return
    const ok = await copyText(data.config)
    if (ok) {
      setCopied(true)
    } else {
      pushToast({
        title: '复制失败',
        description: '请手动选中预览内容并按 Ctrl+C 复制',
        variant: 'warning',
      })
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={`预览生成配置 · ${node.name}`}
      size="xl"
      footer={<Btn variant="secondary" onClick={onClose}>关闭</Btn>}
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <div className="space-y-1">
            <p className="text-xs text-soft">仅预览，不会同步至节点；`private_key` 已脱敏。</p>
            <ProtocolSummary nodeKeys={nodeKeys ?? []} activeProfiles={activeProfiles} />
          </div>
          <Btn variant="secondary" onClick={() => void handleCopy()} disabled={!data?.config}>
            <Copy className="h-4 w-4" />
            {copied ? '已复制' : '快速复制'}
          </Btn>
        </div>
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
            <pre className="max-h-[68vh] overflow-auto rounded-lg border border-[var(--border)] bg-slate-950 p-4 font-mono text-xs leading-5 text-slate-100 whitespace-pre">
              {data.config}
            </pre>
          </>
        )}
      </div>
    </Modal>
  )
}

function ProtocolSummary({ nodeKeys, activeProfiles }: { nodeKeys: NodeKey[]; activeProfiles: InboundProfile[] }) {
  const profileMap = new Map(activeProfiles.map((profile) => [profile.id, profile]))
  const matchedProfiles = nodeKeys
    .map((key) => profileMap.get(key.profile_id))
    .filter((item): item is InboundProfile => Boolean(item))

  if (matchedProfiles.length === 0) {
    return <p className="text-xs text-soft">当前节点未绑定任何协议，预览只会包含系统基础入站。</p>
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-xs text-soft">
      <span>当前绑定协议 {matchedProfiles.length} 个：</span>
      {matchedProfiles.map((profile) => (
        <Badge key={profile.id} label={profile.name} variant="blue" />
      ))}
    </div>
  )
}

function NodeProtocolsDrawer({
  node,
  profiles,
  onPreview,
  onClose,
}: {
  node: Node
  profiles: InboundProfile[]
  onPreview: () => void
  onClose: () => void
}) {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [activeProfileId, setActiveProfileId] = useState<number | null>(null)
  const [selectedProfileIds, setSelectedProfileIds] = useState<number[]>([])
  const [loadedByProfile, setLoadedByProfile] = useState<Record<number, { settings: string; port: number }>>({})
  const [draftByProfile, setDraftByProfile] = useState<Record<number, { settings: string; port: number }>>({})
  const [filter, setFilter] = useState('')
  const [msg, setMsg] = useState('')
  const [msgType, setMsgType] = useState<'ok' | 'err'>('ok')

  const { data: nodeKeys } = useQuery({
    queryKey: ['nodeKeys', node.id],
    queryFn: () => nodeApi.getKeys(node.id).then((r) => r.data.data ?? []),
  })

  const keyByProfileId = useMemo(() => {
    const m = new Map<number, NodeKey>()
    ;(nodeKeys ?? []).forEach((k) => m.set(k.profile_id, k))
    return m
  }, [nodeKeys])

  // 同步已绑协议的 loaded 数据
  useEffect(() => {
    if (!nodeKeys) return
    nodeKeys.forEach((k) => {
      if (loadedByProfile[k.profile_id] !== undefined) return
      setLoadedByProfile((prev) => ({
        ...prev,
        [k.profile_id]: {
          settings: stringifyNodeKeySettings(k.settings),
          port: k.port,
        },
      }))
    })
  }, [nodeKeys, loadedByProfile])

  const activeProfiles = profiles.filter((p) => p.active)
  const filteredProfiles = activeProfiles.filter((p) => {
    const kw = filter.trim().toLowerCase()
    if (!kw) return true
    return p.name.toLowerCase().includes(kw) || p.protocol.toLowerCase().includes(kw)
  })

  const activateProfile = (id: number) => {
    // 未绑定协议激活时，给一个默认草稿，方便编辑
    if (loadedByProfile[id] === undefined && draftByProfile[id] === undefined) {
      setDraftByProfile((prev) => ({ ...prev, [id]: { settings: '{}', port: 0 } }))
    }
    setActiveProfileId(id)
  }

  const updateDraft = (profileId: number, patch: Partial<{ settings: string; port: number }>) => {
    const base = draftByProfile[profileId] ?? loadedByProfile[profileId] ?? { settings: '{}', port: 0 }
    setDraftByProfile((prev) => ({ ...prev, [profileId]: { ...base, ...patch } }))
  }

  const dirtyProfileIds = Object.keys(draftByProfile)
    .map(Number)
    .filter((id) => {
      const d = draftByProfile[id]
      const l = loadedByProfile[id]
      return !l || d.settings !== l.settings || d.port !== l.port
    })

  const toggleProfileSelect = (id: number) => {
    setSelectedProfileIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  const activeProfile = activeProfileId !== null ? activeProfiles.find((p) => p.id === activeProfileId) : undefined
  const activeKey = activeProfileId !== null ? keyByProfileId.get(activeProfileId) : undefined
  const activeLocked = activeKey?.locked ?? false
  const activeDraft = activeProfileId !== null ? (draftByProfile[activeProfileId] ?? loadedByProfile[activeProfileId]) : undefined

  // 端口冲突前端实时检测（后端仍会硬校验兜底）
  const portConflict = useMemo(() => {
    if (!activeProfile || !activeDraft) return null
    const effPort = activeDraft.port > 0 ? activeDraft.port : activeProfile.port
    const transport = activeProfile.protocol === 'hysteria2' ? 'udp' : 'tcp'
    for (const k of nodeKeys ?? []) {
      if (k.profile_id === activeProfile.id) continue
      const other = profiles.find((p) => p.id === k.profile_id)
      if (!other) continue
      const otherTransport = other.protocol === 'hysteria2' ? 'udp' : 'tcp'
      const otherEffPort = k.port > 0 ? k.port : other.port
      if (otherTransport === transport && otherEffPort === effPort) {
        return `${transport.toUpperCase()} 端口 ${effPort} 已被协议 [${other.name}] 占用`
      }
    }
    return null
  }, [activeProfile, activeDraft, nodeKeys, profiles])

  const batchSave = useMutation({
    mutationFn: async () => {
      const targets = selectedProfileIds.length > 0 ? selectedProfileIds : activeProfileId !== null ? [activeProfileId] : []
      if (targets.length === 0) throw new Error('请先选择目标协议')
      const results: Array<{ id: number; ok: boolean; msg: string }> = []
      for (const id of targets) {
        const draft = draftByProfile[id] ?? loadedByProfile[id] ?? { settings: '{}', port: 0 }
        const settings = draft.settings || '{}'
        try {
          await nodeApi.upsertKey(node.id, id, settings, draft.port)
          results.push({ id, ok: true, msg: '已保存' })
        } catch (e) {
          results.push({ id, ok: false, msg: (e as Error).message })
        }
      }
      return results
    },
    onSuccess: (results) => {
      const ok = results.filter((r) => r.ok).length
      const failed = results.length - ok
      results.filter((r) => r.ok).forEach((r) => {
        const d = draftByProfile[r.id] ?? loadedByProfile[r.id] ?? { settings: '{}', port: 0 }
        setLoadedByProfile((prev) => ({ ...prev, [r.id]: d }))
        setDraftByProfile((prev) => {
          const n = { ...prev }
          delete n[r.id]
          return n
        })
      })
      setMsg(
        failed === 0
          ? `已保存 ${ok} 个协议`
          : `成功 ${ok}，失败 ${failed}。${results.filter((r) => !r.ok).map((r) => `#${r.id} ${r.msg}`).join('；')}`,
      )
      setMsgType(failed === 0 ? 'ok' : 'err')
      qc.invalidateQueries({ queryKey: ['nodes'] })
      qc.invalidateQueries({ queryKey: ['nodeKeys', node.id] })
      qc.invalidateQueries({ queryKey: ['node-keys-for-list'] })
    },
    onError: (e: Error) => {
      setMsg(`保存失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const keygen = useMutation({
    mutationFn: async () => {
      if (activeProfileId === null) throw new Error('请先选择协议')
      if (activeProfile?.protocol !== 'vless-reality') throw new Error('仅 VLESS+Reality 支持一键生成')
      const res = await nodeApi.keygen()
      const keys = res.data.data
      if (!keys) throw new Error('密钥生成失败')
      return JSON.stringify(
        { private_key: keys.private_key, public_key: keys.public_key, short_ids: generateShortIds(6) },
        null,
        2,
      )
    },
    onSuccess: (settings) => {
      if (activeProfileId !== null) updateDraft(activeProfileId, { settings })
      setMsg('密钥已生成（草稿），记得保存')
      setMsgType('ok')
    },
    onError: (e: Error) => {
      setMsg(`生成失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const fillTemplate = () => {
    if (selectedProfileIds.length > 0) {
      selectedProfileIds.forEach((id) => {
        const p = activeProfiles.find((x) => x.id === id)
        if (p) updateDraft(id, { settings: stringifyNodeKeySettings(p.settings) })
      })
      setMsg(`已为 ${selectedProfileIds.length} 个协议填入模板配置`)
      setMsgType('ok')
      return
    }
    if (activeProfileId === null || !activeProfile) return
    updateDraft(activeProfileId, { settings: stringifyNodeKeySettings(activeProfile.settings) })
    setMsg('已填入协议默认配置')
    setMsgType('ok')
  }

  const lockToggle = useMutation({
    mutationFn: (locked: boolean) => {
      if (activeProfileId === null) throw new Error('请先选择协议')
      return nodeApi.setKeyLock(node.id, activeProfileId, locked)
    },
    onSuccess: (_, locked) => {
      setMsg(locked ? '已锁定该节点协议' : '已解除锁定')
      setMsgType('ok')
      qc.invalidateQueries({ queryKey: ['nodeKeys', node.id] })
    },
    onError: (e: Error) => {
      setMsg(`操作失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const remove = useMutation({
    mutationFn: (profileId: number) => nodeApi.deleteKey(node.id, profileId),
    onSuccess: (_, profileId) => {
      setMsg('已移除该协议绑定')
      setMsgType('ok')
      setLoadedByProfile((prev) => {
        const n = { ...prev }
        delete n[profileId]
        return n
      })
      setDraftByProfile((prev) => {
        const n = { ...prev }
        delete n[profileId]
        return n
      })
      qc.invalidateQueries({ queryKey: ['nodes'] })
      qc.invalidateQueries({ queryKey: ['nodeKeys', node.id] })
      qc.invalidateQueries({ queryKey: ['node-keys-for-list'] })
    },
    onError: (e: Error) => {
      setMsg(`移除失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const targets = selectedProfileIds.length > 0 ? selectedProfileIds : activeProfileId !== null ? [activeProfileId] : []
  const unboundSelected = selectedProfileIds.filter((id) => !keyByProfileId.has(id))
  const saveLabel = selectedProfileIds.length > 1
    ? unboundSelected.length === selectedProfileIds.length
      ? `批量绑定 (${selectedProfileIds.length})`
      : `批量保存 (${selectedProfileIds.length})`
    : keyByProfileId.has(activeProfileId ?? -1) ? '保存协议' : '绑定协议'

  return (
    <Drawer
      open
      onClose={onClose}
      title={`节点协议管理 · ${node.name}`}
      description="左侧选择/多选协议；右侧编辑参数与端口。未绑协议直接保存即视为绑定。"
      width="xl"
      footer={
        <>
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
          <Btn variant="secondary" onClick={onPreview}>
            <FileCode className="h-4 w-4" />
            查看完整配置
          </Btn>
          <Btn
            loading={batchSave.isPending}
            onClick={() => batchSave.mutate()}
            disabled={targets.length === 0 || (selectedProfileIds.length === 0 && activeLocked) || portConflict !== null}
          >
            {saveLabel}
          </Btn>
        </>
      }
    >
      <div className="grid gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
        {/* 左侧：协议清单 */}
        <div className="space-y-3">
          <div className="rounded-xl border border-[var(--border)] bg-[var(--panel-muted)] p-3 text-xs text-soft">
            <div className="font-medium text-[var(--text)]">
              <span className="text-faint">#{node.id}</span>
              <span className="mx-2">{node.name}</span>
            </div>
            <div className="mt-1 truncate">
              {[node.ip, node.domain].filter(Boolean).join(' · ')}
            </div>
            <div className="mt-1 truncate">
              <span className="text-faint">地区</span> {node.region || '--'}
              <span className="mx-1.5 text-faint">·</span>
              <span className="text-faint">所有者</span> {node.owner || '--'}
            </div>
          </div>
          <input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="搜索协议 名称/类型"
            className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none"
          />
          <div className="flex items-center justify-between text-xs text-soft">
            <span>共 {filteredProfiles.length} 个</span>
            <button
              type="button"
              onClick={() => {
                const allVis = filteredProfiles.every((p) => selectedProfileIds.includes(p.id))
                setSelectedProfileIds(
                  allVis
                    ? selectedProfileIds.filter((id) => !filteredProfiles.some((p) => p.id === id))
                    : Array.from(new Set([...selectedProfileIds, ...filteredProfiles.map((p) => p.id)])),
                )
              }}
              className="text-[var(--accent)] hover:underline"
            >
              {filteredProfiles.every((p) => selectedProfileIds.includes(p.id)) && filteredProfiles.length > 0 ? '清空选择' : '全选可见'}
            </button>
          </div>
          <div className="max-h-[520px] space-y-1.5 overflow-y-auto pr-1">
            {filteredProfiles.map((p) => {
              const k = keyByProfileId.get(p.id)
              const isActive = activeProfileId === p.id
              const isSelected = selectedProfileIds.includes(p.id)
              const isDirty = dirtyProfileIds.includes(p.id)
              const effPort = k && k.port > 0 ? k.port : p.port
              return (
                <div
                  key={p.id}
                  className={`flex items-center gap-2 rounded-lg border px-2.5 py-2 text-sm transition ${
                    isActive
                      ? 'border-[var(--accent)]/40 bg-[var(--accent-soft)]'
                      : 'border-[var(--border)] bg-[var(--panel-strong)] hover:bg-[var(--panel-muted)]'
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={isSelected}
                    onChange={() => toggleProfileSelect(p.id)}
                    className="h-4 w-4 shrink-0 rounded border-[var(--border-strong)]"
                    aria-label={`选择 ${p.name}`}
                  />
                  <button type="button" onClick={() => activateProfile(p.id)} className="flex min-w-0 flex-1 flex-col gap-1 text-left">
                    <span className="flex items-center gap-1.5">
                      <Badge label={protocolLabel(p.protocol)} variant={protocolBadgeVariant(p.protocol)} />
                      {isDirty && <span className="h-1.5 w-1.5 rounded-full bg-amber-500" title="未保存" />}
                    </span>
                    <span className="flex items-center gap-1.5 truncate text-xs text-soft">
                      <span className="truncate font-medium text-[var(--text)]">{p.name}</span>
                      <span className="text-faint">·</span>
                      {k ? (
                        <>
                          <span>{k.port > 0 ? `端口 ${effPort}` : `默认 ${effPort}`}</span>
                          {k.locked && <span className="text-amber-500">🔒</span>}
                        </>
                      ) : (
                        <span className="text-faint">未绑</span>
                      )}
                    </span>
                  </button>
                  {k && <Badge label="已绑" variant="green" />}
                </div>
              )
            })}
          </div>
        </div>

        {/* 右侧：编辑区 */}
        <div className="space-y-4">
          {targets.length === 0 && (
            <div className="rounded-2xl border border-dashed border-[var(--border)] p-6 text-center text-sm text-soft">
              请从左侧选择协议（点击进入编辑，复选框用于批量操作）
            </div>
          )}

          {targets.length > 0 && (
            <>
              <FieldGroup
                title={
                  selectedProfileIds.length > 1
                    ? `批量目标 · ${selectedProfileIds.length} 个协议${unboundSelected.length > 0 ? `（含 ${unboundSelected.length} 个未绑）` : ''}`
                    : `当前协议 · ${activeProfile?.name ?? ''}${!keyByProfileId.has(activeProfileId ?? -1) ? '（未绑）' : ''}`
                }
                description={
                  selectedProfileIds.length > 1
                    ? '批量保存会对所有勾选的协议生效；未绑协议会以模板默认参数完成绑定（可保存后逐个微调）。'
                    : activeProfile
                      ? `协议默认端口 ${activeProfile.port}（${activeProfile.protocol === 'hysteria2' ? 'UDP' : 'TCP'}）。端口留空或填 0 即继承默认。`
                      : ''
                }
              >
                <div className="flex flex-wrap items-center gap-2">
                  {selectedProfileIds.length === 0 && activeProfile?.protocol === 'vless-reality' && (
                    <Btn variant="secondary" loading={keygen.isPending} onClick={() => keygen.mutate()} disabled={activeLocked}>
                      <Sparkles className="h-4 w-4" />
                      一键生成
                    </Btn>
                  )}
                  <Btn variant="secondary" onClick={fillTemplate} disabled={selectedProfileIds.length === 0 && activeLocked}>
                    填充模板
                  </Btn>
                  {selectedProfileIds.length === 0 && activeKey && (
                    <Btn variant="secondary" loading={lockToggle.isPending} onClick={() => lockToggle.mutate(!activeLocked)}>
                      {activeLocked ? '解除锁定' : '锁定协议'}
                    </Btn>
                  )}
                  {selectedProfileIds.length === 0 && activeKey && (
                    <Btn
                      variant="secondary"
                      disabled={activeLocked || remove.isPending}
                      onClick={async () => {
                        const ok = await confirm({
                          title: `移除协议「${activeProfile?.name}」？`,
                          description: '移除后该协议将不再参与节点配置生成，节点会重新进入待同步状态。',
                          confirmText: '移除协议',
                          cancelText: '取消',
                          tone: 'danger',
                        })
                        if (ok && activeProfileId !== null) remove.mutate(activeProfileId)
                      }}
                    >
                      移除协议
                    </Btn>
                  )}
                </div>
              </FieldGroup>

              {selectedProfileIds.length === 0 && activeProfile && activeDraft && (
                <FieldGroup title="参数与端口" description="保存空对象 `{}` 表示继承协议默认参数；端口填 0 表示继承协议模板端口。">
                  <Field
                    label={`监听端口（留空继承模板 ${activeProfile.port}）`}
                    value={String(activeDraft.port || '')}
                    onChange={(e) => updateDraft(activeProfileId!, { port: Number(e.target.value) || 0 })}
                    placeholder={`${activeProfile.port}`}
                    type="number"
                  />
                  {portConflict && <p className="text-xs text-rose-500">端口冲突：{portConflict}（保存会被后端拒绝）</p>}
                  <textarea
                    value={activeDraft.settings}
                    onChange={(e) => updateDraft(activeProfileId!, { settings: e.target.value })}
                    rows={12}
                    disabled={activeLocked}
                    className="min-h-[240px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                    placeholder='{} 表示继承协议默认参数'
                  />
                  {activeLocked && <p className="text-xs text-amber-500">当前节点协议已锁定，需先解锁才能修改、删除或重新生成。</p>}
                </FieldGroup>
              )}

              {selectedProfileIds.length > 1 && (
                <FieldGroup title="批量草稿预览" description="每个协议保有独立草稿；按钮（如填充模板）的效果会写入所有选中协议。">
                  <div className="space-y-2">
                    {selectedProfileIds.map((id) => {
                      const p = activeProfiles.find((x) => x.id === id)
                      const draft = draftByProfile[id] ?? loadedByProfile[id] ?? { settings: '{}', port: 0 }
                      const hasDraft = !!draftByProfile[id]
                      const bound = keyByProfileId.has(id)
                      return (
                        <div key={id} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-xs">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="truncate font-medium">{p?.name ?? `#${id}`}</span>
                            {p && <Badge label={protocolLabel(p.protocol)} variant={protocolBadgeVariant(p.protocol)} />}
                            {!bound && <Badge label="新绑定" variant="yellow" />}
                          </div>
                          <div className="flex items-center gap-2 text-soft">
                            <span>{draft.port > 0 ? `端口 ${draft.port}` : '默认端口'}</span>
                            <span>{draft.settings && draft.settings !== '{}' ? `${draft.settings.length} 字符` : '继承默认'}</span>
                            {hasDraft && <Badge label="未保存" variant="yellow" />}
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </FieldGroup>
              )}

              {msg && <p className={`text-sm ${msgType === 'ok' ? 'text-emerald-500' : 'text-rose-500'}`}>{msg}</p>}
            </>
          )}
        </div>
      </div>
    </Drawer>
  )
}

function stringifyNodeKeySettings(settings: unknown) {
  if (!settings) return '{}'
  if (typeof settings === 'string') {
    try {
      return JSON.stringify(JSON.parse(settings), null, 2)
    } catch {
      return settings
    }
  }
  return JSON.stringify(settings, null, 2)
}

