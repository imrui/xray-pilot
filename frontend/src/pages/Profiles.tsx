import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, PencilLine, Sparkles } from 'lucide-react'
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

const PAGE_SIZE = 20

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
        setSettings(JSON.stringify(typeof key.settings === 'string' ? JSON.parse(key.settings) : key.settings, null, 2))
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
    mutationFn: () => nodeApi.keygenForNode(Number(nodeId), profile.id),
    onSuccess: (res) => {
      const key = res.data.data
      if (key) {
        setSettings(JSON.stringify(typeof key.settings === 'string' ? JSON.parse(key.settings) : key.settings, null, 2))
      }
      setMsg('密钥已生成')
      setMsgType('ok')
      qc.invalidateQueries({ queryKey: ['nodeKey', nodeId, profile.id] })
    },
    onError: (e: Error) => {
      setMsg(`生成失败: ${e.message}`)
      setMsgType('err')
    },
  })

  const fillTemplate = () => {
    if (profile.protocol === 'vless-reality') {
      setSettings(JSON.stringify({ private_key: '', public_key: '', short_id: '' }, null, 2))
    } else {
      setSettings(JSON.stringify({ cert_path: '', key_path: '' }, null, 2))
    }
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

        <FieldGroup title="密钥参数" description="支持一键生成、模板填充和手动编辑。">
          <div className="flex items-center justify-between">
            <div className="text-xs text-soft">{loadingKey ? '正在加载当前密钥…' : '保持 JSON 结构正确后再保存。'}</div>
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
            className="min-h-[260px] w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
            placeholder='{"private_key": "...", "public_key": "...", "short_id": ""}'
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
  const [drawer, setDrawer] = useState<{ open: boolean; profile?: InboundProfile }>({ open: false })
  const [keyDrawer, setKeyDrawer] = useState<InboundProfile | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [search, setSearch] = useState('')
  const [protocolFilter, setProtocolFilter] = useState<'all' | Protocol>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['profiles', page],
    queryFn: () => profileApi.list({ page, page_size: PAGE_SIZE }).then((r) => r.data.data!),
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
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{p.name}</span>
            <Badge label={protocolOptions.find((o) => o.value === p.protocol)?.label ?? p.protocol} variant={protocolBadge[p.protocol as Protocol] ?? 'gray'} />
          </div>
          <div className="text-xs text-soft">监听端口 {p.port}</div>
        </div>
      ),
    },
    { key: 'active', label: '状态', render: (p: InboundProfile) => <Badge label={p.active ? '启用' : '禁用'} variant={p.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (p: InboundProfile) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => openEdit(p)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-soft transition hover:bg-[var(--panel)] hover:text-[var(--text)]"
          >
            <PencilLine className="h-3.5 w-3.5" />
            编辑
          </button>
          <button
            onClick={() => setKeyDrawer(p)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-[var(--accent)] transition hover:bg-[var(--panel)]"
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
        description="把常用编辑与密钥配置前置，复杂 JSON 参数放入抽屉处理，避免中断列表浏览。"
        actions={<Btn onClick={openCreate}>新增协议</Btn>}
        stats={[
          { label: '总协议数', value: data?.total ?? 0 },
          { label: '默认端口', value: form.port || '443' },
          { label: '当前协议', value: protocolOptions.find((item) => item.value === form.protocol)?.label ?? 'VLESS + Reality' },
        ]}
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

      <SurfaceCard className="p-4">
        <Table columns={columns} data={filteredProfiles} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

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
            <textarea
              value={form.settings}
              onChange={(e) => setForm((p) => ({ ...p, settings: e.target.value }))}
              rows={12}
              className="min-h-[280px] w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
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
