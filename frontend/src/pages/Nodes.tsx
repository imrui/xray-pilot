import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Copy, FileCode, Filter, PencilLine, Plus, RefreshCw, Search, Wifi } from 'lucide-react'
import { nodeApi, profileApi } from '@/lib/api'
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
  drifted: { label: '配置漂移', variant: 'yellow', tip: '配置已变更，需要重新同步' },
  failed: { label: '同步失败', variant: 'red', tip: '同步失败，建议先测试 SSH 连通性' },
  pending: { label: '待同步', variant: 'gray', tip: '节点尚未同步，请点击同步按钮' },
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
  name: '',
  region: '',
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
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'synced' | 'drifted' | 'failed' | 'pending'>('all')
  const [regionFilter, setRegionFilter] = useState<string>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['nodes', page, pageSize],
    queryFn: () => nodeApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
  })
  const { data: profilesData } = useQuery({
    queryKey: ['profiles-for-nodes'],
    queryFn: () => profileApi.list({ page: 1, page_size: 100 }).then((r) => r.data.data!),
  })

  const regions = useMemo(() => {
    return Array.from(new Set((data?.list ?? []).map((n) => n.region).filter(Boolean)))
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
          : '漂移节点已完成同步。',
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
    return (data?.list ?? []).filter((n) => {
      const keyword = search.trim().toLowerCase()
      const matchesKeyword =
        keyword === '' ||
        n.name.toLowerCase().includes(keyword) ||
        n.ip.toLowerCase().includes(keyword) ||
        n.domain.toLowerCase().includes(keyword) ||
        n.region.toLowerCase().includes(keyword)
      const matchesStatus = statusFilter === 'all' || n.sync_status === statusFilter
      const matchesRegion = regionFilter === 'all' || n.region === regionFilter
      return matchesKeyword && matchesStatus && matchesRegion
    })
  }, [data?.list, search, statusFilter, regionFilter])

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
              <option value="drifted">配置漂移</option>
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
        </div>

        <div className="flex items-center gap-2">
          <Btn variant="secondary" loading={syncDrifted.isPending} onClick={() => syncDrifted.mutate()}>
            <RefreshCw className="h-4 w-4" />
            同步漂移节点
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

      <SurfaceCard className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1240px] text-left text-sm">
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
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">在线用户</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">最后同步</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">启用</th>
                <th className="px-4 py-3 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-faint">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-soft">加载中…</td>
                </tr>
              ) : filteredNodes.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-soft">暂无节点数据</td>
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
                      <div className="font-semibold">{n.name}</div>
                      <div className="text-xs text-soft">{n.region || '未设置地区'}</div>
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="font-medium text-[var(--text)]">{n.ip}</div>
                      <div className="text-xs text-soft">{n.domain || '—'}</div>
                    </td>
                    <td className="px-4 py-3.5">
                      <Tooltip content={(statusMeta[n.sync_status] ?? statusMeta.pending).tip}>
                        <span tabIndex={0} className="inline-flex">
                          <Badge
                            label={(statusMeta[n.sync_status] ?? statusMeta.pending).label}
                            variant={(statusMeta[n.sync_status] ?? statusMeta.pending).variant}
                          />
                        </span>
                      </Tooltip>
                    </td>
                    <td className="px-4 py-3.5">
                      {n.group_names && n.group_names.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                          {n.group_names.map((groupName) => (
                            <Badge key={groupName} label={groupName} variant="blue" />
                          ))}
                        </div>
                      ) : (
                        <span className="text-soft">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5">{n.online_user_count}</td>
                    <td className="px-4 py-3.5 text-soft">{n.last_sync_at ? new Date(n.last_sync_at).toLocaleString('zh-CN') : '—'}</td>
                    <td className="px-4 py-3.5">
                      <Switch checked={n.active} onChange={() => toggle.mutate(n.id)} />
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex flex-col items-end gap-2">
                        <div className="flex items-center justify-end gap-2">
                          <button
                            onClick={() => sync.mutate(n.id)}
                            className={`inline-flex h-9 items-center gap-1 rounded-md border px-3 text-xs font-medium transition ${
                              n.sync_status === 'failed'
                                ? 'border-amber-500/40 bg-amber-500/12 text-amber-300 hover:bg-amber-500/18'
                                : 'border-[var(--border)] bg-[var(--panel-strong)] text-[var(--accent)] hover:bg-[var(--panel-muted)]'
                            }`}
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
              <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="例如：高可用入口节点" />
            </div>
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
          activeProfiles={(profilesData?.list ?? []).filter((item) => item.active)}
          onClose={() => setPreviewNode(null)}
        />
      )}
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
    await navigator.clipboard.writeText(data.config)
    setCopied(true)
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
