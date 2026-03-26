import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { profileApi, nodeApi } from '@/lib/api'
import type { InboundProfile, Protocol } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn } from '@/components/ui/Form'

const PAGE_SIZE = 20

const protocolOptions = [
  { value: 'vless-reality', label: 'VLESS + Reality' },
  { value: 'vless-ws-tls', label: 'VLESS + WS + TLS' },
  { value: 'trojan',       label: 'Trojan + TLS' },
  { value: 'hysteria2',    label: 'Hysteria2' },
]

const protocolBadge: Record<Protocol, string> = {
  'vless-reality': 'green',
  'vless-ws-tls':  'blue',
  'trojan':        'yellow',
  'hysteria2':     'red',
}

// 不同协议的默认 settings JSON
// vless-reality 含可选默认密钥字段；各节点可通过「配置密钥」→「一键生成」覆盖
const defaultSettings: Record<Protocol, string> = {
  'vless-reality': JSON.stringify({
    sni: 'www.microsoft.com',
    fingerprint: 'chrome',
    private_key: '',
    public_key: '',
    short_ids: [],
  }, null, 2),
  'vless-ws-tls':  JSON.stringify({ host: 'cdn.example.com', path: '/ws' }, null, 2),
  'trojan':        JSON.stringify({ sni: 'example.com' }, null, 2),
  'hysteria2':     JSON.stringify({ sni: 'example.com', up_mbps: 100, down_mbps: 100 }, null, 2),
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
  name: '', protocol: 'vless-reality', port: '443',
  settings: defaultSettings['vless-reality'], active: true, remark: '',
})

