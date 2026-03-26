import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { nodeApi, profileApi } from '@/lib/api'
import type { InboundProfile, Protocol } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

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

function NodeKeyModal({ profile, onClose }: { profile: InboundProfile; onClose: () => void }) {
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
    <Modal
      open
      onClose={onClose}
      title={`配置节点密钥 · ${profile.name}`}
      footer={
        <>
          <Btn variant="secondary" onClick={onClose}>关闭</Btn>
          <Btn loading={save.isPending} onClick={() => save.mutate()} disabled={!nodeId || !settings}>保存密钥</Btn>
        </>
      }
    >
      <div className="space-y-4">
        <SelectField
          label="目标节点"
          value={nodeId}
          onChange={setNodeId}
          options={(nodes ?? []).map((n) => ({ value: n.id, label: `${n.name} (${n.ip})` }))}
          placeholder="选择节点"
        />
        <div>
          <div className="mb-2 flex items-center justify-between">
            <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">
              密钥 JSON
              {loadingKey && <span className="ml-2 text-[11px] normal-case tracking-normal text-soft">加载中…</span>}
            </label>
            <div className="flex gap-2">
              {profile.protocol === 'vless-reality' && (
                <Btn variant="secondary" loading={keygen.isPending} onClick={() => keygen.mutate()} disabled={!nodeId}>
                  一键生成
                </Btn>
              )}
              <button onClick={fillTemplate} className="text-xs text-[var(--accent)] transition hover:brightness-110">填充模板</button>
            </div>
          </div>
          <textarea
            value={settings}
            onChange={(e) => setSettings(e.target.value)}
            rows={8}
            className="min-h-[220px] w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
            placeholder='{"private_key": "...", "public_key": "...", "short_id": ""}'
          />
        </div>
        {msg && <p className={`text-sm ${msgType === 'ok' ? 'text-emerald-500' : 'text-rose-500'}`}>{msg}</p>}
      </div>
    </Modal>
  )
}

export default function Profiles() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; profile?: InboundProfile }>({ open: false })
  const [keyModal, setKeyModal] = useState<InboundProfile | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')

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
      return modal.profile ? profileApi.update(modal.profile.id, payload) : profileApi.create(payload)
    },
    onSuccess: () => {
      invalidate()
      closeModal()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({ mutationFn: (id: number) => profileApi.remove(id), onSuccess: invalidate })

  const openCreate = () => {
    setForm(emptyForm())
    setErr('')
    setModal({ open: true })
  }

  const openEdit = (p: InboundProfile) => {
    setForm({
      name: p.name,
      protocol: p.protocol,
      port: String(p.port),
      settings: p.settings ? (typeof p.settings === 'string' ? p.settings : JSON.stringify(p.settings, null, 2)) : '',
      active: p.active,
      remark: p.remark,
    })
    setErr('')
    setModal({ open: true, profile: p })
  }

  const closeModal = () => setModal({ open: false })

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

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: '配置名', render: (p: InboundProfile) => <span className="font-semibold">{p.name}</span> },
    {
      key: 'protocol',
      label: '协议',
      render: (p: InboundProfile) => <Badge label={protocolOptions.find((o) => o.value === p.protocol)?.label ?? p.protocol} variant={protocolBadge[p.protocol as Protocol] ?? 'gray'} />,
    },
    { key: 'port', label: '端口', render: (p: InboundProfile) => <span className="text-soft">{p.port}</span> },
    { key: 'active', label: '状态', render: (p: InboundProfile) => <Badge label={p.active ? '启用' : '禁用'} variant={p.active ? 'green' : 'gray'} /> },
    {
      key: 'actions',
      label: '操作',
      render: (p: InboundProfile) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(p)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
          <button onClick={() => setKeyModal(p)} className="text-xs text-[var(--accent)] transition hover:brightness-110">配置密钥</button>
          <button
            onClick={() => {
              if (confirm(`删除协议配置「${p.name}」？`)) remove.mutate(p.id)
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
        title="协议配置"
        description="管理 Reality、Trojan、Hysteria2 等接入协议，并维护每个节点的密钥与参数模板。"
        actions={<Btn onClick={openCreate}>新增协议</Btn>}
        stats={[
          { label: '总协议数', value: data?.total ?? 0 },
          { label: '默认端口', value: form.port || '443' },
          { label: '当前协议', value: protocolOptions.find((item) => item.value === form.protocol)?.label ?? 'VLESS + Reality' },
        ]}
      />

      <SurfaceCard className="p-4">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Modal
        open={modal.open}
        onClose={closeModal}
        title={modal.profile ? '编辑协议配置' : '新增协议配置'}
        size="lg"
        footer={
          <>
            <Btn variant="secondary" onClick={closeModal}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="space-y-4">
          <Field label="配置名 *" value={form.name} onChange={f('name')} placeholder="如：Reality 主协议" />
          <div className="grid gap-4 md:grid-cols-2">
            <SelectField label="协议类型 *" value={form.protocol} onChange={handleProtocolChange} options={protocolOptions} />
            <Field label="监听端口 *" value={form.port} onChange={f('port')} type="number" />
          </div>
          <div>
            <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">协议参数 (JSON)</label>
            <textarea
              value={form.settings}
              onChange={(e) => setForm((p) => ({ ...p, settings: e.target.value }))}
              rows={7}
              className="mt-1.5 min-h-[200px] w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 font-mono text-xs text-[var(--text)] focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
            />
          </div>
          <Field label="备注" value={form.remark} onChange={f('remark')} />
          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Modal>

      {keyModal && <NodeKeyModal profile={keyModal} onClose={() => setKeyModal(null)} />}
    </PageShell>
  )
}
