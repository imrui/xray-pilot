import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Copy, FileCode, Filter, Lock, PencilLine, Plus, RefreshCw, Sparkles, Terminal, Wifi } from 'lucide-react'
import { OneClickInstallDialog } from '@/components/OneClickInstallDialog'
import { dnsStatus, useDNSBindings } from '@/lib/dns'
import { NodeDNS } from '@/components/nodes/NodeDNS'
import { nodeApi, profileApi } from '@/lib/api'
import { fetchAllPages } from '@/lib/pagination'
import { useListSearch } from '@/lib/use-list-search'
import { SearchInput } from '@/components/ui/SearchInput'
import { HelpPopover } from '@/components/ui/HelpPopover'
import { Pagination } from '@/components/ui/Table'
import { generateShortIds } from '@/lib/keygen'
import { copyText } from '@/lib/clipboard'
import { protocolBadgeVariant, protocolLabel } from '@/lib/protocol'
import type { InboundProfile, Node, NodeKey, SyncStatus } from '@/types'
import { PreviewConfigModal } from '@/components/nodes/ConfigPreview'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup, SelectField, Switch } from '@/components/ui/Form'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/lib/confirm-context'
import { Tooltip } from '@/components/ui/Tooltip'
import { pushToast } from '@/lib/notify'
import { useSearchParams } from 'react-router-dom'
import { RoutingBindingDrawer } from '@/components/routing/BindingEditor'
import { NodeDiagnostics, SyncHistory } from '@/components/routing/SyncTools'
import { submitNodeSync, type SyncSubmission } from '@/lib/node-sync'
import { useRoutingOverview } from '@/lib/routing'
import { routingLabel, jobStatus } from '@/lib/routing-model'

const DEFAULT_PAGE_SIZE = 10
const configFailureMarkers = [
  'PREFLIGHT_FAILED', 'IPV6_UNAVAILABLE', 'VERSION_UNSUPPORTED', 'PATH_MISMATCH', 'ROLLED_BACK',
  'ROLLBACK_FAILED', 'AUTHORIZATION_STOPPED', 'HEALTH_FAILED', 'HEALTH_PROBE_UNAVAILABLE',
  '生成配置失败', '协议配置不完整', '节点未绑定协议', '无法读取当前配置', '目标文件已存在', '节点配置任务未完成', '恢复配置无效',
]

// 节点级日志级别选项；空值表示继承系统设置（Settings 里的 xray.log_level）
const NODE_LOG_LEVEL_OPTIONS = [
  { value: '', label: '继承系统设置' },
  { value: 'debug', label: 'debug（排障用，日志量大）' },
  { value: 'info', label: 'info' },
  { value: 'warning', label: 'warning' },
  { value: 'error', label: 'error' },
  { value: 'none', label: 'none（关闭）' },
]

const statusMeta: Record<SyncStatus, { label: string; variant: 'green' | 'yellow' | 'red' | 'gray'; tip: string }> = {
  synced: { label: '已同步', variant: 'green', tip: '配置已是最新' },
  drifted: { label: '配置有变更', variant: 'yellow', tip: '检测到配置变更或远端配置不一致，建议重新同步' },
  failed: { label: '同步失败', variant: 'red', tip: '可在同步记录中查看失败原因' },
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
  log_level: string
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
  log_level: '',
  remark: '',
})