// NodeKeyModal: 管理单个节点对某协议的密钥材料
function NodeKeyModal({
  profile, onClose,
}: { profile: InboundProfile; onClose: () => void }) {
  const [nodeId, setNodeId] = useState('')
  const [settings, setSettings] = useState('')
  const [msg, setMsg] = useState('')
  const [msgType, setMsgType] = useState<'ok' | 'err'>('ok')
  const qc = useQueryClient()

  const { data: nodes } = useQuery({
    queryKey: ['nodes-all'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 100 }).then(r => r.data.data?.list ?? []),
  })

  // 切换节点时自动加载该节点已有密钥
  const { isFetching: loadingKey } = useQuery({
    queryKey: ['nodeKey', nodeId, profile.id],
    queryFn: async () => {
      const res = await nodeApi.getKeys(Number(nodeId))
      const keys = res.data.data ?? []
      const key = keys.find(k => k.profile_id === profile.id)
      if (key) {
        setSettings(JSON.stringify(
          typeof key.settings === 'string' ? JSON.parse(key.settings) : key.settings,
          null, 2
        ))
      } else {
        setSettings('')
      }
      return key ?? null
    },
    enabled: !!nodeId,
  })

  const save = useMutation({
    mutationFn: () => nodeApi.upsertKey(Number(nodeId), profile.id, settings),
    onSuccess: () => { setMsg('已保存'); setMsgType('ok'); qc.invalidateQueries({ queryKey: ['nodes'] }) },
    onError: (e: Error) => { setMsg('保存失败: ' + e.message); setMsgType('err') },
  })

  const keygen = useMutation({
    mutationFn: () => nodeApi.keygenForNode(Number(nodeId), profile.id),
    onSuccess: (res) => {
      const key = res.data.data
      if (key) {
        setSettings(JSON.stringify(
          typeof key.settings === 'string' ? JSON.parse(key.settings) : key.settings,
          null, 2
        ))
      }
      setMsg('密钥已生成'); setMsgType('ok')
      qc.invalidateQueries({ queryKey: ['nodeKey', nodeId, profile.id] })
    },
    onError: (e: Error) => { setMsg('生成失败: ' + e.message); setMsgType('err') },
  })

  // 填充空模板（手动输入用）
  const fillTemplate = () => {
    if (profile.protocol === 'vless-reality') {
      setSettings(JSON.stringify({ private_key: '', public_key: '', short_id: '' }, null, 2))
    } else {
      setSettings(JSON.stringify({ cert_path: '', key_path: '' }, null, 2))
    }
  }

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg">
        <div className="flex items-center justify-between p-6 border-b border-gray-200">
          <h3 className="text-lg font-semibold text-gray-900">配置节点密钥 · {profile.name}</h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">✕</button>
        </div>
        <div className="p-6 space-y-3">
          <SelectField
            label="目标节点"
            value={nodeId}
            onChange={setNodeId}
            options={(nodes ?? []).map(n => ({ value: n.id, label: `${n.name} (${n.ip})` }))}
            placeholder="选择节点"
          />
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-sm font-medium text-gray-700">
                密钥 JSON
                {loadingKey && <span className="ml-2 text-xs text-slate-400">加载中…</span>}
              </label>
              <div className="flex gap-2">
                {profile.protocol === 'vless-reality' && (
                  <Btn
                    variant="secondary"
                    loading={keygen.isPending}
                    onClick={() => keygen.mutate()}
                    disabled={!nodeId}
                  >
                    一键生成
                  </Btn>
                )}
                <button onClick={fillTemplate} className="text-xs text-indigo-500 hover:text-indigo-700">填充模板</button>
              </div>
            </div>
            <textarea
              value={settings}
              onChange={e => setSettings(e.target.value)}
              rows={8}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
              placeholder='{"private_key": "...", "public_key": "...", "short_id": ""}'
            />
          </div>
          {msg && (
            <p className={`text-sm ${msgType === 'ok' ? 'text-green-600' : 'text-red-500'}`}>{msg}</p>
          )}
          <div className="flex gap-2 justify-end">
            <Btn variant="secondary" onClick={onClose}>关闭</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()} disabled={!nodeId || !settings}>保存密钥</Btn>
          </div>
        </div>
      </div>
    </div>
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
    queryFn: () => profileApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['profiles'] })

  // settings 文本框内容是字符串，发送时解析为 JSON object，避免后端二次编码问题
  const parseSettings = (s: string) => {
    try { return JSON.parse(s) } catch { return s }
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
      return modal.profile
        ? profileApi.update(modal.profile.id, payload)
        : profileApi.create(payload)
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: Error) => setErr(e.message),
  })

  const remove = useMutation({ mutationFn: (id: number) => profileApi.remove(id), onSuccess: invalidate })

  const openCreate = () => { setForm(emptyForm()); setErr(''); setModal({ open: true }) }
  const openEdit = (p: InboundProfile) => {
    setForm({
      name: p.name, protocol: p.protocol, port: String(p.port),
      settings: p.settings
        ? (typeof p.settings === 'string' ? p.settings : JSON.stringify(p.settings, null, 2))
        : '',
      active: p.active, remark: p.remark,
    })
    setErr('')
    setModal({ open: true, profile: p })
  }
  const closeModal = () => setModal({ open: false })

  const handleProtocolChange = (v: string) => {
    const proto = v as Protocol
    setForm(p => ({
      ...p,
      protocol: v,
      settings: defaultSettings[proto] ?? '',
      port: proto === 'hysteria2' ? '2096' : proto === 'vless-ws-tls' ? '443' : '443',
    }))
  }

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [k]: e.target.value }))

  const columns = [
    { key: 'id', label: 'ID' },
    {
      key: 'name', label: '配置名',
      render: (p: InboundProfile) => <span className="font-medium text-slate-900">{p.name}</span>,
    },
    {
      key: 'protocol', label: '协议',
      render: (p: InboundProfile) => (
        <Badge
          label={protocolOptions.find(o => o.value === p.protocol)?.label ?? p.protocol}
          variant={protocolBadge[p.protocol as Protocol] as 'green' | 'blue' | 'yellow' | 'red' ?? 'gray'}
        />
      ),
    },
    { key: 'port', label: '端口', render: (p: InboundProfile) => <span className="text-slate-600">{p.port}</span> },
    { key: 'active', label: '状态', render: (p: InboundProfile) => <Badge label={p.active ? '启用' : '禁用'} variant={p.active ? 'green' : 'gray'} /> },
    {
      key: 'actions', label: '操作',
      render: (p: InboundProfile) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(p)} className="text-xs text-slate-500 hover:text-slate-900">编辑</button>
          <button onClick={() => setKeyModal(p)} className="text-xs text-indigo-500 hover:text-indigo-700">配置密钥</button>
          <button
            onClick={() => { if (confirm(`删除协议配置「${p.name}」？`)) remove.mutate(p.id) }}
            className="text-xs text-red-500 hover:text-red-700"
          >删除</button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">协议配置</h1>
          <p className="text-sm text-slate-500 mt-0.5">管理节点接入协议（VLESS Reality / Trojan / Hysteria2 等）· 共 {data?.total ?? 0} 个</p>
        </div>
        <Btn onClick={openCreate}>新增协议</Btn>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </div>

      {/* 新增/编辑 Modal */}
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
        <div className="space-y-3">
          <Field label="配置名 *" value={form.name} onChange={f('name')} placeholder="如：Reality 主协议" />
          <div className="grid grid-cols-2 gap-3">
            <SelectField
              label="协议类型 *"
              value={form.protocol}
              onChange={handleProtocolChange}
              options={protocolOptions}
            />
            <Field label="监听端口 *" value={form.port} onChange={f('port')} type="number" />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">协议参数 (JSON)</label>
            <textarea
              value={form.settings}
              onChange={e => setForm(p => ({ ...p, settings: e.target.value }))}
              rows={5}
              className="w-full px-3 py-2 border border-gray-300 rounded-lg text-xs font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          <Field label="备注" value={form.remark} onChange={f('remark')} />
          {err && <p className="text-sm text-red-500">{err}</p>}
        </div>
      </Modal>

      {/* 节点密钥配置 Modal */}
      {keyModal && (
        <NodeKeyModal profile={keyModal} onClose={() => setKeyModal(null)} />
      )}
    </div>
  )
}
