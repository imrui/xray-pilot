import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import request from '@/lib/axios'
import { useAuthStore } from '@/store/auth'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'

type Policy = { enable_ipv6_outbound: boolean; ipv4_force_domains: string[]; ipv6_force_domains: string[]; ipv6_failure_policy: string; custom_rules: unknown[] }
type Template = { id: number; name: string; description: string; draft: string; version: number }
type Revision = { template_id: number; revision: number; payload: string }
type Binding = { scope: string; target_id: number; mode: string; template_id: number; revision: number; overrides: string; udp443_policy: string; version: number; config_path?: string }
type Job = { id: number; node_id: number; status: string; error: string; created_at: string }
const empty: Policy = { enable_ipv6_outbound: false, ipv4_force_domains: [], ipv6_force_domains: [], ipv6_failure_policy: 'strict', custom_rules: [] }
const preset4 = ['anthropic.com', 'claude.ai', 'openai.com', 'chatgpt.com', 'x.ai', 'grok.com', 'gemini.google.com', 'aistudio.google.com'].map(x => 'domain:' + x)
const preset6 = ['youtube.com', 'googlevideo.com', 'google.com', 'netflix.com'].map(x => 'domain:' + x)
const control = 'w-full rounded border border-[var(--border)] bg-[var(--panel-strong)] p-2 text-sm'
const statusNames: Record<string, string> = { pending: '等待执行', validating: '正在预检', staged: '预检通过', persisted: '已落盘', applying: '正在应用', verifying: '健康验证', succeeded: '已应用', failed: '失败', unknown: '待核对', superseded: '已被新配置取代', rolled_back: '已恢复健康策略', rollback_failed: '恢复失败，自动重试已停止' }
async function get<T>(url: string): Promise<T> { return (await request.get<{ data: T }>(url)).data.data }
async function allOptions(kind: string): Promise<{ id: number; name: string }[]> {
 const rows: { id: number; name: string }[] = []
 for (let page = 1; ; page++) {
  const data = await get<{ list: { id: number; name?: string; username?: string }[] | null; total: number }>(`/${kind}?page=${page}&page_size=200`)
  const list = data.list ?? []
  rows.push(...list.map(x => ({ id: x.id, name: x.name ?? x.username ?? String(x.id) })))
  if (!list.length || rows.length >= data.total) return rows
 }
}
async function deploy(nodeIDs: number[]) {
 const pairs = await Promise.all(nodeIDs.map(async id => [id, (await get<{ desired_hash: string }>(`/nodes/${id}/effective-config`)).desired_hash] as const))
 await request.post('/config-deployments', { node_ids: nodeIDs, expected_hashes: Object.fromEntries(pairs), request_key: crypto.randomUUID() })
}