export default function Nodes() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const affectedFilter = params.get('ids') ?? ''
  const routing = useRoutingOverview()
  const [routingNode, setRoutingNode] = useState<Node | null>(null)
  const [diagnosticNode, setDiagnosticNode] = useState<Pick<Node, 'id' | 'name'> | null>(null)
  const [history, setHistory] = useState<{ nodeID?: number; submitted?: SyncSubmission[] } | null>(null)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [drawer, setDrawer] = useState<{ open: boolean; node?: Node }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [previewNode, setPreviewNode] = useState<Node | null>(null)
  const [protocolNode, setProtocolNode] = useState<Node | null>(null)
  const { search, setSearch, keyword: debouncedSearch } = useListSearch()
  const statusFilter = params.get('sync_status') || 'all'
  const setStatusFilter = (value: string) => {
    setParams(previous => {
      const next = new URLSearchParams(previous)
      if (value === 'all') next.delete('sync_status')
      else next.set('sync_status', value)
      return next
    }, { replace: true })
  }
  const [healthFilter, setHealthFilter] = useState<'all' | 'healthy' | 'unhealthy' | 'unchecked'>('all')
  const [regionFilter, setRegionFilter] = useState('')
  const [ownerFilter, setOwnerFilter] = useState('')
  const [selectedIds, setSelectedIds] = useState<number[]>([])
  const [installOpen, setInstallOpen] = useState(false)
  // 更换服务器目标节点；非空时接入对话框进入更换模式
  const [replaceNode, setReplaceNode] = useState<Node | null>(null)

  const filterKey = JSON.stringify([debouncedSearch, statusFilter, healthFilter, regionFilter, ownerFilter, affectedFilter])
  const [pageFilterKey, setPageFilterKey] = useState(filterKey)
  if (pageFilterKey !== filterKey) {
    setPageFilterKey(filterKey)
    setPage(1)
    setSelectedIds([])
  }

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page, pageSize, debouncedSearch, statusFilter, healthFilter, regionFilter, ownerFilter, affectedFilter],
    refetchInterval: 5000,
    queryFn: () =>
      nodeApi
        .list({
          page,
          page_size: pageSize,
          keyword: debouncedSearch || undefined,
          sync_status: statusFilter === 'all' ? undefined : statusFilter,
          health: healthFilter === 'all' ? undefined : healthFilter,
          region: regionFilter || undefined,
          owner: ownerFilter || undefined,
          ids: affectedFilter || undefined,
        })
        .then((r) => r.data.data!),
  })
  const { data: bindings, isFetching: refreshingDNS, refetch: refreshDNS } = useDNSBindings()
  const dnsByNode = useMemo(() => bindings && new Map(bindings.map(b => [b.node_id, b])), [bindings])
  const { data: profiles = [] } = useQuery({
    queryKey: ['profiles-for-nodes'],
    queryFn: () => fetchAllPages((page, pageSize) =>
      profileApi.list({ page, page_size: pageSize }).then((r) => r.data.data!)),
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

  // 下拉候选值来自专用接口（全量），不能从当前页数据推导——否则翻页后选项残缺
  const { data: filterOptions } = useQuery({
    queryKey: ['nodes', 'filter-options'],
    queryFn: () => nodeApi.filterOptions().then((r) => r.data.data!),
  })
  const regions = new Set(filterOptions?.regions)
  const owners = new Set(filterOptions?.owners)
  if (regionFilter) regions.add(regionFilter)
  if (ownerFilter) owners.add(ownerFilter)

  const invalidate = () => Promise.all([qc.invalidateQueries({ queryKey: ['nodes'] }), qc.invalidateQueries({ queryKey: ['dns-bindings'] })])
  const nodeStatus = (n: Node) => {
    const row = routing.data?.nodes.find(row => row.id === n.id)
    if (row?.error) return { label: '路由冲突', tip: row.error, variant: 'red' as const }
    if (row?.job_status && ['pending', 'validating', 'staged', 'persisted', 'applying', 'verifying', 'unknown'].includes(row.job_status)) return { label: jobStatus[row.job_status], tip: '在同步记录中查看任务结果', variant: 'yellow' as const }
    if (n.sync_status !== 'synced' && row?.job_status && ['rolled_back', 'rollback_failed'].includes(row.job_status)) return { label: jobStatus[row.job_status], tip: '目标配置仍未同步，请查看记录并处理', variant: 'red' as const }
    return getConfiguredProfiles(n.id).length === 0 && !n.last_sync_at ? unconfiguredStatusMeta : (statusMeta[n.sync_status] ?? statusMeta.pending)
  }

  const save = useMutation({
    mutationFn: () => {
      // 所有文本字段去除首尾空格：域名/IP 带空格会生成错误的订阅地址，
      // 且客户端更新订阅时该节点会连不上（曾出现 " tw03.xxx" 线上事故）
      const payload = {
        name: form.name.trim(),
        region: form.region.trim(),
        owner: form.owner.trim(),
        ip: form.ip.trim(),
        domain: form.domain.trim(),
        ssh_user: form.ssh_user.trim(),
        ssh_key_path: form.ssh_key_path.trim(),
        log_level: form.log_level,
        remark: form.remark.trim(),
        ssh_port: Number(form.ssh_port) || 22,
      }
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
    mutationFn: (ids: number[]) => submitNodeSync(ids),
    onSuccess: (submitted, ids) => {
      setHistory({ nodeID: ids.length === 1 ? ids[0] : undefined, submitted })
      setSelectedIds([])
      invalidate()
    },
  })
  const syncDrifted = useMutation({
    mutationFn: async () => {
      const current = await routing.refetch()
      if (!current.data || current.error) throw new Error('无法读取待处理节点，请重试。')
      return submitNodeSync(current.data.nodes.filter(n => n.active && ['pending', 'drifted', 'failed'].includes(n.sync_status)).map(n => n.id))
    },
    onSuccess: submitted => {
      setHistory({ submitted })
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
      log_level: n.log_level ?? '',
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

  // 轻量查询返回后，以其为准，包括已解除的绑定。
  const filteredNodes = useMemo(() => {
    const nodes = (data?.list ?? []).map(n => dnsByNode ? { ...n, dns_binding: dnsByNode.get(n.id) } : n)
    return nodes.sort((a, b) =>
      a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' }),
    )
  }, [data?.list, dnsByNode])

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
  const activeProfiles = profiles.filter((item) => item.active)
  const activeProfileMap = new Map(activeProfiles.map((profile) => [profile.id, profile]))

  const renderFailureHint = (n: Node) => {
    if (n.sync_status !== 'failed' || routing.isPending) return null
    const summary = routing.data?.nodes.find(row => row.id === n.id)
    const reason = summary?.last_sync_error ?? ''
    const linkClass = 'inline-flex items-center gap-1 text-sm hover:underline'
    if (reason.includes('DRIFT_CONFLICT')) {
      return <button className={`${linkClass} text-[var(--warning)]`} onClick={() => setDiagnosticNode(n)}>
        <AlertTriangle className="h-3.5 w-3.5" />基线冲突，去核对
      </button>
    }
    const configFailure = configFailureMarkers.some(marker => reason.includes(marker))
    const errorAt = summary?.last_sync_error_at
    if (!configFailure && (!n.last_check_at || !n.last_check_ok || (errorAt && Date.parse(n.last_check_at) < Date.parse(errorAt)))) {
      return <button className={`${linkClass} text-[var(--warning)]`} onClick={() => testSSH.mutate(n.id)} disabled={testSSH.isPending && testSSH.variables === n.id}>
        建议先测试 SSH
      </button>
    }
    return <button className={`${linkClass} text-soft`} onClick={() => setHistory({ nodeID: n.id })}>查看失败原因</button>
  }

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

  const renderRoutingLink = (node: Node) => {
    const entry = routing.data?.nodes.find((row) => row.id === node.id)
    const label = entry
      ? routingLabel(entry, routing.data?.templates ?? [])
      : routing.error ? '加载失败，点击查看' : '加载中…'
    const isBasic = entry?.routing?.mode === 'legacy' && !entry.error
    return (
      <button
        className="mt-1 block max-w-xs text-left text-sm text-[var(--accent)] hover:underline"
        title={label}
        onClick={() => setRoutingNode(node)}
      >
        {isBasic ? '基础路由' : label}
      </button>
    )
  }

  const renderProtocolCell = (node: Node) => {
    const matchedProfiles = getConfiguredProfiles(node.id)

    if (matchedProfiles.length === 0) {
      return (
        <button
          type="button"
          onClick={() => setProtocolNode(node)}
          title="绑定协议后可生成节点配置"
          className="inline-flex h-7 items-center rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-2.5 text-sm font-medium text-soft transition hover:bg-[var(--panel-strong)] hover:text-[var(--text)]"
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
              className="inline-flex h-6 items-center rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-sm font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
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
        <div className="flex min-w-0 flex-1 flex-col gap-3 md:flex-row md:flex-wrap">
          <SearchInput
            value={search}
            onChange={setSearch}
            placeholder="搜索节点名称、地址、地区、所有者或备注"
            className="min-w-0 flex-1 md:min-w-60 md:max-w-[380px]"
            inputClassName="h-11"
          />
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select
              value={statusFilter}
              aria-label="节点同步状态"
              onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
              className="w-full bg-transparent text-sm outline-none"
            >
              <option value="all">全部状态</option>
              <option value="needs_sync">全部待处理</option>
              <option value="synced">已同步</option>
              <option value="drifted">配置有变更</option>
              <option value="failed">同步失败</option>
              <option value="pending">待同步</option>
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select
              value={healthFilter}
              aria-label="节点健康度"
              onChange={(e) => setHealthFilter(e.target.value as typeof healthFilter)}
              className="w-full bg-transparent text-sm outline-none"
            >
              <option value="all">全部健康度</option>
              <option value="healthy">健康</option>
              <option value="unhealthy">检测异常</option>
              <option value="unchecked">未检测</option>
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select aria-label="节点地区" value={regionFilter} onChange={(e) => setRegionFilter(e.target.value)} className="w-full bg-transparent text-sm outline-none">
              <option value="">全部地区</option>
              {Array.from(regions, (region) => (
                <option key={region} value={region}>
                  {region}
                </option>
              ))}
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select aria-label="节点所有者" value={ownerFilter} onChange={(e) => setOwnerFilter(e.target.value)} className="w-full bg-transparent text-sm outline-none">
              <option value="">全部所有者</option>
              {Array.from(owners, (owner) => (
                <option key={owner} value={owner}>
                  {owner}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="flex min-w-0 flex-wrap items-center gap-2 xl:justify-end [&>*]:shrink-0 [&>button]:whitespace-nowrap">
          <HelpPopover title="节点同步说明">
            <p>同步将已保存的协议、有效用户和路由一起下发。节点协议参数保存后才会参与配置生成。</p>
            <p>「同步全部待处理节点」会处理所有已启用且待同步、配置有变更或同步失败的节点，包括当前筛选之外的节点。</p>
          </HelpPopover>
          <Btn variant="secondary" onClick={() => setHistory({})}>同步记录</Btn>
          <Btn variant="secondary" loading={syncDrifted.isPending} disabled={sync.isPending} onClick={() => syncDrifted.mutate()}>
            <RefreshCw className="h-4 w-4" />
            同步全部待处理节点
          </Btn>
          <Btn variant="accent" onClick={() => setInstallOpen(true)}>
            <Terminal className="h-4 w-4" />
            一键接入
          </Btn>
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增节点
          </Btn>
        </div>
      </div>

      {selectedNodes.length > 0 && (
        <SurfaceCard className="p-3">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="text-sm text-soft">
              已选择 <span className="font-semibold text-[var(--text)]">{selectedNodes.length}</span> 个节点
            </div>
            <div className="flex flex-wrap gap-2">
              <Btn variant="secondary" loading={sync.isPending} disabled={syncDrifted.isPending} onClick={() => sync.mutate(selectedNodes.map(n => n.id))}>批量同步</Btn>
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
                    description: '这些节点的协议密钥、分组关系和路由绑定将一并删除，无法撤销。',
                    confirmText: '批量删除',
                    cancelText: '取消',
                    tone: 'danger',
                  })
                  if (!ok) return
                  // 串行删除：并发写会触发 SQLite 锁冲突，且逐个执行才能统计失败明细
                  let success = 0
                  const failures: string[] = []
                  for (const n of selectedNodes) {
                    try {
                      await nodeApi.remove(n.id)
                      success += 1
                    } catch (e) {
                      failures.push(`${n.name}: ${(e as Error).message}`)
                    }
                  }
                  pushToast(
                    failures.length === 0
                      ? { title: `已删除 ${success} 个节点`, variant: 'success' }
                      : {
                          title: `批量删除完成：成功 ${success}，失败 ${failures.length}`,
                          description: failures.join('；'),
                          variant: 'warning',
                        },
                  )
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
        <div className="relative overflow-x-auto overflow-y-visible">
          <table className="w-full min-w-[1140px] text-left text-sm">
            <thead className="border-b border-[var(--border)] bg-[var(--panel-muted)] text-soft [&_th]:whitespace-nowrap [&_th]:px-3 [&_th]:py-3 [&_th]:font-medium">
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
                <th className="min-w-32">节点</th>
                <th>连接地址</th>
                <th>同步状态</th>
                <th>分组 / 所有者</th>
                <th>在线用户 / Xray</th>
                <th>协议 / 目标路由</th>
                <th>启用</th>
                <th className="text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr>
                  <td colSpan={9} className="px-3 py-10 text-center text-soft">加载中…</td>
                </tr>
              ) : filteredNodes.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-3 py-10 text-center text-soft">暂无节点数据</td>
                </tr>
              ) : (
                filteredNodes.map((n) => (
                  <tr key={n.id} className="transition hover:bg-[var(--panel-muted)]">
                    <td className="px-3 py-3.5">
                      <input
                        type="checkbox"
                        checked={selectedIds.includes(n.id)}
                        onChange={() => toggleSelect(n.id)}
                        className="h-4 w-4 rounded border-[var(--border-strong)]"
                        aria-label={`选择节点 ${n.name}`}
                      />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="flex w-max max-w-48 items-center gap-2">
                        <HealthDot node={n} />
                        <span className="truncate font-semibold" title={n.name}>{n.name}</span>
                      </div>
                      <div className="mt-1 text-sm text-soft">
                        #{n.id}{n.region && ` · ${n.region}`}
                        {n.last_check_at && !n.last_check_ok && (
                          <span className="ml-1.5 text-[var(--danger)]">· 健康检测异常</span>
                        )}
                      </div>
                    </td>
                    <td className="px-3 py-3.5">
                      <CopyableAddress value={n.ip} strong />
                      {n.domain && <CopyableAddress value={n.domain} />}
                      {n.dns_binding && <span className="text-xs text-soft">DNS · {dnsStatus[n.dns_binding.status]}</span>}
                    </td>
                    <td className="px-3 py-3.5">
                      <Tooltip
                        content={nodeStatus(n).tip}
                        side="right"
                        className="max-w-[220px] whitespace-normal"
                      >
                        <span tabIndex={0} className="inline-flex">
                          <Badge
                            label={nodeStatus(n).label}
                            variant={nodeStatus(n).variant}
                          />
                        </span>
                      </Tooltip>
                      {n.last_sync_at && (
                        <time dateTime={n.last_sync_at} title="最后同步" className="mt-1 block text-sm text-soft">
                          <span className="sr-only">最后同步 </span>
                          {new Date(n.last_sync_at).toLocaleString('zh-CN')}
                        </time>
                      )}
                      {renderFailureHint(n)}
                    </td>
                    <td className="px-3 py-3.5">
                      {(n.group_names?.length || !n.owner) ? renderGroupCell(n.group_names) : null}
                      {n.owner && <div className="text-sm text-soft">所有者 {n.owner}</div>}
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="font-medium text-[var(--text)]">{n.online_user_count} 人</div>
                      <div className="mt-1 text-sm text-soft">{n.xray_version ? `Xray ${n.xray_version}` : '版本未知'}</div>
                    </td>
                    <td className="px-3 py-3.5">
                      {renderProtocolCell(n)}
                      {renderRoutingLink(n)}
                    </td>
                    <td className="px-3 py-3.5">
                      <Switch checked={n.active} onChange={() => toggle.mutate(n.id)} />
                    </td>
                    <td className="px-3 py-3.5">
                      <div className="flex flex-col items-end gap-2">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            onClick={() => sync.mutate([n.id])}
                            disabled={sync.isPending || syncDrifted.isPending || (getConfiguredProfiles(n.id).length === 0 && !n.last_sync_at)}
                            className={`inline-flex h-9 items-center gap-1 whitespace-nowrap rounded-md border px-3 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-45 ${
                              n.sync_status === 'failed'
                                ? 'border-[var(--warning-border)] bg-[var(--warning-soft)] text-[var(--warning)] hover:bg-[var(--warning-soft)]'
                                : 'border-[var(--border)] bg-[var(--panel-strong)] text-[var(--accent)] hover:bg-[var(--panel-muted)]'
                            }`}
                          >
                            <RefreshCw className={`h-3.5 w-3.5${sync.isPending && sync.variables?.includes(n.id) ? ' animate-spin' : ''}`} />
                            同步
                          </button>
                          <button
                            onClick={() => openEdit(n)}
                            className="inline-flex h-9 items-center gap-1 whitespace-nowrap rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                          >
                            <PencilLine className="h-3.5 w-3.5" />
                            编辑
                          </button>
                          <ActionMenu
                            items={[
                              { label: '路由配置', onSelect: () => setRoutingNode(n) },
                              { label: '同步记录', onSelect: () => setHistory({ nodeID: n.id }) },
                              { label: '诊断与恢复', onSelect: () => setDiagnosticNode(n) },
                              {
                                label: '测试 SSH',
                                onSelect: () => testSSH.mutate(n.id),
                                disabled: testSSH.isPending && testSSH.variables === n.id,
                              },
                              {
                                label: '更换服务器',
                                onSelect: () => {
                                  setReplaceNode(n)
                                  setInstallOpen(true)
                                },
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
                                    description: '节点的协议密钥、分组关系和路由绑定将一并删除，无法撤销。',
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
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </SurfaceCard>

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
        title={drawer.node ? `编辑节点 · ${drawer.node.name}` : '新增节点'}
        idleStatus=""
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
        <div className="space-y-4 [&_label]:text-sm">
          <FieldGroup title="节点标识">
            <Field label="节点名 *" value={form.name} onChange={f('name')} />
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="地区" value={form.region} onChange={f('region')} placeholder="如：香港" />
              <Field label="所有者" value={form.owner} onChange={f('owner')} placeholder="标识节点来源，如：供应商A" />
            </div>
            <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="例如：高可用入口节点" />
          </FieldGroup>

          <FieldGroup title="连接地址">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="服务器 IP *" value={form.ip} onChange={f('ip')} />
              <Field label="连接域名（留空使用 IP）" value={form.domain} onChange={f('domain')} placeholder="CDN 或中转域名" />
            </div>
          </FieldGroup>

          {drawer.node && <NodeDNS
            id={drawer.node.id}
            binding={dnsByNode ? dnsByNode.get(drawer.node.id) : drawer.node.dns_binding}
            changed={form.ip.trim() !== drawer.node.ip || form.domain.trim() !== (drawer.node.domain ?? '')}
            refreshing={refreshingDNS}
            onRefresh={() => void refreshDNS()}
          />}
          <FieldGroup title="SSH 连接">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="SSH 端口" value={form.ssh_port} onChange={f('ssh_port')} type="number" />
              <Field label="SSH 用户" value={form.ssh_user} onChange={f('ssh_user')} />
            </div>
            <Field
              label="SSH 密钥路径（留空继承系统设置）"
              value={form.ssh_key_path}
              onChange={f('ssh_key_path')}
              placeholder="/etc/xray-pilot/ssh/id_ed25519"
            />
          </FieldGroup>

          <FieldGroup title="Xray 运行参数">
            <SelectField
              label="节点日志级别"
              value={form.log_level}
              onChange={(v) => setForm((prev) => ({ ...prev, log_level: v }))}
              options={NODE_LOG_LEVEL_OPTIONS}
            />
          </FieldGroup>

          {drawer.node && (
            <FieldGroup title="连接与配置检查">
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

          {err && <p className="text-sm text-[var(--danger)]">{err}</p>}
        </div>
      </Drawer>

      {affectedFilter && <div className="text-sm text-soft">当前仅显示受影响节点。<Btn variant="secondary" onClick={() => setParams(previous => {
        const next = new URLSearchParams(previous)
        next.delete('ids')
        return next
      }, { replace: true })}>清除节点筛选</Btn></div>}
      {history && <SyncHistory {...history} onInspect={(id, name) => { setHistory(null); setDiagnosticNode({ id, name }) }} onClose={() => { setHistory(null); void invalidate() }} />}
      {diagnosticNode && <NodeDiagnostics key={diagnosticNode.id} {...diagnosticNode} lastSyncError={routing.data?.nodes.find(n => n.id === diagnosticNode.id)?.last_sync_error} onClose={() => setDiagnosticNode(null)} />}
      {routingNode && <RoutingBindingDrawer scope="node" id={routingNode.id} name={routingNode.name} onClose={() => setRoutingNode(null)} onPreview={() => { setPreviewNode(routingNode); setRoutingNode(null) }} />}
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
          profiles={profiles}
          onPreview={() => {
            setPreviewNode(protocolNode)
            setProtocolNode(null)
          }}
          onClose={() => setProtocolNode(null)}
        />
      )}
      <OneClickInstallDialog
        open={installOpen}
        onClose={() => {
          setInstallOpen(false)
          setReplaceNode(null)
        }}
        onRegistered={() => invalidate()}
        replaceNode={replaceNode}
      />
    </PageShell>
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
  // explicitActiveId 用户主动点击激活的协议；下面派生 activeProfileId 时还会被
  // "勾选恰好 1 个协议"的语义覆盖，让两种选中方式行为一致。
  const [explicitActiveId, setExplicitActiveId] = useState<number | null>(null)
  const [selectedProfileIds, setSelectedProfileIds] = useState<number[]>([])
  const [loadedByProfile, setLoadedByProfile] = useState<Record<number, { settings: string; port: number }>>({})
  const [draftByProfile, setDraftByProfile] = useState<Record<number, { settings: string; port: number }>>({})
  const [filter, setFilter] = useState('')
  const [msg, setMsg] = useState('')
  const [msgType, setMsgType] = useState<'ok' | 'err'>('ok')

  // 勾选 >1 个 = 批量；勾选恰好 1 个 = 视为激活该协议（让"复选框选 1 个"与
  // "点击条目激活"语义合并，避免出现"勾选了但右侧编辑区不显示"的死区）
  const isBatchMode = selectedProfileIds.length > 1
  const activeProfileId =
    isBatchMode
      ? null
      : selectedProfileIds.length === 1
        ? selectedProfileIds[0]
        : explicitActiveId

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
    return [p.name, p.protocol, protocolLabel(p.protocol)].some(value => value.toLowerCase().includes(kw))
  })

  const activateProfile = (id: number) => {
    // 未绑定协议激活时，给一个默认草稿，方便编辑
    if (loadedByProfile[id] === undefined && draftByProfile[id] === undefined) {
      setDraftByProfile((prev) => ({ ...prev, [id]: { settings: '{}', port: 0 } }))
    }
    setExplicitActiveId(id)
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
  // 勾选 1 个但还没生成过草稿时，draft/loaded 都 undefined；给个默认占位让
  // 端口/settings 编辑区能渲染出来（用户开始输入后会被 updateDraft 写入 state）
  const activeDraft = activeProfileId !== null
    ? (draftByProfile[activeProfileId] ?? loadedByProfile[activeProfileId] ?? { settings: '{}', port: 0 })
    : undefined

  // 端口冲突前端实时检测（后端仍会硬校验兜底）
  const portConflict = (() => {
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
  })()

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
      // 继承协议模板的 SNI / 指纹，作为节点级可覆盖的初值带出（留空则订阅回退模板默认）
      const tpl = (activeProfile?.settings ?? {}) as { sni?: string; fingerprint?: string }
      return JSON.stringify(
        {
          private_key: keys.private_key,
          public_key: keys.public_key,
          short_ids: generateShortIds(6),
          sni: tpl.sni ?? '',
          fingerprint: tpl.fingerprint ?? '',
        },
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
  const saveLabel = isBatchMode
    ? unboundSelected.length === selectedProfileIds.length
      ? `批量绑定 (${selectedProfileIds.length})`
      : `批量保存 (${selectedProfileIds.length})`
    : keyByProfileId.has(activeProfileId ?? -1) ? '保存协议' : '绑定协议'

  const protocolBusy = batchSave.isPending || keygen.isPending || lockToggle.isPending || remove.isPending
  const canCloseProtocols = async () => dirtyProfileIds.length === 0 || await confirm({ title: '放弃未保存的协议修改？', description: '未保存的协议参数不会出现在完整配置预览中。', confirmText: '放弃修改', cancelText: '继续编辑', tone: 'danger' })

  return (
    <Drawer
      open
      onClose={onClose}
      title={`节点协议管理 · ${node.name}`}
      dirty={dirtyProfileIds.length > 0}
      saving={protocolBusy}
      onBeforeClose={canCloseProtocols}
      description={<span className="wrap-anywhere">{[`#${node.id}`, node.ip, node.domain, node.region, node.owner && `所有者 ${node.owner}`].filter(Boolean).join(' · ')}</span>}
      idleStatus=""
      footer={
        <>
          <Btn variant="secondary" disabled={protocolBusy} onClick={() => void canCloseProtocols().then(ok => { if (ok) onClose() })}>关闭</Btn>
          <Btn variant="secondary" disabled={dirtyProfileIds.length > 0 || protocolBusy} title="仅预览已保存的完整配置，请先保存协议修改" onClick={onPreview}>
            <FileCode className="h-4 w-4" />
            查看完整配置
          </Btn>
          <Btn
            loading={batchSave.isPending}
            onClick={() => batchSave.mutate()}
            disabled={targets.length === 0 || (!isBatchMode && activeLocked) || portConflict !== null}
          >
            {saveLabel}
          </Btn>
        </>
      }
    >
      <div className="grid gap-4 md:grid-cols-[280px_minmax(0,1fr)] [&_label]:text-sm">
        {/* 左侧：协议清单 */}
        <div className="space-y-3">
          <SearchInput
            value={filter}
            onChange={setFilter}
            placeholder="搜索协议名称或类型"
          />
          <div className="flex items-center justify-between text-sm text-soft">
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
                    <span className="flex max-w-full items-center gap-1.5 font-semibold text-[var(--text)]">
                      <span className="truncate">{p.name}</span>
                      {isDirty && <span className="size-1.5 shrink-0 rounded-full bg-[var(--warning)]" title="未保存" />}
                      {k?.locked && <Lock className="size-3.5 shrink-0 text-[var(--warning)]" aria-label="已锁定" />}
                    </span>
                    <span className="flex flex-wrap items-center gap-1.5 text-sm text-soft">
                      {p.name !== protocolLabel(p.protocol) && <Badge label={protocolLabel(p.protocol)} variant={protocolBadgeVariant(p.protocol)} />}
                      <span>{k ? (k.port > 0 ? `端口 ${effPort}` : `默认端口 ${effPort}`) : '未绑定'}</span>
                    </span>
                  </button>
                  {k && <Badge label="已绑定" variant="green" />}
                </div>
              )
            })}
          </div>
        </div>

        {/* 右侧：编辑区 */}
        <div className="space-y-4">
          {targets.length === 0 && (
            <div className="rounded-2xl border border-dashed border-[var(--border)] p-6 text-center text-sm text-soft">
              选择协议以编辑，勾选多个协议可批量绑定或保存。
            </div>
          )}

          {targets.length > 0 && (
            <>
              <FieldGroup
                title={
                  isBatchMode
                    ? `批量目标 · ${selectedProfileIds.length} 个协议${unboundSelected.length > 0 ? `（含 ${unboundSelected.length} 个未绑）` : ''}`
                    : `当前协议 · ${activeProfile?.name ?? ''}${!keyByProfileId.has(activeProfileId ?? -1) ? '（未绑）' : ''}`
                }
                description={
                  isBatchMode
                    ? '保存时将绑定尚未绑定的协议，并应用每个协议的草稿。'
                    : undefined
                }
              >
                <div className="flex flex-wrap items-center gap-2">
                  {!isBatchMode && activeProfile?.protocol === 'vless-reality' && (
                    <Btn variant="secondary" loading={keygen.isPending} onClick={() => keygen.mutate()} disabled={activeLocked}>
                      <Sparkles className="h-4 w-4" />
                      一键生成
                    </Btn>
                  )}
                  <Btn variant="secondary" onClick={fillTemplate} disabled={!isBatchMode && activeLocked}>
                    填充模板
                  </Btn>
                  {!isBatchMode && activeKey && (
                    <Btn variant="secondary" loading={lockToggle.isPending} onClick={() => lockToggle.mutate(!activeLocked)}>
                      {activeLocked ? '解除锁定' : '锁定协议'}
                    </Btn>
                  )}
                  {!isBatchMode && activeKey && (
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

              {!isBatchMode && activeProfile && activeDraft && (
                <FieldGroup title="节点覆盖">
                  <Field
                    label={`${activeProfile.protocol === 'hysteria2' ? 'UDP' : 'TCP'} 监听端口（留空或填 0 继承模板 ${activeProfile.port}）`}
                    value={String(activeDraft.port || '')}
                    onChange={(e) => updateDraft(activeProfileId!, { port: Number(e.target.value) || 0 })}
                    placeholder={`${activeProfile.port}`}
                    type="number"
                  />
                  {portConflict && <p role="alert" className="text-sm text-[var(--danger)]">端口冲突：{portConflict}</p>}
                  <label htmlFor="node-protocol-settings" className="block text-sm font-medium text-soft">协议参数（{'{}'} 继承模板）</label>
                  <textarea
                    id="node-protocol-settings"
                    value={activeDraft.settings}
                    onChange={(e) => updateDraft(activeProfileId!, { settings: e.target.value })}
                    rows={12}
                    disabled={activeLocked}
                    className="min-h-[240px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-3 font-mono text-sm text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                    placeholder="{}"
                  />
                  {activeLocked && <p className="text-sm text-[var(--warning)]">协议已锁定。修改参数、生成密钥或移除绑定前，请先解除锁定。</p>}
                </FieldGroup>
              )}

              {isBatchMode && (
                <FieldGroup title="批量草稿预览">
                  <div className="space-y-2">
                    {selectedProfileIds.map((id) => {
                      const p = activeProfiles.find((x) => x.id === id)
                      const draft = draftByProfile[id] ?? loadedByProfile[id] ?? { settings: '{}', port: 0 }
                      const hasDraft = !!draftByProfile[id]
                      const bound = keyByProfileId.has(id)
                      return (
                        <div key={id} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-sm">
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

              {msg && <p className={`text-sm ${msgType === 'ok' ? 'text-[var(--success)]' : 'text-[var(--danger)]'}`}>{msg}</p>}
            </>
          )}
        </div>
      </div>
    </Drawer>
  )
}

// HealthDot 节点健康状态圆点，语义与仪表盘"节点健康"卡片一致：
// 绿 = 最近一次检测通过；红 = 检测过但失败；灰 = 从未检测过
function HealthDot({ node }: { node: Node }) {
  const meta = !node.last_check_at
    ? { color: 'bg-[var(--border-strong)]', label: '未检测', detail: '尚未进行健康检测，可点击「测试 SSH」主动触发' }
    : node.last_check_ok
      ? {
          color: 'bg-[var(--success)]',
          label: '健康',
          detail: `延迟 ${node.last_latency_ms > 0 ? `${node.last_latency_ms}ms` : '—'} · 最后检测 ${new Date(node.last_check_at).toLocaleString('zh-CN')}`,
        }
      : {
          color: 'bg-[var(--danger)]',
          label: '检测异常',
          detail: `最后检测 ${new Date(node.last_check_at).toLocaleString('zh-CN')} 失败，建议测试 SSH 排查`,
        }
  return (
    <Tooltip content={`健康检测：${meta.label} · ${meta.detail}`} side="right" className="max-w-[280px] whitespace-normal">
      <span tabIndex={0} className={`inline-flex h-2.5 w-2.5 shrink-0 rounded-full ${meta.color}`} aria-label={`健康检测：${meta.label}`} />
    </Tooltip>
  )
}

// CopyableAddress 连接地址单元格的一行（IP 或域名），带快速复制按钮
function CopyableAddress({ value, strong }: { value: string; strong?: boolean }) {
  if (!value) return <div className="text-sm text-soft">—</div>
  return (
    <div
      className={`flex w-max max-w-56 items-center gap-1.5 ${
        strong ? 'font-medium text-[var(--text)]' : 'text-sm text-soft'
      }`}
    >
      <span className="truncate" title={value}>{value}</span>
      <button
        type="button"
        title={`复制 ${value}`}
        aria-label={`复制 ${value}`}
        onClick={async () => {
          const ok = await copyText(value)
          pushToast(
            ok
              ? { title: '已复制', description: value, variant: 'success' }
              : { title: '复制失败', description: '请手动选中复制', variant: 'warning' },
          )
        }}
        className="shrink-0 text-faint transition hover:text-[var(--accent)]"
      >
        <Copy className="h-3.5 w-3.5" />
      </button>
    </div>
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

