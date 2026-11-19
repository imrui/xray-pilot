import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { Field, Btn, Switch, SelectField } from '@/components/ui/Form'
import { dnsApi, dnsStatus, useDNSBindings, type DNSSettings } from '@/lib/dns'
import { useConfirm } from '@/lib/confirm-context'
import { useAuthStore } from '@/store/auth'
import { pushToast } from '@/lib/notify'
import { Section } from './shared'

export default function DNSSection() {
  const admin = useAuthStore(s => s.role === 'super_admin')
  const settings = useQuery({ queryKey: ['dns-settings'], queryFn: dnsApi.settings, enabled: admin })
  if (!admin) return <p>仅超级管理员可管理 DNS 服务商。</p>
  if (!settings.data) return <p>{settings.isError ? '读取 DNS 设置失败' : '加载中…'}</p>
  return <DNSForm settings={settings.data} />
}

function DNSForm({ settings }: { settings: DNSSettings }) {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const [draft, setDraft] = useState<DNSSettings | null>(null)
  const [token, setToken] = useState('')
  const [tested, setTested] = useState(false)
  const form = draft ?? settings
  const { data: bindings = [] } = useDNSBindings()
  const attention = bindings.filter(b => ['failed', 'review', 'rebind'].includes(b.status))
  const action = useMutation({
    mutationFn: async (kind: 'save' | 'clear' | 'test') => {
      if (kind === 'test') return dnsApi.test()
      if (kind === 'clear') return dnsApi.clear()
      return dnsApi.save({ provider: form.provider, enabled: form.enabled, zone_id: form.zone_id, token })
    },
    onSuccess: async (_, kind) => {
      setTested(kind === 'test')
      if (kind === 'test') return
      setToken('')
      await Promise.all([qc.invalidateQueries({ queryKey: ['dns-settings'] }), qc.invalidateQueries({ queryKey: ['dns-bindings'] })])
      setDraft(null)
      pushToast({ title: 'DNS 设置已保存', variant: 'success' })
    },
  })
  return <Section title="节点 DNS 同步" description="节点更换 IP 后，更新已绑定的 Cloudflare 记录。" actions={<label className="inline-flex items-center gap-2">启用 DNS 同步<Switch checked={form.enabled} onChange={enabled => setDraft({ ...form, enabled })} /></label>}>
    <SelectField label="DNS 服务商" value={form.provider} onChange={provider => setDraft({ ...form, provider })} options={[{ value: 'cloudflare', label: 'Cloudflare' }]} />
    <Field label="Zone ID" value={form.zone_id} onChange={e => setDraft({ ...form, zone_id: e.target.value })} />
    <Field label={settings.configured ? 'API Token（已配置）' : 'API Token'} type="password" autoComplete="new-password" value={token} onChange={e => setToken(e.target.value)} placeholder={settings.configured ? '留空沿用原凭据' : '需要目标 Zone 的 DNS 编辑权限'} />
    <div className="flex flex-wrap items-center gap-2">
      <Btn disabled={action.isPending || (!draft && !token)} onClick={() => action.mutate('save')}>保存配置</Btn>
      <Btn variant="secondary" disabled={action.isPending || !settings.configured} onClick={() => { setTested(false); action.mutate('test') }}>测试已保存的连接</Btn>
      {tested && <span role="status">读取成功</span>}
      {settings.configured && <Btn variant="ghost" disabled={action.isPending} onClick={async () => {
        if (await confirm({ title: '清除 API Token？', description: '清除后将关闭 DNS 同步。', tone: 'danger' })) action.mutate('clear')
      }}>清除凭据</Btn>}
    </div>
    {attention.length > 0 && <div className="space-y-2"><h4>需要处理的记录</h4>{attention.map(b => <Link key={b.node_id} className="block text-[var(--accent)]" to={`/nodes?ids=${b.node_id}`}>{b.domain} · {dnsStatus[b.status]} · 节点 #{b.node_id}</Link>)}</div>}
  </Section>
}