export default function Routing() {
 const qc = useQueryClient()
 const superAdmin = useAuthStore(s => s.role) === 'super_admin'
 const [message, setMessage] = useState('')
 const [busy, setBusy] = useState(false)
 const [edit, setEdit] = useState<Template>({ id: 0, name: '', description: '', draft: JSON.stringify(empty), version: 0 })
 const [policy, setPolicy] = useState<Policy>(empty)
 const [rules, setRules] = useState('[]')
 const [scope, setScope] = useState('node')
 const [target, setTarget] = useState(0)
 const [binding, setBinding] = useState<Binding>()
 const [preview, setPreview] = useState('')
 const [selectedNodes, setSelectedNodes] = useState<number[]>([])
 const [sni, setSni] = useState('')
 const [probeURL, setProbeURL] = useState('')
 const templates = useQuery({ queryKey: ['routing-templates'], queryFn: () => get<Template[]>('/routing-templates') })
 const revisions = useQuery({ queryKey: ['routing-revisions', binding?.template_id], queryFn: () => get<Revision[]>('/routing-templates/' + binding!.template_id + '/revisions'), enabled: !!binding?.template_id })
 const objects = useQuery({ queryKey: ['routing-objects'], queryFn: async () => {
  const [node, group, user] = await Promise.all([allOptions('nodes'), allOptions('groups'), allOptions('users')])
  return { node, group, user }
 } })
 const jobs = useQuery({ queryKey: ['routing-jobs'], queryFn: () => get<Job[]>('/config-deployments'), refetchInterval: 4000 })
 const path = (s: string, id: number) => s === 'global' ? '/settings/routing-default' : '/' + ({ node: 'nodes', group: 'groups', user: 'users' }[s]) + '/' + id + '/routing'
 async function run(fn: () => Promise<void>) { setBusy(true); setMessage(''); try { await fn(); await qc.invalidateQueries({ queryKey: ['routing-templates'] }); await qc.invalidateQueries({ queryKey: ['routing-revisions'] }); await qc.invalidateQueries({ queryKey: ['routing-jobs'] }) } catch (e) { setMessage(e instanceof Error ? e.message : '操作失败') } finally { setBusy(false) } }
 function load(t: Template) { setEdit(t); const p = JSON.parse(t.draft) as Policy; setPolicy(p); setRules(JSON.stringify(p.custom_rules, null, 2)) }
 async function save() {
  const normalize = (items: string[]) => [...new Set(items.map(x => x.trim()).filter(Boolean))]
  const data = { ...edit, draft: { ...policy, ipv4_force_domains: normalize(policy.ipv4_force_domains), ipv6_force_domains: normalize(policy.ipv6_force_domains), custom_rules: JSON.parse(rules) } }
  const res = edit.id ? await request.patch('/routing-templates/' + edit.id, data) : await request.post('/routing-templates', data)
  load(res.data.data); setMessage('草稿已保存，发布修订后才能绑定节点。')
 }
 async function loadBinding(s: string, id: number) { setBinding(undefined); setPreview(''); setScope(s); setTarget(id); if (id || s === 'global') setBinding(await get<Binding>(path(s, id))) }
 const choices = objects.data?.[scope as 'node' | 'group' | 'user'] ?? []
 return <PageShell className="space-y-5">
  <div><h2 className="text-xl font-semibold">路由与配置发布</h2><p className="mt-2 text-sm text-soft">现有节点保持原有路由。先保存草稿、发布修订，再绑定并下发；完整配置应用会重启节点。</p></div>
  <p className="text-sm text-soft">模板模式当前支持 Xray 26.3.27。IPv6 为严格策略；IP 字面量不会转换地址族。有可用用户时执行代理请求验证；空用户节点只验证进程、监听端口和管理接口。</p>
  {message && <p role="status" className="rounded border border-[var(--border)] p-3 text-sm">{message}</p>}
  {(templates.isError || objects.isError || jobs.isError) && <p role="alert">加载失败，请刷新后重试。</p>}
  <div className="grid gap-5 lg:grid-cols-[260px_minmax(0,1fr)]">
   <SurfaceCard className="space-y-3 p-4"><h3 className="font-semibold">路由模板</h3>
    {templates.data?.map(t => <button key={t.id} className="block w-full rounded border border-[var(--border)] p-3 text-left text-sm" onClick={() => load(t)}>{t.name}<span className="block text-xs text-soft">草稿版本 {t.version}</span></button>)}
    {superAdmin && <Btn variant="secondary" onClick={() => load({ id: 0, name: '', description: '', draft: JSON.stringify(empty), version: 0 })}>新建模板</Btn>}
   </SurfaceCard>
   <SurfaceCard className="space-y-4 p-5">
    <h3 className="font-semibold">模板草稿</h3>
    <label className="grid gap-1 text-sm">名称<input className={control} value={edit.name} disabled={!superAdmin} onChange={e => setEdit({ ...edit, name: e.target.value })} /></label>
    <label className="grid gap-1 text-sm">说明<input className={control} value={edit.description} disabled={!superAdmin} onChange={e => setEdit({ ...edit, description: e.target.value })} /></label>
    <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={policy.enable_ipv6_outbound} disabled={!superAdmin} onChange={e => setPolicy({ ...policy, enable_ipv6_outbound: e.target.checked })} />启用 IPv6 分流（严格 IPv6，故障时不回退）</label>
    <div className="grid gap-3 md:grid-cols-2">
     <label className="grid gap-1 text-sm">IPv4 域名（每行一条）<textarea rows={7} className={control} disabled={!superAdmin} value={policy.ipv4_force_domains.join('\n')} onChange={e => setPolicy({ ...policy, ipv4_force_domains: e.target.value.split('\n') })} /></label>
     <label className="grid gap-1 text-sm">IPv6 域名（关闭 IPv6 时使用 IPv4）<textarea rows={7} className={control} disabled={!superAdmin} value={policy.ipv6_force_domains.join('\n')} onChange={e => setPolicy({ ...policy, ipv6_force_domains: e.target.value.split('\n') })} /></label>
    </div>
    {superAdmin && <Btn variant="secondary" onClick={() => setPolicy({ ...policy, ipv4_force_domains: preset4, ipv6_force_domains: preset6 })}>填入 AI／流媒体参考域名</Btn>}
    <details><summary className="cursor-pointer text-sm">高级自定义路由 JSON</summary><p className="my-2 text-xs text-soft">规则优先于域名列表，不能修改 API 或运维代理保护规则。</p><textarea aria-label="自定义路由 JSON" rows={8} className={control + ' font-mono'} disabled={!superAdmin} value={rules} onChange={e => setRules(e.target.value)} /></details>
    {superAdmin && <div className="flex flex-wrap gap-2"><Btn disabled={busy} onClick={() => void run(save)}>保存草稿</Btn><Btn variant="secondary" disabled={busy || !edit.id} onClick={() => void run(async () => { await request.post('/routing-templates/' + edit.id + '/publish', { version: edit.version }); const list = await get<Template[]>('/routing-templates'); load(list.find(t => t.id === edit.id)!); setMessage('修订已发布，现有绑定仍保持原修订。') })}>发布已保存草稿</Btn></div>}
    {edit.id > 0 && <div className="flex flex-wrap gap-2"><Btn variant="secondary" disabled={busy} onClick={() => void run(async () => setPreview(JSON.stringify({ bindings: await get('/routing-templates/' + edit.id + '/impact'), revisions: await get('/routing-templates/' + edit.id + '/revisions') }, null, 2)))}>查看历史修订与引用</Btn>{superAdmin && <Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { await request.delete('/routing-templates/' + edit.id); load({ id: 0, name: '', description: '', draft: JSON.stringify(empty), version: 0 }); setMessage('未发布草稿已删除。') })}>删除未发布草稿</Btn>}</div>}
   </SurfaceCard>
  </div>
  <SurfaceCard className="space-y-4 p-5"><h3 className="font-semibold">绑定、继承与下发</h3>
   <div className="grid gap-3 sm:grid-cols-2"><label className="grid gap-1 text-sm">配置范围<select className={control} value={scope} onChange={e => void run(() => loadBinding(e.target.value, 0))}><option value="node">节点</option><option value="user">用户订阅策略</option>{superAdmin && <><option value="group">分组</option><option value="global">全局默认</option></>}</select></label>
   {scope !== 'global' && <label className="grid gap-1 text-sm">对象<select className={control} value={target} onChange={e => void run(() => loadBinding(scope, Number(e.target.value)))}><option value="0">请选择</option>{choices.map(x => <option key={x.id} value={x.id}>{x.name}</option>)}</select></label>}</div>
   {binding && <>
    {scope !== 'user' && <div className="grid gap-3 sm:grid-cols-3">
     <label className="grid gap-1 text-sm">模式<select className={control} value={binding.mode} onChange={e => setBinding({ ...binding, mode: e.target.value, template_id: 0, revision: 0 })}>{scope === 'node' && <option value="legacy">保持原有路由</option>}{scope !== 'global' && <option value="inherit">继承</option>}<option value="template">指定模板</option></select></label>
     {binding.mode === 'template' && <><label className="grid gap-1 text-sm">模板<select className={control} value={binding.template_id} onChange={e => setBinding({ ...binding, template_id: Number(e.target.value), revision: 0 })}><option value="0">请选择</option>{templates.data?.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}</select></label><label className="grid gap-1 text-sm">固定修订<select className={control} value={binding.revision} onChange={e => setBinding({ ...binding, revision: Number(e.target.value) })}><option value="0">请选择</option>{revisions.data?.map(r => <option key={r.revision} value={r.revision}>{r.revision}</option>)}</select></label></>}
    </div>}
    {(scope === 'node' || scope === 'user' || scope === 'global') && <label className="grid gap-1 text-sm">客户端 UDP 443 策略<select className={control} value={binding.udp443_policy} onChange={e => setBinding({ ...binding, udp443_policy: e.target.value })}><option value="inherit">继承（全局默认阻止）</option><option value="block">阻止 UDP 443</option><option value="allow">允许 UDP 443</option></select><span className="text-xs text-soft">阻止 UDP 443 保留标准 Vision；允许 UDP 443 在 Clash / sing-box 暂无等价表达时排除对应条目并提示。</span></label>}
    {scope === 'node' && <label className="grid gap-1 text-sm">节点字段覆盖 JSON（null 继承，[] 清空）<textarea rows={4} className={control + ' font-mono'} value={binding.overrides} onChange={e => setBinding({ ...binding, overrides: e.target.value })} /></label>}
    {scope === 'node' && superAdmin && <label className="grid gap-1 text-sm">Xray 配置文件路径（空白使用安装默认路径）<input className={control} value={binding.config_path ?? ''} onChange={e => setBinding({ ...binding, config_path: e.target.value })} placeholder="/usr/local/etc/xray/config.json" /><span className="text-xs text-soft">下发前会与 systemd 启动参数核对，不支持多配置文件合并。</span></label>}
    <div className="flex flex-wrap gap-2">
     <Btn disabled={busy} onClick={() => void run(async () => { const res = await request.put(path(scope, target), binding); await loadBinding(scope, target); const conflicts = res.data.data?.conflicts ?? {}; setMessage('绑定已保存。' + (Object.keys(conflicts).length ? '以下节点保留现有配置，需解决冲突：' + Object.entries(conflicts).map(([id, reason]) => `节点 ${id}: ${reason}`).join('；') : '有效配置变化的节点将按同步设置处理。')) })}>保存绑定</Btn>
     {scope === 'node' && <><Btn variant="secondary" disabled={busy} onClick={() => void run(async () => setPreview(JSON.stringify(await get('/nodes/' + target + '/effective-config'), null, 2)))}>查看已保存配置与来源</Btn><Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { await request.post('/nodes/' + target + '/preflight', {}, { timeout: 240000 }); setMessage('节点配置预检通过，未替换正式配置。') })}>节点预检</Btn><Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { await deploy([target]); setMessage('已创建下发任务，将应用已保存配置并重启节点。') })}>下发已保存配置</Btn></>}
    </div>
   </>}
   {preview && <pre className="max-h-96 overflow-auto rounded bg-[var(--panel-muted)] p-3 text-xs">{preview}</pre>}
  </SurfaceCard>
  <SurfaceCard className="space-y-4 p-5"><h3 className="font-semibold">批量下发</h3><p className="text-sm text-soft">应用所选节点的已保存配置，并逐个重启验证；结果按节点显示。</p><div className="grid max-h-48 gap-2 overflow-auto sm:grid-cols-3">{objects.data?.node.map(n => <label key={n.id} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={selectedNodes.includes(n.id)} onChange={e => setSelectedNodes(e.target.checked ? [...selectedNodes, n.id] : selectedNodes.filter(id => id !== n.id))} />{n.name}</label>)}</div><Btn disabled={busy || selectedNodes.length === 0 || selectedNodes.length > 50} onClick={() => void run(async () => { await deploy(selectedNodes); setMessage('已创建批量任务，可在下方查看每个节点的结果。') })}>下发所选 {selectedNodes.length} 个节点</Btn></SurfaceCard>
  <SurfaceCard className="space-y-4 p-5"><h3 className="font-semibold">节点诊断与恢复</h3><p className="text-sm text-soft">先在上方选择一个节点。SNI 诊断从该节点发起，分别检查 TLS 与 HTTP；结果不替代 Reality 握手验证。</p><label className="grid gap-1 text-sm">公开 SNI 域名<input className={control} value={sni} onChange={e => setSni(e.target.value)} placeholder="www.microsoft.com" /></label><div className="flex flex-wrap gap-2"><Btn disabled={busy || scope !== 'node' || !target || !sni} onClick={() => void run(async () => { const res = await request.post('/nodes/' + target + '/sni-diagnostics', { host: sni.trim() }, { timeout: 45000 }); setPreview(res.data.data) })}>诊断 SNI</Btn><Btn variant="secondary" disabled={busy || scope !== 'node' || !target} onClick={() => void run(async () => setPreview(JSON.stringify(await get('/nodes/' + target + '/config-state'), null, 2)))}>查看磁盘与运行版本</Btn>{superAdmin && <Btn variant="secondary" disabled={busy || scope !== 'node' || !target || !binding} onClick={() => void run(async () => { await request.post('/nodes/' + target + '/restore-policy', { version: binding!.version, request_key: crypto.randomUUID() }); await loadBinding(scope, target); setMessage('已创建健康策略恢复任务，使用当前用户和密钥。') })}>恢复最近健康策略</Btn>}</div>
   {superAdmin && <details><summary className="cursor-pointer text-sm">健康探测目标</summary><p className="my-2 text-xs text-soft">使用管理员可控的公网 HTTPS 地址；完整发布会通过真实代理请求该目标。</p><input aria-label="健康探测 URL" className={control} value={probeURL} onChange={e => setProbeURL(e.target.value)} placeholder="https://www.gstatic.com/generate_204" /><div className="mt-2 flex gap-2"><Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { const settings = await get<Record<string, string>>('/system/settings'); setProbeURL(settings['xray.health_probe_url']) })}>读取当前值</Btn><Btn disabled={busy || !probeURL} onClick={() => void run(async () => { await request.put('/system/settings', { 'xray.health_probe_url': probeURL }); setMessage('健康探测目标已保存。') })}>保存探测目标</Btn></div></details>}
  </SurfaceCard>
  <SurfaceCard className="overflow-hidden p-5"><h3 className="mb-3 font-semibold">最近发布任务</h3><div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr><th>任务</th><th>节点</th><th>状态</th><th>结果</th><th>操作</th></tr></thead><tbody>{jobs.data?.map(j => <tr key={j.id} className="border-t border-[var(--border)]"><td className="py-3">#{j.id}</td><td>{objects.data?.node.find(n => n.id === j.node_id)?.name ?? j.node_id}</td><td>{statusNames[j.status] ?? j.status}</td><td className="max-w-md break-words">{j.error || '—'}</td><td>{j.status === 'unknown' && <Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { const r = await request.post('/config-deployments/' + j.id + '/reconcile', {}, { timeout: 60000 }); setMessage('核对结果：' + (statusNames[r.data.data.status] ?? r.data.data.status)) })}>核对结果</Btn>}{['failed', 'unknown', 'rolled_back', 'rollback_failed'].includes(j.status) && <Btn variant="secondary" disabled={busy} onClick={() => void run(async () => { await request.post('/config-deployments/' + j.id + '/retry', {}, { headers: { 'Idempotency-Key': crypto.randomUUID() } }); setMessage('已用最新期望配置创建重试任务。') })}>重试</Btn>}</td></tr>)}</tbody></table></div></SurfaceCard>
 </PageShell>
}
