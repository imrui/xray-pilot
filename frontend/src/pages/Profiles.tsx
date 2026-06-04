import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, PencilLine, Plus, Sparkles } from 'lucide-react'
import { nodeApi, profileApi } from '@/lib/api'
import { generateShortIds } from '@/lib/keygen'
import { PROTOCOL_OPTIONS, protocolBadgeVariant, protocolLabel } from '@/lib/protocol'
import type { InboundProfile, NodeKey, Protocol } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn, FieldGroup } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { BulkBar, FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { useConfirm } from '@/components/ui/ConfirmProvider'

const DEFAULT_PAGE_SIZE = 10
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

const protocolOptions = PROTOCOL_OPTIONS

const defaultSettings: Record<Protocol, string> = {
  'vless-reality': JSON.stringify({ sni: 'www.microsoft.com', fingerprint: 'chrome', private_key: '', public_key: '', short_ids: [] }, null, 2),
  'vless-ws-tls': JSON.stringify({ host: 'cdn.example.com', path: '/ws' }, null, 2),
  trojan: JSON.stringify({ sni: 'example.com' }, null, 2),
  hysteria2: JSON.stringify({ sni: 'example.com', up_mbps: 100, down_mbps: 100 }, null, 2),
}

function stringifySettings(settings: unknown) {
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

interface FormState {
  name: string
  protocol: string
  port: string
  settings: string
  active: boolean
  remark: string
}

const emptyForm = (): FormState => ({
  name: '',
  protocol: 'vless-reality',
  port: '443',
  settings: defaultSettings['vless-reality'],
  active: true,
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

function NodeKeyDrawer({ profile, onClose }: { profile: InboundProfile; onClose: () => void }) {
  const [activeNodeId, setActiveNodeId] = useState<number | null>(null)
  const [selectedNodeIds, setSelectedNodeIds] = useState<number[]>([])
  const [loadedByNode, setLoadedByNode] = useState<Record<number, { settings: string; port: number }>>({})
  const [draftByNode, setDraftByNode] = useState<Record<number, { settings: string; port: number }>>({})
  const [filter, setFilter] = useState('')
  const [msg, setMsg] = useState('')
  const [msgType, setMsgType] = useState<'ok' | 'err'>('ok')
  const qc = useQueryClient()

  const { data: nodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 200 }).then((r) => r.data.data?.list ?? []),
  })
  const sortedNodes = [...(nodes ?? [])].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' }))
  const filteredNodes = sortedNodes.filter((n) => {
    const kw = filter.trim().toLowerCase()
    if (!kw) return true
    return n.name.toLowerCase().includes(kw) || n.ip.toLowerCase().includes(kw) || n.region.toLowerCase().includes(kw)
  })

  // 一次性加载所有节点的密钥概览，用于显示"已绑定 / 端口 / 锁定"标记
  const { data: keysByNode } = useQuery({
    queryKey: ['node-keys-overview', profile.id, (nodes ?? []).map((n) => n.id).join(',')],
    queryFn: async () => {
      const result: Record<number, NodeKey | null> = {}
      await Promise.all(
        (nodes ?? []).map(async (n) => {
          const res = await nodeApi.getKeys(n.id)
          const keys = res.data.data ?? []
          result[n.id] = keys.find((k) => k.profile_id === profile.id) ?? null
        }),
      )
      return result
    },
    enabled: (nodes?.length ?? 0) > 0,
  })

  // 切换激活节点时，自动加载草稿/已存值
  useEffect(() => {
    if (!activeNodeId || !keysByNode) return
    if (loadedByNode[activeNodeId] !== undefined) return
    const k = keysByNode[activeNodeId]
    const next = {
      settings: k ? stringifySettings(k.settings) : '',
      port: k?.port ?? 0,
    }
    setLoadedByNode((prev) => ({ ...prev, [activeNodeId]: next }))
  }, [activeNodeId, keysByNode, loadedByNode])

  const activeDraft = activeNodeId !== null ? (draftByNode[activeNodeId] ?? loadedByNode[activeNodeId]) : undefined
  const activeKey = activeNodeId !== null && keysByNode ? keysByNode[activeNodeId] : null
  const activeLocked = activeKey?.locked ?? false

  const updateDraft = (nodeId: number, patch: Partial<{ settings: string; port: number }>) => {
    const base = draftByNode[nodeId] ?? loadedByNode[nodeId] ?? { settings: '', port: 0 }
    setDraftByNode((prev) => ({ ...prev, [nodeId]: { ...base, ...patch } }))
  }

  const dirtyNodeIds = Object.keys(draftByNode)
    .map(Number)
    .filter((id) => {
      const d = draftByNode[id]
      const l = loadedByNode[id]
      return !l || d.settings !== l.settings || d.port !== l.port
    })

  const toggleNodeSelect = (id: number) => {
    setSelectedNodeIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }

  // 批量保存：对选中节点逐个 upsert，逐个反馈成功/失败
  const batchSave = useMutation({
    mutationFn: async () => {
      const targets = selectedNodeIds.length > 0 ? selectedNodeIds : activeNodeId !== null ? [activeNodeId] : []
      if (targets.length === 0) throw new Error('请先选择目标节点')
      const results: Array<{ id: number; ok: boolean; msg: string }> = []
      for (const id of targets) {
        const draft = draftByNode[id] ?? loadedByNode[id]
        if (!draft || !draft.settings) {
          results.push({ id, ok: false, msg: '密钥参数为空' })
          continue
        }
        try {
          await nodeApi.upsertKey(id, profile.id, draft.settings, draft.port)
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
      // 成功节点：把草稿落地为 loaded，清掉草稿
      results.filter((r) => r.ok).forEach((r) => {
        const d = draftByNode[r.id] ?? loadedByNode[r.id]
        if (d) {
          setLoadedByNode((prev) => ({ ...prev, [r.id]: d }))
          setDraftByNode((prev) => {
            const next = { ...prev }
            delete next[r.id]
            return next
          })
        }
      })
      setMsg(failed === 0 ? `已保存 ${ok} 个节点` : `保存完成：成功 ${ok}，失败 ${failed}。失败详情：${results.filter((r) => !r.ok).map((r) => `#${r.id} ${r.msg}`).join('；')}`)
      setMsgType(failed === 0 ? 'ok' : 'err')
      qc.invalidateQueries({ queryKey: ['nodes'] })
      qc.invalidateQueries({ queryKey: ['node-keys-overview'] })
    },
    onError: (e: Error) => {
      setMsg(`保存失败: ${e.message}`)
      setMsgType('err')
    },
  })

  // 批量一键生成（仅 vless-reality）：为每个选中节点独立生成密钥到草稿
  const batchKeygen = useMutation({
    mutationFn: async () => {
      const targets = selectedNodeIds.length > 0 ? selectedNodeIds : activeNodeId !== null ? [activeNodeId] : []
      if (targets.length === 0) throw new Error('请先选择目标节点')
      if (profile.protocol !== 'vless-reality') throw new Error('仅 VLESS+Reality 支持一键生成')
      const generated: Record<number, string> = {}
      for (const id of targets) {
        const res = await nodeApi.keygen()
        const keys = res.data.data
        if (!keys) throw new Error('密钥生成失败')
        generated[id] = JSON.stringify(
          { private_key: keys.private_key, public_key: keys.public_key, short_ids: generateShortIds(6) },
          null,
          2,
        )
      }
      return generated
    },
    onSuccess: (generated) => {
      Object.entries(generated).forEach(([idStr, settings]) => {
        const id = Number(idStr)
        const cur = draftByNode[id] ?? loadedByNode[id] ?? { settings: '', port: 0 }
        setDraftByNode((prev) => ({ ...prev, [id]: { ...cur, settings } }))
      })
      setMsg(`已为 ${Object.keys(generated).length} 个节点生成密钥草稿，记得保存`)
      setMsgType('ok')
    },
    onError: (e: Error) => {
      setMsg(`生成失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const fillTemplate = () => {
    const targets = selectedNodeIds.length > 0 ? selectedNodeIds : activeNodeId !== null ? [activeNodeId] : []
    if (targets.length === 0) {
      setMsg('请先选择目标节点')
      setMsgType('err')
      return
    }
    const tpl = stringifySettings(profile.settings)
    targets.forEach((id) => updateDraft(id, { settings: tpl }))
    setMsg(`已为 ${targets.length} 个节点填入协议默认配置`)
    setMsgType('ok')
  }

  const lockToggle = useMutation({
    mutationFn: (locked: boolean) => {
      if (activeNodeId === null) throw new Error('请先选择节点')
      return nodeApi.setKeyLock(activeNodeId, profile.id, locked)
    },
    onSuccess: (_, locked) => {
      setMsg(locked ? '已锁定该节点协议' : '已解除锁定')
      setMsgType('ok')
      qc.invalidateQueries({ queryKey: ['node-keys-overview'] })
    },
    onError: (e: Error) => {
      setMsg(`操作失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const targetsCount = selectedNodeIds.length > 0 ? selectedNodeIds : activeNodeId !== null ? [activeNodeId] : []
  const saveLabel = selectedNodeIds.length > 1 ? `批量保存 (${selectedNodeIds.length})` : '保存密钥'

  return (
    <Drawer
      open
      onClose={onClose}
      title={`配置节点密钥 · ${profile.name}`}
      description="左侧选择/多选目标节点；右侧编辑参数与端口。支持批量生成与批量保存。"
      footer={
        <>
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
          <Btn
            loading={batchSave.isPending}
            onClick={() => batchSave.mutate()}
            disabled={targetsCount.length === 0 || (selectedNodeIds.length === 0 && activeLocked)}
          >
            {saveLabel}
          </Btn>
        </>
      }
      width="xl"
    >
      <div className="grid h-full gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
        {/* 左侧节点列表 */}
        <div className="flex h-full min-h-0 flex-col space-y-3">
          <div>
            <input
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="搜索节点 名称/IP/地区"
              className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none"
            />
          </div>
          <div className="flex items-center justify-between text-xs text-soft">
            <span>共 {filteredNodes.length} 个</span>
            <button
              type="button"
              onClick={() => {
                const allVis = filteredNodes.every((n) => selectedNodeIds.includes(n.id))
                setSelectedNodeIds(allVis ? selectedNodeIds.filter((id) => !filteredNodes.some((n) => n.id === id)) : Array.from(new Set([...selectedNodeIds, ...filteredNodes.map((n) => n.id)])))
              }}
              className="text-[var(--accent)] hover:underline"
            >
              {filteredNodes.every((n) => selectedNodeIds.includes(n.id)) && filteredNodes.length > 0 ? '清空选择' : '全选可见'}
            </button>
          </div>
          <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto pr-1">
            {filteredNodes.map((n) => {
              const k = keysByNode?.[n.id]
              const isActive = activeNodeId === n.id
              const isSelected = selectedNodeIds.includes(n.id)
              const isDirty = dirtyNodeIds.includes(n.id)
              return (
                <div
                  key={n.id}
                  className={`flex items-center gap-2 rounded-lg border px-2.5 py-2 text-sm transition ${
                    isActive ? 'border-[var(--accent)]/40 bg-[var(--accent-soft)]' : 'border-[var(--border)] bg-[var(--panel-strong)] hover:bg-[var(--panel-muted)]'
                  }`}
                >
                  <input
                    type="checkbox"
                    checked={isSelected}
                    onChange={() => toggleNodeSelect(n.id)}
                    className="h-4 w-4 shrink-0 rounded border-[var(--border-strong)]"
                    aria-label={`选择 ${n.name}`}
                  />
                  <button type="button" onClick={() => setActiveNodeId(n.id)} className="flex min-w-0 flex-1 flex-col text-left">
                    <span className="flex items-center gap-1.5">
                      <span className="truncate font-medium">{n.name}</span>
                      {isDirty && <span className="h-1.5 w-1.5 rounded-full bg-amber-500" title="未保存" />}
                    </span>
                    <span className="flex min-w-0 items-center gap-1 text-xs text-soft">
                      <span className="min-w-0 flex-1 truncate">{n.ip}</span>
                      {k && (
                        <span className="flex shrink-0 items-center gap-1">
                          <span className="text-faint">·</span>
                          <span>{k.port > 0 ? `端口 ${k.port}` : `默认`}</span>
                          {k.locked && <span className="text-amber-500">🔒</span>}
                        </span>
                      )}
                    </span>
                  </button>
                  {k && <Badge label="已绑" variant="green" />}
                </div>
              )
            })}
          </div>
        </div>

        {/* 右侧编辑区 */}
        <div className="min-h-0 space-y-4 overflow-y-auto pr-1">
          {activeNodeId === null && selectedNodeIds.length === 0 && (
            <div className="rounded-2xl border border-dashed border-[var(--border)] p-6 text-center text-sm text-soft">
              请从左侧选择节点（点击进入编辑，复选框用于批量操作）
            </div>
          )}

          {(activeNodeId !== null || selectedNodeIds.length > 0) && (
            <>
              <FieldGroup
                title={selectedNodeIds.length > 1 ? `批量目标 · ${selectedNodeIds.length} 个节点` : `当前节点 · ${sortedNodes.find((n) => n.id === activeNodeId)?.name ?? ''}`}
                description={selectedNodeIds.length > 1 ? '批量操作会对所有勾选的节点生效；填充/生成会写入草稿，保存后才提交。' : `协议默认端口 ${profile.port}（${profile.protocol === 'hysteria2' ? 'UDP' : 'TCP'}）。端口留空或填 0 即继承默认。`}
              >
                <div className="flex flex-wrap items-center gap-2">
                  {profile.protocol === 'vless-reality' && (
                    <Btn
                      variant="secondary"
                      loading={batchKeygen.isPending}
                      onClick={() => batchKeygen.mutate()}
                      disabled={targetsCount.length === 0 || (selectedNodeIds.length === 0 && activeLocked)}
                    >
                      <Sparkles className="h-4 w-4" />
                      {selectedNodeIds.length > 1 ? `批量生成 (${selectedNodeIds.length})` : '一键生成'}
                    </Btn>
                  )}
                  <Btn variant="secondary" onClick={fillTemplate} disabled={targetsCount.length === 0 || (selectedNodeIds.length === 0 && activeLocked)}>填充模板</Btn>
                  {selectedNodeIds.length === 0 && activeKey && (
                    <Btn
                      variant="secondary"
                      loading={lockToggle.isPending}
                      onClick={() => lockToggle.mutate(!activeLocked)}
                    >
                      {activeLocked ? '解除锁定' : '锁定协议'}
                    </Btn>
                  )}
                </div>
              </FieldGroup>

              {selectedNodeIds.length === 0 && activeDraft && (
                <FieldGroup title="参数与端口" description="保存空对象 `{}` 表示继承协议默认参数；端口填 0 表示继承协议模板端口。">
                  <Field
                    label={`监听端口（留空继承模板 ${profile.port}）`}
                    value={String(activeDraft.port || '')}
                    onChange={(e) => updateDraft(activeNodeId!, { port: Number(e.target.value) || 0 })}
                    placeholder={`${profile.port}`}
                    type="number"
                  />
                  <textarea
                    value={activeDraft.settings}
                    onChange={(e) => updateDraft(activeNodeId!, { settings: e.target.value })}
                    rows={12}
                    disabled={activeLocked}
                    className="min-h-[240px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                    placeholder='{"private_key": "...", "public_key": "...", "short_ids": ["..."]}'
                  />
                  {activeLocked && <p className="text-xs text-amber-500">当前节点协议已锁定，需先解锁才能修改、删除或重新生成。</p>}
                </FieldGroup>
              )}

              {selectedNodeIds.length > 1 && (
                <FieldGroup title="批量草稿预览" description="每个节点保有独立草稿；上方按钮的效果会写入所有选中节点。">
                  <div className="space-y-2">
                    {selectedNodeIds.map((id) => {
                      const node = sortedNodes.find((n) => n.id === id)
                      const draft = draftByNode[id] ?? loadedByNode[id]
                      const hasDraft = !!draftByNode[id]
                      return (
                        <div key={id} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-xs">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="truncate font-medium">{node?.name ?? `#${id}`}</span>
                            <span className="text-soft">{node?.ip}</span>
                          </div>
                          <div className="flex items-center gap-2 text-soft">
                            {draft ? (
                              <>
                                <span>{draft.port > 0 ? `端口 ${draft.port}` : '默认端口'}</span>
                                <span>{draft.settings ? `${draft.settings.length} 字符` : '空'}</span>
                                {hasDraft && <Badge label="未保存" variant="yellow" />}
                              </>
                            ) : (
                              <span className="text-faint">未加载</span>
                            )}
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

export default function Profiles() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [drawer, setDrawer] = useState<{ open: boolean; profile?: InboundProfile }>({ open: false })
  const [keyDrawer, setKeyDrawer] = useState<InboundProfile | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [search, setSearch] = useState('')
  const [protocolFilter, setProtocolFilter] = useState<'all' | Protocol>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const generateProtocolDefaults = useMutation({
    mutationFn: async () => {
      const res = await nodeApi.keygen()
      const keys = res.data.data
      if (!keys) throw new Error('密钥生成失败')
      return JSON.stringify(
        {
          sni: 'www.microsoft.com',
          fingerprint: 'chrome',
          private_key: keys.private_key,
          public_key: keys.public_key,
          short_ids: generateShortIds(6),
        },
        null,
        2,
      )
    },
    onSuccess: (settings) => {
      setForm((prev) => ({ ...prev, settings }))
      setErr('')
    },
    onError: (e: Error) => setErr(e.message),
  })

  const { data, isLoading } = useQuery({
    queryKey: ['profiles', page, pageSize],
    queryFn: () => profileApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['profiles'] })

  const parseSettings = (s: string) => {
    try {
      return JSON.parse(s)
    } catch {
      return s
    }
  }

  const save = useMutation({
    mutationFn: () => {
      const payload = {
        name: form.name,
        protocol: form.protocol,
        port: Number(form.port) || 443,
        settings: parseSettings(form.settings),
        active: form.active,
        remark: form.remark,
      }
      return drawer.profile ? profileApi.update(drawer.profile.id, payload) : profileApi.create(payload)
    },
    onSuccess: () => {
      invalidate()
      closeDrawer()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({ mutationFn: (id: number) => profileApi.remove(id), onSuccess: invalidate })
  const toggle = useMutation({
    mutationFn: ({ id, active }: { id: number; active: boolean }) => profileApi.update(id, { active }),
    onSuccess: invalidate,
  })

  const openCreate = () => {
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true })
  }

  const openEdit = (p: InboundProfile) => {
    const next = {
      name: p.name,
      protocol: p.protocol,
      port: String(p.port),
      settings: p.settings ? (typeof p.settings === 'string' ? p.settings : JSON.stringify(p.settings, null, 2)) : '',
      active: p.active,
      remark: p.remark,
    }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true, profile: p })
  }

  const closeDrawer = () => setDrawer({ open: false })
  const confirmCloseDrawer = async () => {
    if (!dirty) return true
    return confirm({
      title: '放弃未保存的协议修改？',
      description: '当前协议参数和端口更改尚未保存，关闭后会丢失。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    })
  }

  const handleProtocolChange = (v: string) => {
    const proto = v as Protocol
    setForm((p) => ({
      ...p,
      protocol: v,
      settings: defaultSettings[proto] ?? '',
      port: proto === 'hysteria2' ? '2096' : '443',
    }))
  }

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const filteredProfiles = (data?.list ?? []).filter((p) => {
    const keyword = search.trim().toLowerCase()
    const matchesKeyword = keyword === '' || p.name.toLowerCase().includes(keyword) || p.protocol.toLowerCase().includes(keyword)
    const matchesProtocol = protocolFilter === 'all' || p.protocol === protocolFilter
    return matchesKeyword && matchesProtocol
  })
  const allVisibleSelected = filteredProfiles.length > 0 && filteredProfiles.every((p) => selectedIds.includes(p.id))
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm) && drawer.open

  const toggleSelect = (id: number) =>
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))

  const toggleSelectVisible = () =>
    setSelectedIds((prev) => {
      if (allVisibleSelected) return prev.filter((id) => !filteredProfiles.some((p) => p.id === id))
      return Array.from(new Set([...prev, ...filteredProfiles.map((p) => p.id)]))
    })

  const selectedProfiles = filteredProfiles.filter((p) => selectedIds.includes(p.id))

  const columns = [
    {
      key: 'select',
      label: (
        <input type="checkbox" checked={allVisibleSelected} onChange={toggleSelectVisible} aria-label="选择当前页可见协议" className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ) as unknown as string,
      render: (p: InboundProfile) => (
        <input type="checkbox" checked={selectedIds.includes(p.id)} onChange={() => toggleSelect(p.id)} aria-label={`选择协议 ${p.name}`} className="h-4 w-4 rounded border-[var(--border-strong)]" />
      ),
    },
    {
      key: 'name',
      label: '名称',
      render: (p: InboundProfile) => (
        <div className="space-y-1.5">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{p.name}</span>
            <span className="text-xs text-faint">#{p.id}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'meta',
      label: '类型',
      render: (p: InboundProfile) => (
        <div className="space-y-1.5">
          <button
            type="button"
            onClick={() => setKeyDrawer(p)}
            title="点击配置节点密钥与端口"
            className="inline-flex rounded-full transition hover:-translate-y-0.5 hover:shadow-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
          >
            <Badge label={protocolLabel(p.protocol)} variant={protocolBadgeVariant(p.protocol)} />
          </button>
          <div className="text-xs text-soft">监听端口 {p.port}</div>
        </div>
      ),
    },
    {
      key: 'active',
      label: '状态',
      render: (p: InboundProfile) => <Switch checked={p.active} onChange={(next) => toggle.mutate({ id: p.id, active: next })} />,
    },
    {
      key: 'actions',
      label: '操作',
      render: (p: InboundProfile) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => openEdit(p)}
            className="inline-flex h-9 items-center gap-1 rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-semibold text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
          >
            <PencilLine className="h-3.5 w-3.5" />
            编辑
          </button>
          <button
            onClick={() => setKeyDrawer(p)}
            className="inline-flex h-9 items-center gap-1 rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-semibold text-[var(--accent)] transition hover:bg-[var(--panel-muted)]"
          >
            <KeyRound className="h-3.5 w-3.5" />
            密钥 / 锁定
          </button>
          <ActionMenu
            items={[
              {
                label: '删除协议',
                danger: true,
                onSelect: async () => {
                  const ok = await confirm({
                    title: `删除协议配置「${p.name}」？`,
                    description: '该协议模板以及节点对应的引用关系会失效。',
                    confirmText: '删除协议',
                    cancelText: '取消',
                    tone: 'danger',
                  })
                  if (ok) remove.mutate(p.id)
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
        title="协议配置"
        description="将协议定义、节点密钥和 JSON 参数拆开处理，让高频维护动作保持清晰。"
        actions={
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增协议
          </Btn>
        }
      />

      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索协议名或协议类型"
        onSearchChange={setSearch}
        filters={
          <>
            <FilterChip active={protocolFilter === 'all'} onClick={() => setProtocolFilter('all')}>全部</FilterChip>
            {protocolOptions.map((option) => (
              <FilterChip key={option.value} active={protocolFilter === option.value} onClick={() => setProtocolFilter(option.value as Protocol)}>
                {option.label}
              </FilterChip>
            ))}
          </>
        }
        meta={`当前页匹配 ${filteredProfiles.length} / ${(data?.list ?? []).length} 条`}
        bulkBar={
          selectedProfiles.length > 0 ? (
            <BulkBar>
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="text-sm text-soft">已选择 <span className="font-semibold text-[var(--text)]">{selectedProfiles.length}</span> 个协议配置</div>
                <div className="flex flex-wrap gap-2">
                  <Btn
                    variant="danger"
                    onClick={async () => {
                      const ok = await confirm({
                        title: `删除已选中的 ${selectedProfiles.length} 个协议配置？`,
                        description: '批量删除会影响对应节点的协议分发配置。',
                        confirmText: '批量删除',
                        cancelText: '取消',
                        tone: 'danger',
                      })
                      if (!ok) return
                      await Promise.all(selectedProfiles.map((p) => profileApi.remove(p.id)))
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
          <Table columns={columns} data={filteredProfiles} loading={isLoading} />
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
            <div className="text-sm font-semibold">协议提示</div>
            <p className="mt-2 text-sm leading-6 text-soft">
              协议配置负责定义模板，节点密钥负责补齐每台机器的差异化参数。把这两层拆开后，维护复杂 Reality 或 TLS 场景会更稳。
            </p>
          </div>
          <div className="space-y-3">
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Template</div>
              <div className="mt-2 text-sm font-semibold">协议模板管理</div>
              <p className="mt-2 text-xs leading-5 text-soft">统一维护端口、SNI、传输参数等共性配置，减少节点端重复输入。</p>
            </div>
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-faint">Node Keys</div>
              <div className="mt-2 text-sm font-semibold">节点密钥独立配置</div>
              <p className="mt-2 text-xs leading-5 text-soft">将节点专属密钥放进单独抽屉管理，避免把敏感值混进大表单里。</p>
            </div>
          </div>
        </SurfaceCard>
      </div>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.profile ? `编辑协议 · ${drawer.profile.name}` : '新增协议'}
        description="保持列表在左侧可见，右侧专注于协议参数、端口和备注配置。"
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
          <FieldGroup title="协议定义" description="先选协议类型，再根据模板补充端口与参数。">
            <Field label="配置名 *" value={form.name} onChange={f('name')} placeholder="如：Reality 主协议" />
            <div className="grid gap-4 md:grid-cols-2">
              <SelectField label="协议类型 *" value={form.protocol} onChange={handleProtocolChange} options={protocolOptions} />
              <Field label="监听端口 *" value={form.port} onChange={f('port')} type="number" />
            </div>
          </FieldGroup>

          <FieldGroup title="协议参数" description="编辑 JSON 时保留格式化结构，切换协议会自动填入默认模板。">
            {form.protocol === 'vless-reality' && (
              <div className="flex items-center justify-between gap-3">
                <p className="text-xs text-soft">默认参数用于给所有未覆盖节点生成一份可直接使用的 Reality 配置。</p>
                <Btn variant="secondary" loading={generateProtocolDefaults.isPending} onClick={() => generateProtocolDefaults.mutate()}>
                  <Sparkles className="h-4 w-4" />
                  一键生成
                </Btn>
              </div>
            )}
            <textarea
              value={form.settings}
              onChange={(e) => setForm((p) => ({ ...p, settings: e.target.value }))}
              rows={12}
              className="min-h-[280px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
            />
          </FieldGroup>

          <FieldGroup title="附加信息">
            <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="记录用途、适配节点或特殊说明" />
          </FieldGroup>

          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Drawer>

      {keyDrawer && <NodeKeyDrawer profile={keyDrawer} onClose={() => setKeyDrawer(null)} />}
    </PageShell>
  )
}
