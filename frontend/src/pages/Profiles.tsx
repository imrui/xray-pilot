import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, PencilLine, Plus, Sparkles } from 'lucide-react'
import { nodeApi, profileApi } from '@/lib/api'
import type { InboundProfile, Protocol } from '@/types'
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

const protocolOptions = [
  { value: 'vless-reality', label: 'VLESS + Reality' },
  { value: 'vless-ws-tls', label: 'VLESS + WS + TLS' },
  { value: 'trojan', label: 'Trojan + TLS' },
  { value: 'hysteria2', label: 'Hysteria2' },
]

const protocolBadge: Record<Protocol, 'green' | 'blue' | 'yellow' | 'red'> = {
  'vless-reality': 'green',
  'vless-ws-tls': 'blue',
  trojan: 'yellow',
  hysteria2: 'red',
}

const defaultSettings: Record<Protocol, string> = {
  'vless-reality': JSON.stringify({ sni: 'www.microsoft.com', fingerprint: 'chrome', private_key: '', public_key: '', short_ids: [] }, null, 2),
  'vless-ws-tls': JSON.stringify({ host: 'cdn.example.com', path: '/ws' }, null, 2),
  trojan: JSON.stringify({ sni: 'example.com' }, null, 2),
  hysteria2: JSON.stringify({ sni: 'example.com', up_mbps: 100, down_mbps: 100 }, null, 2),
}

function generateShortIds(count = 6) {
  return Array.from({ length: count }, () => {
    const bytes = new Uint8Array(4)
    crypto.getRandomValues(bytes)
    return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
  })
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
  const [nodeId, setNodeId] = useState('')
  const [settings, setSettings] = useState('')
  const [msg, setMsg] = useState('')
  const [msgType, setMsgType] = useState<'ok' | 'err'>('ok')
  const qc = useQueryClient()

  const { data: nodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 100 }).then((r) => r.data.data?.list ?? []),
  })

  const { isFetching: loadingKey } = useQuery({
    queryKey: ['nodeKey', nodeId, profile.id],
    queryFn: async () => {
      const res = await nodeApi.getKeys(Number(nodeId))
      const keys = res.data.data ?? []
      const key = keys.find((k) => k.profile_id === profile.id)
      if (key) {
        setSettings(stringifySettings(key.settings))
      } else {
        setSettings('')
      }
      return key ?? null
    },
    enabled: !!nodeId,
  })

  const save = useMutation({
    mutationFn: () => nodeApi.upsertKey(Number(nodeId), profile.id, settings),
    onSuccess: () => {
      setMsg('已保存')
      setMsgType('ok')
      qc.invalidateQueries({ queryKey: ['nodes'] })
    },
    onError: (e: Error) => {
      setMsg(`保存失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const keygen = useMutation({
    mutationFn: async () => {
      const res = await nodeApi.keygen()
      const keys = res.data.data
      if (!keys) throw new Error('密钥生成失败')
      return JSON.stringify(
        {
          private_key: keys.private_key,
          public_key: keys.public_key,
          short_ids: generateShortIds(6),
        },
        null,
        2,
      )
    },
    onSuccess: (nextSettings) => {
      setSettings(nextSettings)
      setMsg('密钥已生成')
      setMsgType('ok')
    },
    onError: (e: Error) => {
      setMsg(`生成失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const fillTemplate = () => {
    setSettings(stringifySettings(profile.settings))
    setMsg('已填入协议默认配置')
    setMsgType('ok')
  }

  return (
    <Drawer
      open
      onClose={onClose}
      title={`配置节点密钥 · ${profile.name}`}
      description="先选择目标节点，再生成或覆写该节点对应协议的密钥材料。"
      footer={
        <>
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
          <Btn loading={save.isPending} onClick={() => save.mutate()} disabled={!nodeId || !settings}>保存密钥</Btn>
        </>
      }
      width="lg"
    >
      <div className="space-y-4">
        <FieldGroup title="目标节点" description="切换节点后会自动加载当前保存的密钥配置。">
          <SelectField
            label="目标节点"
            value={nodeId}
            onChange={setNodeId}
            options={(nodes ?? []).map((n) => ({ value: n.id, label: `${n.name} (${n.ip})` }))}
            placeholder="选择节点"
          />
        </FieldGroup>

        <FieldGroup title="密钥参数" description="支持一键生成、模板填充和手动编辑。保存空对象 `{}` 表示该节点继承协议默认参数。">
          <div className="flex items-center justify-between">
            <div className="text-xs text-soft">{loadingKey ? '正在加载当前密钥…' : '保持 JSON 结构正确后再保存；只保存后该协议才会绑定到当前节点。'}</div>
            <div className="flex gap-2">
              {profile.protocol === 'vless-reality' && (
                <Btn variant="secondary" loading={keygen.isPending} onClick={() => keygen.mutate()} disabled={!nodeId}>
                  <Sparkles className="h-4 w-4" />
                  一键生成
                </Btn>
              )}
              <Btn variant="secondary" onClick={fillTemplate}>填充模板</Btn>
            </div>
          </div>
          <textarea
            value={settings}
            onChange={(e) => setSettings(e.target.value)}
            rows={12}
            className="min-h-[260px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
            placeholder='{"private_key": "...", "public_key": "...", "short_ids": ["..."]}'
          />
          {msg && <p className={`text-sm ${msgType === 'ok' ? 'text-emerald-500' : 'text-rose-500'}`}>{msg}</p>}
        </FieldGroup>
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
      label: '协议配置',
      render: (p: InboundProfile) => (
        <div className="space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-semibold">{p.name}</span>
            <Badge label={protocolOptions.find((o) => o.value === p.protocol)?.label ?? p.protocol} variant={protocolBadge[p.protocol as Protocol] ?? 'gray'} />
          </div>
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
            配置密钥
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
