import './dom-setup.mjs'
import React from 'react'
import { readFileSync } from 'node:fs'
import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { render, screen, fireEvent, waitFor, cleanup, within, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Routes, Route, Outlet } from 'react-router-dom'
import { ConfirmProvider } from '../src/components/ui/ConfirmProvider'
import Routing from '../src/pages/Routing'
import Profiles from '../src/pages/Profiles'
import Users from '../src/pages/Users'
import SubscriptionSection from '../src/pages/settings/SubscriptionSection'
import { Btn } from '../src/components/ui/Form'
import { Drawer } from '../src/components/ui/Drawer'
import { TemplateNodes, TemplateEditor } from '../src/components/routing/Templates'
import { RoutingBindingDrawer } from '../src/components/routing/BindingEditor'
import { DraftPreflightModal } from '../src/components/routing/DraftPreflightModal'
import { SyncHistory } from '../src/components/routing/SyncTools'
import { submitNodeSync } from '../src/lib/node-sync'
import { requestID } from '../src/lib/request-id'
import { useAuthStore } from '../src/store/auth'
import request from '../src/lib/axios'
import {
  emptyPolicy,
  configConfirmed,
  isValidObservatoryProbeURL,
  normalizeBinding,
  upgradeRoutingPolicy,
  requiresIPv6,
} from '../src/lib/routing-model'
import type { RoutingBinding, RoutingPolicy, RoutingTemplate } from '../src/lib/routing-model'

const template: RoutingTemplate = {
  id: 1,
  name: 'IPv4 标准',
  description: '共享模板',
  draft: JSON.stringify(emptyPolicy),
  version: 4,
  latest_revision: 2,
  draft_changed: false,
  binding_count: 1,
  node_count: 2,
}
const binding: RoutingBinding = {
  scope: 'node',
  target_id: 1,
  mode: 'inherit',
  template_id: 0,
  revision: 0,
  version: 7,
  overrides: '{"ipv4_force_domains":["domain:private.example"]}',
  config_path: '/etc/xray/custom.json',
}
const overview = {
  templates: [template],
  presets: [
    {
      key: 'ai-media',
      name: 'AI 工具 / 流媒体分流',
      description: '参考策略',
      payload: JSON.stringify({
        ...emptyPolicy,
        ipv4_force_domains: ['domain:example-ai.test'],
        ipv6_force_domains: ['domain:example-video.test'],
      }),
    },
  ],
  auto_sync: false,
  nodes: [1, 2].map((id) => ({
    id,
    name: `lab-${id}`,
    ip: `192.0.2.${id}`,
    active: true,
    sync_status: 'synced',
    routing: {
      mode: 'inherit',
      binding: { ...binding, target_id: id, overrides: id === 1 ? binding.overrides : '{}' },
      template_id: 1,
      revision: 1,
      policy: emptyPolicy,
      groups: [],
      sources: {},
    },
  })),
}
const revisions = [
  {
    revision: 2,
    payload: template.draft,
    created_at: new Date().toISOString(),
    actor: 'admin:test',
  },
]
const previewConfig = (id: number) =>
  JSON.stringify({
    inbounds: [
      { tag: 'vless-reality-1', protocol: 'vless', port: 24443 },
      { tag: 'api-inbound', port: 10085 },
    ],
    outbounds: [
      { tag: 'direct-v4', protocol: 'freedom', settings: { domainStrategy: 'ForceIPv4' } },
    ],
    routing: {
      rules: [{ type: 'field', domain: [`domain:node-${id}.example`], outboundTag: 'direct-v4' }],
    },
  })
let calls: { method: string; url: string; data: unknown }[] = []
let clients: QueryClient[] = []
function api(handler?: (method: string, url: string, body: Record<string, unknown>) => unknown) {
  calls = []
  request.defaults.adapter = async (config) => {
    const method = config.method ?? 'get',
      url = config.url ?? '',
      body = config.data ? JSON.parse(config.data) : {}
    calls.push({ method, url, data: body })
    let data = await handler?.(method, url, body)
    if (data === undefined) {
      if (url === '/routing-overview') data = overview
      else if (url.endsWith('/routing/template-preview'))
        data = { ...overview.nodes[0].routing, policy: { ...emptyPolicy, ipv4_force_domains: ['domain:template.example'] } }
      else if (url.endsWith('/preview-config'))
        data = { config: previewConfig(Number(url.split('/')[2])), warnings: [] }
      else if (url.endsWith('/effective-config'))
        data = {
          routing: overview.nodes[Number(url.split('/')[2]) - 1].routing,
          desired_hash: 'hash',
        }
      else if (url.endsWith('/keys')) data = [{ profile_id: 1, settings: '{}', port: 24443 }]
      else if (url === '/profiles')
        data = { total: 1, list: [{ id: 1, name: 'Reality', active: true }] }
      else if (url.endsWith('/revisions')) data = revisions
      else if (url.match(/\/nodes\/\d+\/routing$/) && method === 'get')
        data = { ...binding, target_id: Number(url.split('/')[2]) }
      else if (method === 'put')
        data = { changed_nodes: [Number(url.split('/')[2])], conflicts: {} }
      else throw new Error(`Unexpected request ${method} ${url}`)
    }
    return { data: { code: 0, data }, status: 200, statusText: 'OK', headers: {}, config }
  }
}
function mount(element: React.ReactNode) {
  useAuthStore.getState().setSession('test', 'root', 'super_admin')
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false, gcTime: 0 },
    },
  })
  clients.push(client)
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ConfirmProvider>{element}</ConfirmProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}
afterEach(() => {
  cleanup()
  clients.forEach((c) => c.clear())
  clients = []
})

test('卡片调序与同模式点击保留当前对象、JSON 草稿及未知字段', () => {
  api()
  const a = { type: 'field', domain: ['domain:a.test'], outboundTag: 'direct', attrs: { ':method': 'GET' } }
  const b = { type: 'field', domain: ['domain:b.test'], outboundTag: 'block' }
  mount(<TemplateEditor template={{ ...template, draft: JSON.stringify({ ...emptyPolicy, custom_rules: [a, b] }) }} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.change(screen.getByLabelText('规则 #1 备注'), { target: { value: 'edited' } })
  fireEvent.click(screen.getAllByRole('button', { name: '表单' })[0])
  assert.equal((screen.getByLabelText('规则 #1 备注') as HTMLInputElement).value, 'edited')
  fireEvent.click(screen.getAllByRole('button', { name: 'JSON' })[0])
  fireEvent.click(screen.getByRole('button', { name: '下移规则 #1' }))
  assert.deepEqual(JSON.parse((screen.getByLabelText('规则 #2 JSON') as HTMLTextAreaElement).value).domain, a.domain)
  fireEvent.click(screen.getAllByRole('button', { name: '表单' })[1])
  fireEvent.click(screen.getByRole('tab', { name: '策略 JSON 源码' }))
  const policy = JSON.parse((screen.getByLabelText('策略 JSON 源码') as HTMLTextAreaElement).value)
  assert.deepEqual(policy.custom_rules, [b, { ...a, remark: 'edited' }])
})

test('无效规则 JSON 调序及切换功能页后仍保留，阻止保存与全局 JSON 切换', async () => {
  api()
  const rules = [
    { type: 'field', domain: ['domain:a.test'], outboundTag: 'direct' },
    { type: 'field', domain: ['domain:b.test'], outboundTag: 'block' },
  ]
  mount(<TemplateEditor template={{ ...template, draft: JSON.stringify({ ...emptyPolicy, custom_rules: rules }) }} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.click(screen.getAllByRole('button', { name: 'JSON' })[0])
  fireEvent.change(screen.getByLabelText('规则 #1 JSON'), { target: { value: '{"domain":' } })
  fireEvent.click(screen.getByRole('button', { name: '下移规则 #1' }))
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  fireEvent.click(screen.getByRole('tab', { name: /路由规则/ }))
  assert.equal((screen.getByLabelText('规则 #2 JSON') as HTMLTextAreaElement).value, '{"domain":')
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText(/存在尚未修正的单条 JSON/)
  assert.equal(calls.some((c) => c.method === 'patch'), false)
  fireEvent.click(screen.getByRole('tab', { name: '策略 JSON 源码' }))
  assert.equal(screen.queryByLabelText('策略 JSON 源码'), null)
  fireEvent.change(screen.getByLabelText('规则 #2 JSON'), { target: { value: JSON.stringify(rules[0]) } })
  fireEvent.click(screen.getByRole('tab', { name: '策略 JSON 源码' }))
  assert.ok(screen.getByLabelText('策略 JSON 源码'))
})

test('删除前方出站不串用后方卡片 JSON，修复后表单与保存保持一致', async () => {
  api((_method, _url, body) => body.draft ? { ...template, draft: JSON.stringify(body.draft), version: 5 } : undefined)
  const first = { tag: 'first', protocol: 'socks', settings: { servers: [{ address: 'a.test', port: 1080 }] } }
  const second = { tag: 'second', protocol: 'socks', mux: { enabled: false }, settings: { servers: [{ address: 'b.test', port: 1080 }] } }
  mount(<TemplateEditor template={{ ...template, draft: JSON.stringify({ ...emptyPolicy, custom_outbounds: [first, second] }) }} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  fireEvent.change(screen.getAllByLabelText('落地服务器地址 *')[0], { target: { value: 'edited.test' } })
  fireEvent.click(screen.getAllByRole('button', { name: '表单' })[0])
  assert.equal((screen.getAllByLabelText('落地服务器地址 *')[0] as HTMLInputElement).value, 'edited.test')
  fireEvent.click(screen.getAllByRole('button', { name: 'JSON' })[1])
  fireEvent.change(screen.getByLabelText('自定义出站 second JSON'), { target: { value: '{"tag":' } })
  fireEvent.click(screen.getAllByRole('button', { name: '删除' })[0])
  assert.equal((screen.getByLabelText('自定义出站 second JSON') as HTMLTextAreaElement).value, '{"tag":')
  fireEvent.change(screen.getByLabelText('自定义出站 second JSON'), { target: { value: JSON.stringify(second) } })
  fireEvent.click(screen.getByRole('button', { name: '表单' }))
  assert.equal((screen.getByLabelText('落地服务器地址 *') as HTMLInputElement).value, 'b.test')
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const saved = calls.find((c) => c.method === 'patch')!.data as { draft: { custom_outbounds: unknown[] } }
  assert.deepEqual(saved.draft.custom_outbounds, [second])
})

test('WireGuard Reserved 校验三个字节并写入 settings，保留未知参数', async () => {
  api((_method, _url, body) => body.draft ? { ...template, draft: JSON.stringify(body.draft), version: 5 } : undefined)
  const outbound = { tag: 'warp', protocol: 'wireguard', settings: { secretKey: 'key', address: ['172.16.0.2/32'], peers: [{ endpoint: 'warp.test:2408', publicKey: 'public', keepAlive: 25 }], noKernelTun: true } }
  mount(<TemplateEditor template={{ ...template, draft: JSON.stringify({ ...emptyPolicy, custom_outbounds: [outbound] }) }} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  const field = screen.getByLabelText('WARP Reserved（可选，如 0,0,0）')
  for (const value of ['1,2', '1,2,256', '1,2,-1', '1,,3', '1,2,3.5']) {
    fireEvent.change(field, { target: { value } })
    assert.ok(screen.getByText(/Reserved 必须是 3 个/))
  }
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText(/存在尚未修正/)
  assert.equal(calls.some((c) => c.method === 'patch'), false)
  fireEvent.change(field, { target: { value: '1,2,3' } })
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const saved = calls.find((c) => c.method === 'patch')!.data as { draft: { custom_outbounds: typeof outbound[] } }
  assert.deepEqual(saved.draft.custom_outbounds[0].settings, { ...outbound.settings, reserved: [1, 2, 3] })
})

test('已保存节点覆盖的无效 JSON 计入 dirty，阻止保存、影响预览和沙箱，修正后可保存', async () => {
  const outbound = { tag: 'proxy', protocol: 'socks', settings: { servers: [{ address: 'old.test', port: 1080 }] } }
  api((method, url) => {
    if (method === 'get' && url.endsWith('/routing')) return { ...binding, overrides: JSON.stringify({ outbound_overrides: { proxy: { mode: 'replace', outbound } } }) }
    if (url.endsWith('/template-preview')) return { ...overview.nodes[0].routing, policy: { ...emptyPolicy, custom_outbounds: [outbound] } }
  })
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  await screen.findByLabelText('出站覆盖模式 · proxy')
  fireEvent.click(screen.getByRole('button', { name: 'JSON' }))
  fireEvent.change(screen.getByLabelText('自定义出站 proxy JSON'), { target: { value: '{"tag":' } })
  assert.ok(screen.getByRole('button', { name: '放弃本节修改' }))
  for (const name of ['保存路由', '查看影响范围', '沙箱预检']) {
    const button = screen.getByRole('button', { name }) as HTMLButtonElement
    assert.equal(button.disabled, true)
    fireEvent.click(button)
  }
  assert.equal(calls.some((c) => c.method === 'put' || c.url.endsWith('/preflight-draft') || c.url.endsWith('/routing/preview')), false)
  fireEvent.click(screen.getByRole('button', { name: '关闭', exact: true }))
  await screen.findByText('放弃未保存的路由修改？')
  fireEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  const updated = { ...outbound, settings: { servers: [{ address: 'new.test', port: 1080 }] } }
  fireEvent.change(screen.getByLabelText('自定义出站 proxy JSON'), { target: { value: JSON.stringify(updated) } })
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await waitFor(() => assert.ok(calls.find((c) => c.method === 'put')))
  const saved = JSON.parse((calls.find((c) => c.method === 'put')!.data as RoutingBinding).overrides)
  assert.deepEqual(saved.outbound_overrides.proxy.outbound, updated)
})

test('继承模板刷新移除出站后，清理失效覆盖同时解除该卡片的错误状态', async () => {
  const outbound = { tag: 'proxy', protocol: 'socks', settings: { servers: [{ address: 'old.test', port: 1080 }] } }
  api((method, url) => {
    if (method === 'get' && url.endsWith('/routing')) return { ...binding, overrides: JSON.stringify({ outbound_overrides: { proxy: { mode: 'replace', outbound } } }) }
    if (url.endsWith('/template-preview')) return { ...overview.nodes[0].routing, policy: { ...emptyPolicy, custom_outbounds: [outbound] } }
  })
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  await screen.findByLabelText('出站覆盖模式 · proxy')
  fireEvent.click(screen.getByRole('button', { name: 'JSON' }))
  fireEvent.change(screen.getByLabelText('自定义出站 proxy JSON'), { target: { value: '{"tag":' } })
  assert.equal((screen.getByRole('button', { name: '保存路由' }) as HTMLButtonElement).disabled, true)
  await act(async () => {
    clients[0].setQueryData(['routing-template-preview', 1, 'inherit', 0, 0], {
      ...overview.nodes[0].routing,
      policy: emptyPolicy,
    })
  })
  await screen.findByText(/检测到失效的节点出站覆盖/)
  fireEvent.click(screen.getByRole('button', { name: '清理失效出站覆盖' }))
  assert.equal(screen.queryByText(/节点出站仍有未完成或无效的输入/), null)
  assert.equal((screen.getByRole('button', { name: '保存路由' }) as HTMLButtonElement).disabled, false)
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await waitFor(() => assert.ok(calls.find((c) => c.method === 'put')))
  assert.deepEqual(JSON.parse((calls.find((c) => c.method === 'put')!.data as RoutingBinding).overrides), {})
})

test('预检请求固定输入，关闭重开隔离旧响应，修改覆盖条件清除旧结果', async () => {
  const responses: ((v: unknown) => void)[] = []
  api((method, url) => method === 'post' && url.endsWith('/preflight-draft') ? new Promise((resolve) => responses.push(resolve)) : undefined)
  function Harness() {
    const [open, setOpen] = React.useState(true)
    return <><button onClick={() => setOpen(true)}>重新打开预检</button><DraftPreflightModal open={open} onClose={() => setOpen(false)} nodes={overview.nodes} getDraftPolicy={() => emptyPolicy} draftIdentity="draft-1" /></>
  }
  mount(<Harness />)
  fireEvent.click(screen.getByRole('button', { name: '运行远端沙箱预检' }))
  await waitFor(() => assert.equal(responses.length, 1))
  const select = screen.getByLabelText('选择沙箱验证节点') as HTMLSelectElement
  assert.equal(select.matches(':disabled'), true)
  fireEvent.change(select, { target: { value: '2' } })
  assert.equal(select.value, '1')
  fireEvent.click(screen.getByRole('button', { name: '关闭对话框' }))
  fireEvent.click(screen.getByRole('button', { name: '重新打开预检' }))
  fireEvent.change(screen.getByLabelText('选择沙箱验证节点'), { target: { value: '2' } })
  fireEvent.click(screen.getByRole('button', { name: '运行远端沙箱预检' }))
  await waitFor(() => assert.equal(responses.length, 2))
  const result = { valid: true, requires_ipv6: false, elapsed_ms: 1, warnings: [], redacted_config: '{}', error: '' }
  await act(async () => { responses[0]({ ...result, xray_version: 'node-A' }) })
  assert.equal(screen.queryByText('Xray node-A'), null)
  assert.equal(screen.queryByText('远端 Xray 沙箱预检通过'), null)
  await act(async () => { responses[1]({ ...result, xray_version: 'node-B' }) })
  await screen.findByText('Xray node-B')
  fireEvent.click(screen.getByRole('switch'))
  assert.equal(screen.queryByText('远端 Xray 沙箱预检通过'), null)
  const sent = calls.filter((c) => c.url.endsWith('/preflight-draft'))
  assert.deepEqual(sent.map((c) => c.url), ['/nodes/1/preflight-draft', '/nodes/2/preflight-draft'])
})

test('预检中的策略变化取消旧会话，迟到响应不会恢复成功状态', async () => {
  let finish: ((v: unknown) => void) | undefined
  api((method, url) => method === 'post' && url.endsWith('/preflight-draft') ? new Promise((resolve) => { finish = resolve }) : undefined)
  function Harness() {
    const [identity, setIdentity] = React.useState('before')
    return <><button onClick={() => setIdentity('after')}>修改草稿</button><DraftPreflightModal open onClose={() => {}} nodes={overview.nodes} draftIdentity={identity} getDraftPolicy={() => emptyPolicy} /></>
  }
  mount(<Harness />)
  fireEvent.click(screen.getByRole('button', { name: '运行远端沙箱预检' }))
  await waitFor(() => assert.ok(finish))
  fireEvent.click(screen.getByText('修改草稿'))
  await act(async () => { finish!({ valid: true, xray_version: 'old-draft', requires_ipv6: false, elapsed_ms: 1, warnings: [], redacted_config: '{}', error: '' }) })
  assert.equal(screen.queryByText('远端 Xray 沙箱预检通过'), null)
  assert.equal((screen.getByRole('button', { name: '运行远端沙箱预检' }) as HTMLButtonElement).disabled, false)
})

test('DNS 服务器调序保留对应卡片编辑缓冲与未知字段', () => {
  api()
  const first = { address: '1.1.1.1', domains: ['domain:a.test'], timeoutMs: 3000 }
  const second = { address: '8.8.8.8', domains: ['domain:b.test'] }
  mount(<TemplateEditor template={{ ...template, draft: JSON.stringify({ ...emptyPolicy, dns: { enabled: true, servers: [first, second] } }) }} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.click(screen.getByRole('tab', { name: /DNS 设置/ }))
  fireEvent.change(screen.getByLabelText('DNS #1 匹配域名'), { target: { value: 'domain:edited.test\n' } })
  fireEvent.click(screen.getByRole('button', { name: '下移 DNS #1' }))
  assert.equal((screen.getByLabelText('DNS #1 匹配域名') as HTMLTextAreaElement).value, 'domain:b.test')
  assert.equal((screen.getByLabelText('DNS #2 匹配域名') as HTMLTextAreaElement).value, 'domain:edited.test\n')
  fireEvent.click(screen.getByRole('tab', { name: '策略 JSON 源码' }))
  const policy = JSON.parse((screen.getByLabelText('策略 JSON 源码') as HTMLTextAreaElement).value)
  assert.deepEqual(policy.dns.servers, [second, { ...first, domains: ['domain:edited.test'] }])
})

test('路由页采用列表和抽屉，不提供节点同步入口', async () => {
  api()
  mount(<Routing />)
  await screen.findAllByText('IPv4 标准')
  assert.equal(screen.queryByRole('button', { name: /^同步|下发/ }), null)
  fireEvent.click(screen.getAllByRole('button', { name: '编辑' })[0])
  assert.ok(screen.getByRole('dialog', { name: '编辑路由模板 · IPv4 标准' }))
  assert.ok(screen.getByLabelText('模板名称 *'))
})

test('参考域名使用后端预设目录，只填入草稿，不自动应用节点', async () => {
  api()
  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)
  const importButton = screen.getByRole('button', { name: '导入参考域名' })
  await waitFor(() => assert.equal((importButton as HTMLButtonElement).disabled, false))
  fireEvent.click(importButton)
  assert.ok(screen.getByText(/google.com 覆盖范围较广/))
  fireEvent.click(screen.getByRole('button', { name: '替换现有列表' }))
  assert.equal(
    (screen.getByLabelText(/强制 IPv4 域名/) as HTMLTextAreaElement).value,
    'domain:example-ai.test',
  )
  assert.equal(
    (screen.getByLabelText(/强制 IPv6 域名/) as HTMLTextAreaElement).value,
    'domain:example-video.test',
  )
  assert.equal(calls.filter((c) => c.method !== 'get').length, 0)
})

test('预设可复制成独立草稿，不改写源模板或节点绑定', async () => {
  api((method, url, body) => {
    if (method === 'get' && url === '/routing-overview')
      return { ...overview, templates: [{ ...template, preset_key: 'ai-media' }] }
    if (method === 'post' && url === '/routing-templates')
      return { id: 44, version: 1, name: body.name, draft: JSON.stringify(body.draft) }
  })
  mount(<Routing />)
  await screen.findAllByText('IPv4 标准')
  fireEvent.keyDown(screen.getAllByRole('button', { name: '更多操作' })[0], { key: 'ArrowDown' })
  fireEvent.click(await screen.findByRole('menuitem', { name: '复制草稿为新模板' }))
  assert.ok(screen.getByRole('dialog', { name: '新增路由模板' }))
  assert.equal((screen.getByLabelText('模板名称 *') as HTMLInputElement).value, 'IPv4 标准 - 副本')
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const writes = calls.filter((c) => c.method !== 'get')
  assert.equal(writes.length, 1)
  assert.equal(writes[0].url, '/routing-templates')
  assert.equal(writes[0].method, 'post')
  assert.equal((writes[0].data as Record<string, unknown>).preset_key, undefined)
})

test('新路由直接配置两组强制域名，不再使用 IPv6 开关', async () => {
  api((_method, _url, body) =>
    body.draft ? { ...template, draft: JSON.stringify(body.draft), version: 5 } : undefined,
  )
  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)
  assert.equal(screen.queryByLabelText('IPv6 出站'), null)
  fireEvent.change(screen.getByLabelText(/强制 IPv4 域名/), {
    target: { value: 'domain:v4.example' },
  })
  fireEvent.change(screen.getByLabelText(/强制 IPv6 域名/), {
    target: { value: 'domain:v6.example' },
  })
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const payload = (
    calls.find((c) => c.method === 'patch')!.data as { draft: Record<string, unknown> }
  ).draft
  assert.equal(payload.schema_version, 3)
  assert.equal(payload.domain_strategy, 'IPIfNonMatch')
  assert.deepEqual(payload.custom_outbounds, [])
  assert.equal('enable_ipv6_outbound' in payload, false)
  assert.deepEqual(payload.ipv4_force_domains, ['domain:v4.example'])
  assert.deepEqual(payload.ipv6_force_domains, ['domain:v6.example'])
})

test('旧版 IPv6 关闭的草稿转换时保留原出口，新版空列表不要求 IPv6', async () => {
  const old = JSON.stringify({
    ...emptyPolicy,
    schema_version: 1,
    enable_ipv6_outbound: false,
    ipv4_force_domains: ['domain:a.example'],
    ipv6_force_domains: ['domain:b.example'],
  })
  api()
  mount(
    <TemplateEditor
      template={{ ...template, draft: old }}
      onClose={() => {}}
      onConfigure={() => {}}
    />,
  )
  assert.match(
    (screen.getByLabelText(/强制 IPv4 域名/) as HTMLTextAreaElement).value,
    /domain:b.example/,
  )
  assert.equal((screen.getByLabelText(/强制 IPv6 域名/) as HTMLTextAreaElement).value, '')
  assert.ok(screen.getByText(/正在转换旧版草稿/))
  const converted = upgradeRoutingPolicy(old)
  assert.equal(converted.schema_version, 3)
  const unversioned = upgradeRoutingPolicy(
    JSON.stringify({ ipv6_force_domains: ['domain:old.example'], ipv6_failure_policy: 'strict' }),
  )
  assert.deepEqual(unversioned.ipv4_force_domains, ['domain:old.example'])
  assert.deepEqual(unversioned.ipv6_force_domains, [])
  assert.equal(requiresIPv6(converted), false)
  assert.equal(requiresIPv6({ ...emptyPolicy, ipv6_force_domains: ['domain:v6.example'] }), true)
  assert.equal(requiresIPv6({ ...emptyPolicy, custom_rules: [{ outboundTag: 'direct-v6' }] }), true)
})

test('节点域名覆盖区分继承、自定义和清空，继承展示真实模板列表', async () => {
  api((method, url) => method === 'get' && url.endsWith('/routing') ? { ...binding, overrides: '{}' } : undefined)
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  const inherited = await screen.findByLabelText('IPv4 模板域名') as HTMLTextAreaElement
  assert.equal(screen.queryByLabelText('客户端 UDP 443 策略'), null)
  assert.equal(inherited.value, 'domain:template.example')
  assert.equal(inherited.readOnly, true)
  assert.ok(screen.getByText('模板未配置强制 IPv6 域名，无需额外设置。'))
  const choice = screen.getByLabelText('强制 IPv4 域名') as HTMLSelectElement
  assert.equal(choice.value, 'inherit')
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
  fireEvent.change(choice, { target: { value: 'custom' } })
  const input = screen.getByLabelText('IPv4 覆盖域名') as HTMLTextAreaElement
  assert.equal(input.value, 'domain:template.example')
  fireEvent.change(input, { target: { value: '  \n' } })
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/IPv4 自定义列表不能为空/)
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
  fireEvent.change(choice, { target: { value: 'clear' } })
  assert.equal(screen.queryByLabelText('IPv4 覆盖域名'), null)
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  assert.deepEqual(JSON.parse((calls.find((c) => c.method === 'put')!.data as RoutingBinding).overrides), { ipv4_force_domains: [] })
})

test('订阅设置不再读取全局路由策略，原订阅设置仍可编辑', async () => {
  api((_method, url) => url === '/system/feishu-status' ? {} : undefined)
  const changes: string[] = []
  const context = {
    form: {}, setForm: () => {}, set: () => {},
    f: (key: string) => () => { changes.push(key) },
  }
  mount(
    <Routes>
      <Route element={<Outlet context={context} />}>
        <Route index element={<SubscriptionSection />} />
      </Route>
    </Routes>,
  )
  fireEvent.change(screen.getByLabelText('节点备注格式'), { target: { value: '{node_name}' } })
  assert.deepEqual(changes, ['subscription.remark_format'])
  await waitFor(() => assert.ok(calls.some((c) => c.url === '/system/feishu-status')))
  assert.equal(screen.queryByText('全局客户端 UDP 443 策略'), null)
  assert.equal(screen.queryByRole('button', { name: '保存订阅策略' }), null)
  assert.equal(calls.some((c) => c.url.includes('routing')), false)
})

test('用户编辑不再加载路由或订阅策略，直接保存用户修改', async () => {
  const user = { id: 1, username: 'alice', real_name: '', active: true, group_ids: [], groups: [], remark: '', expires_at: null, uuid: 'test', token: 'test', feishu_enabled: false }
  api((method, url) => {
    if (method === 'get' && url === '/users') return { list: [user], total: 1 }
    if (method === 'get' && url === '/groups') return { list: [], total: 0 }
    if (method === 'put' && url === '/users/1') return user
  })
  mount(<Users />)
  fireEvent.click((await screen.findAllByRole('button', { name: '编辑' }))[0])
  fireEvent.change(screen.getByLabelText('用户名（客户端显示） *'), { target: { value: 'alice-updated' } })
  assert.equal(screen.queryByText('客户端 UDP 443'), null)
  const save = screen.getByRole('button', { name: '保存用户' }) as HTMLButtonElement
  assert.equal(save.disabled, false)
  fireEvent.click(save)
  await waitFor(() => assert.ok(calls.some((c) => c.method === 'put' && c.url === '/users/1')))
  assert.equal(calls.some((c) => c.url.includes('routing')), false)
  assert.equal((calls.find((c) => c.method === 'put')!.data as { username: string }).username, 'alice-updated')
})

test('现有自定义及清空设置保留，恢复继承删除覆盖而非复制模板', async () => {
  api((method, url) => method === 'get' && url.endsWith('/routing') ? { ...binding, overrides: '{"ipv4_force_domains":["domain:private.example"],"ipv6_force_domains":[]}' } : undefined)
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  assert.equal((await screen.findByLabelText('IPv4 覆盖域名') as HTMLTextAreaElement).value, 'domain:private.example')
  assert.equal((screen.getByLabelText('强制 IPv6 域名') as HTMLSelectElement).value, 'clear')
  for (const family of ['IPv4', 'IPv6']) fireEvent.change(screen.getByLabelText(`强制 ${family} 域名`), { target: { value: 'inherit' } })
  assert.equal((await screen.findByLabelText('IPv4 模板域名') as HTMLTextAreaElement).value, 'domain:template.example')
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  const saved = calls.find((c) => c.method === 'put')!.data as RoutingBinding
  assert.deepEqual(JSON.parse(saved.overrides), {})
  assert.equal(saved.config_path, binding.config_path)
})

test('更换固定版本实时读取模板，迟到的继承请求不会覆盖当前选择', async () => {
  let resolveOld!: (value: unknown) => void
  api((method, url, body) => {
    if (method === 'get' && url.endsWith('/routing')) return { ...binding, overrides: '{}' }
    if (url.endsWith('/template-preview')) {
      if (body.mode === 'inherit') return new Promise((resolve) => { resolveOld = resolve })
      return { ...overview.nodes[0].routing, template_id: 1, revision: 2, policy: { ...emptyPolicy, ipv4_force_domains: ['domain:version-two.example'] } }
    }
  })
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  fireEvent.change(await screen.findByLabelText('路由来源'), { target: { value: 'template' } })
  fireEvent.change(screen.getByLabelText('路由模板'), { target: { value: '1' } })
  await screen.findByRole('option', { name: 'v2 · 默认直连' })
  fireEvent.change(screen.getByLabelText('固定版本'), { target: { value: '2' } })
  assert.equal((await screen.findByLabelText('IPv4 模板域名') as HTMLTextAreaElement).value, 'domain:version-two.example')
  resolveOld({ ...overview.nodes[0].routing, policy: { ...emptyPolicy, ipv4_force_domains: ['domain:old.example'] } })
  await waitFor(() => assert.equal((screen.getByLabelText('IPv4 模板域名') as HTMLTextAreaElement).value, 'domain:version-two.example'))
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
})

test('旧版节点开关覆盖必须显式移除，保存时不遗留隐藏字段', async () => {
  api((method, url) =>
    method === 'get' && url === '/nodes/1/routing'
      ? { ...binding, overrides: '{"enable_ipv6_outbound":false}' }
      : undefined,
  )
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  fireEvent.click(await screen.findByRole('button', { name: '移除旧版开关覆盖' }))
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  assert.equal((calls.find((c) => c.method === 'put')!.data as RoutingBinding).overrides, '{}')
})

test('模板读取失败不会显示成空列表，重试后恢复只读域名', async () => {
  let fail = true
  api((method, url) => {
    if (method === 'get' && url.endsWith('/routing')) return { ...binding, overrides: '{}' }
    if (url.endsWith('/template-preview') && fail) throw new Error('分组来源加载失败')
  })
  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  await screen.findByText('模板内容加载失败：分组来源加载失败')
  assert.equal(screen.queryByText(/模板未配置强制 IPv4/), null)
  assert.equal(screen.queryByLabelText('IPv4 模板域名'), null)
  fail = false
  fireEvent.click(screen.getByRole('button', { name: '重试加载模板' }))
  assert.equal((await screen.findByLabelText('IPv4 模板域名') as HTMLTextAreaElement).value, 'domain:template.example')
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
})

test('模板侧批量绑定只更改选择的版本，保留每台节点自己的覆盖与并发版本', async () => {
  api()
  mount(<TemplateNodes template={template} onClose={() => {}} />)
  const first = await screen.findByRole('checkbox', { name: /lab-1/ })
  fireEvent.click(first)
  fireEvent.click(screen.getByRole('checkbox', { name: /lab-2/ }))
  fireEvent.change(screen.getByLabelText('为所选节点指定固定版本'), { target: { value: '2' } })
  fireEvent.click(screen.getByRole('button', { name: '批量保存（2）' }))
  await waitFor(() => assert.equal(calls.filter((c) => c.method === 'put').length, 2))
  const bodies = calls.filter((c) => c.method === 'put').map((c) => c.data as RoutingBinding)
  assert.equal(bodies[0].overrides, binding.overrides)
  assert.equal(bodies[1].overrides, '{}')
  assert.equal(bodies[0].version, 7)
  assert.equal(bodies[0].config_path, binding.config_path)
  assert.equal(bodies[0].mode, 'template')
  assert.equal(bodies[0].revision, 2)
  assert.equal(calls.filter((c) => c.url.includes('config-deployments')).length, 0)
  await screen.findByRole('link', { name: '查看受影响的 2 个节点' })
})

test('路由节点行点击只查看，不勾选；显示已保存路由并复用完整配置预览', async () => {
  api()
  mount(<TemplateNodes template={template} onClose={() => {}} />)
  fireEvent.click(await screen.findByRole('button', { name: '查看 lab-1 的路由配置' }))
  const section = await screen.findByRole('region', { name: '已保存路由 · lab-1' })
  await within(section).findByText(/domain:node-1.example/)
  assert.equal(
    (screen.getByRole('checkbox', { name: '选择 lab-1' }) as HTMLInputElement).checked,
    false,
  )
  assert.equal(
    (screen.getByRole('button', { name: '保存当前节点' }) as HTMLButtonElement).disabled,
    true,
  )
  assert.equal(calls.filter((c) => c.method !== 'get').length, 0)
  fireEvent.change(screen.getByLabelText('为所选节点指定固定版本'), { target: { value: '2' } })
  assert.ok(within(section).getByText(/domain:node-1.example/))
  fireEvent.click(screen.getByRole('button', { name: '查看完整配置' }))
  const modal = await screen.findByRole('dialog', { name: '预览生成配置 · lab-1' })
  assert.ok(await within(modal).findByText(/"inbounds"/))
  assert.ok(within(modal).getByText(/"routing"/))
  fireEvent.click(within(modal).getByRole('button', { name: '关闭' }))
  await waitFor(() =>
    assert.equal(screen.queryByRole('dialog', { name: '预览生成配置 · lab-1' }), null),
  )
  assert.equal((screen.getByLabelText('为所选节点指定固定版本') as HTMLSelectElement).value, '2')
  fireEvent.click(screen.getByRole('button', { name: '保存当前节点' }))
  await waitFor(() => assert.equal(calls.filter((c) => c.method === 'put').length, 1))
  assert.equal(calls.find((c) => c.method === 'put')!.url, '/nodes/1/routing')
  await screen.findByRole('link', { name: '查看受影响的 1 个节点' })
  assert.equal(calls.filter((c) => c.url.includes('config-deployments')).length, 0)
})

test('路由批量勾选和单节点查看分开，切换预览不会串用另一台配置', async () => {
  api()
  mount(<TemplateNodes template={template} onClose={() => {}} />)
  fireEvent.click(await screen.findByRole('checkbox', { name: '选择 lab-1' }))
  fireEvent.click(screen.getByRole('checkbox', { name: '选择 lab-2' }))
  assert.ok(screen.getByRole('button', { name: '批量保存（2）' }))
  fireEvent.click(screen.getByRole('button', { name: '查看 lab-2 的路由配置' }))
  const second = await screen.findByRole('region', { name: '已保存路由 · lab-2' })
  await within(second).findByText(/domain:node-2.example/)
  assert.equal(within(second).queryByText(/domain:node-1.example/), null)
  assert.ok(screen.getAllByRole('checkbox').every((c) => !(c as HTMLInputElement).checked))
  fireEvent.change(screen.getByLabelText('为所选节点指定固定版本'), { target: { value: '2' } })
  fireEvent.click(screen.getByRole('button', { name: '保存当前节点' }))
  await waitFor(() => assert.equal(calls.filter((c) => c.method === 'put').length, 1))
  assert.equal(calls.find((c) => c.method === 'put')!.url, '/nodes/2/routing')
  await screen.findByRole('link', { name: '查看受影响的 1 个节点' })
})

test('切换节点前保留未保存的路由选择，取消切换后不丢失输入', async () => {
  api()
  mount(<TemplateNodes template={template} onClose={() => {}} />)
  fireEvent.click(await screen.findByRole('button', { name: '查看 lab-1 的路由配置' }))
  await screen.findByRole('region', { name: '已保存路由 · lab-1' })
  fireEvent.change(screen.getByLabelText('为所选节点指定固定版本'), { target: { value: '2' } })
  fireEvent.click(screen.getByRole('button', { name: '查看 lab-2 的路由配置' }))
  await screen.findByRole('dialog', { name: '放弃未保存的路由绑定？' })
  fireEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  await waitFor(() =>
    assert.equal(screen.queryByRole('dialog', { name: '放弃未保存的路由绑定？' }), null),
  )
  assert.ok(screen.getByRole('region', { name: '已保存路由 · lab-1' }))
  assert.equal((screen.getByLabelText('为所选节点指定固定版本') as HTMLSelectElement).value, '2')
  assert.equal(calls.filter((c) => c.method !== 'get').length, 0)
})

test('保存草稿成功而发布失败时，草稿与输入保留，不变更节点绑定', async () => {
  api((method, url, body) => {
    if (method === 'patch')
      return { ...template, name: body.name, draft: JSON.stringify(body.draft), version: 5 }
    if (url.endsWith('/publish')) throw new Error('模拟版本冲突')
  })
  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)
  fireEvent.change(screen.getByLabelText('模板名称 *'), { target: { value: '已保存新名称' } })
  fireEvent.click(screen.getByRole('button', { name: '保存并发布版本' }))
  await screen.findByText(/模拟版本冲突/)
  assert.ok(screen.getByText('草稿已保存。'))
  assert.equal((screen.getByLabelText('模板名称 *') as HTMLInputElement).value, '已保存新名称')
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
  assert.equal(
    (calls.find((c) => c.url.endsWith('/publish'))!.data as { version: number }).version,
    5,
  )
})

test('关闭有修改的路由抽屉需要选择，取消保留输入；预览只允许已保存状态', async () => {
  api()
  let closed = false
  mount(
    <RoutingBindingDrawer
      scope="node"
      id={1}
      name="lab-1"
      onClose={() => {
        closed = true
      }}
      onPreview={() => {}}
    />,
  )
  const mode = await screen.findByLabelText('路由来源')
  fireEvent.change(mode, { target: { value: 'template' } })
  assert.equal(
    (screen.getByRole('button', { name: '查看完整配置' }) as HTMLButtonElement).disabled,
    true,
  )
  fireEvent.click(screen.getByRole('button', { name: '关闭' }))
  await screen.findByRole('dialog', { name: '放弃未保存的路由修改？' })
  fireEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  assert.equal(closed, false)
  await waitFor(() =>
    assert.equal(screen.queryByRole('dialog', { name: '放弃未保存的路由修改？' }), null),
  )
  assert.equal((screen.getByLabelText('路由来源') as HTMLSelectElement).value, 'template')
})

test('同步记录明确区分回退和未知结果，未知任务先核对', async () => {
  api((_method, url) =>
    url.startsWith('/config-deployments?')
      ? {
          total: 2,
          list: [
            {
              id: 1,
              node_id: 1,
              status: 'rolled_back',
              error: 'ROLLED_BACK',
              actor: 'admin:test',
              created_at: new Date().toISOString(),
            },
            {
              id: 2,
              node_id: 2,
              status: 'unknown',
              error: '',
              actor: 'admin:test',
              created_at: new Date().toISOString(),
            },
          ],
        }
      : undefined,
  )
  mount(<SyncHistory onClose={() => {}} />)
  await screen.findByText('已回退，目标未同步')
  assert.ok(screen.getByText('需要核对'))
  assert.equal(screen.getAllByRole('button', { name: '用最新配置重试' }).length, 1)
  assert.equal(screen.getAllByRole('button', { name: '核对结果' }).length, 1)
})

test('批量同步遇到单节点预览失败时继续其他节点，提交预览哈希', async () => {
  api((_method, url, body) => {
    if (url === '/nodes/1/effective-config') throw new Error('分组冲突')
    if (url === '/nodes/2/effective-config') return { desired_hash: 'hash-2' }
    if (url === '/config-deployments') {
      assert.deepEqual(body.expected_hashes, { 2: 'hash-2' })
      return [{ id: 9 }]
    }
  })
  const result = await submitNodeSync([1, 2, 2])
  assert.deepEqual(result, [
    { nodeID: 1, error: '分组冲突' },
    { nodeID: 2, jobID: 9 },
  ])
})

test('HTTP 局域网面板没有 randomUUID 时仍能提交同步', () => {
  const original = crypto.randomUUID
  Object.defineProperty(crypto, 'randomUUID', { value: undefined, configurable: true })
  try {
    const first = requestID(),
      second = requestID()
    assert.match(first, /^[0-9a-f]{32}$/)
    assert.notEqual(first, second)
  } finally {
    Object.defineProperty(crypto, 'randomUUID', { value: original, configurable: true })
  }
})

test('运行确认接受实际回执类型，恢复中的配置不冒充已同步', () => {
  const state = {
    verified_at: '2026-11-16',
    verification: 'protocol',
    runtime_hash: 'a',
    disk_hash: 'a',
  }
  assert.equal(configConfirmed(state, 'a'), true)
  assert.equal(configConfirmed({ ...state, verification: 'grpc_receipt' }, 'a'), true)
  assert.equal(configConfirmed({ ...state, verification: 'recovery_verified' }, 'a'), false)
  assert.equal(configConfirmed(state, 'b'), false)
  assert.throws(() => normalizeBinding({ ...binding, overrides: '{"custom_rules":"invalid"}' }))
})

test('协议配置勾选一个节点后可直接编辑，与节点协议的选择习惯一致', async () => {
  api((method, url, body) => {
    if (url === '/profiles')
      return {
        total: 1,
        list: [
          {
            id: 1,
            name: 'Reality 模板',
            protocol: 'vless-reality',
            port: 443,
            settings: '{}',
            active: true,
            remark: '',
          },
        ],
      }
    if (url === '/nodes') return { total: 2, list: overview.nodes }
    if (url.endsWith('/keys'))
      return [
        {
          profile_id: 1,
          node_id: Number(url.split('/')[2]),
          settings: '{}',
          port: 0,
          locked: false,
        },
      ]
    if (method === 'put' && url.endsWith('/keys/1')) return body
  })
  mount(<Profiles />)
  fireEvent.click((await screen.findAllByRole('button', { name: '密钥 / 锁定' }))[0])
  fireEvent.click(await screen.findByRole('checkbox', { name: '选择 lab-1' }))
  const port = await screen.findByLabelText(/监听端口（留空继承模板/)
  fireEvent.change(port, { target: { value: '24443' } })
  fireEvent.click(screen.getByRole('button', { name: '关闭' }))
  await screen.findByRole('dialog', { name: '放弃未保存的协议修改？' })
  fireEvent.click(screen.getByRole('button', { name: '继续编辑' }))
  await waitFor(() =>
    assert.equal(screen.queryByRole('dialog', { name: '放弃未保存的协议修改？' }), null),
  )
  fireEvent.click(screen.getByRole('button', { name: '保存密钥' }))
  await waitFor(() => assert.equal(calls.filter((c) => c.method === 'put').length, 1))
  await screen.findByText('已保存 1 个节点')
  const saved = calls.find((c) => c.method === 'put')!
  assert.equal(saved.url, '/nodes/1/keys/1')
  assert.deepEqual(saved.data, { settings: '{}', port: 24443 })
})

test('加载中的按钮不能被 disabled=false 重新启用，抽屉支持 Esc 和焦点返回', async () => {
  const button = render(
    <Btn loading disabled={false}>
      正在保存
    </Btn>,
  )
  assert.equal(
    (screen.getByRole('button', { name: '正在保存' }) as HTMLButtonElement).disabled,
    true,
  )
  button.unmount()
  function Example() {
    const [open, setOpen] = React.useState(false)
    return (
      <>
        <button onClick={() => setOpen(true)}>编辑配置</button>
        <Drawer open={open} title="测试抽屉" onClose={() => setOpen(false)}>
          <input aria-label="名称" />
        </Drawer>
      </>
    )
  }
  mount(<Example />)
  const opener = screen.getByRole('button', { name: '编辑配置' })
  opener.focus()
  fireEvent.click(opener)
  await screen.findByRole('dialog', { name: '测试抽屉' })
  fireEvent.keyDown(document, { key: 'Escape' })
  await waitFor(() => assert.equal(screen.queryByRole('dialog', { name: '测试抽屉' }), null))
  await waitFor(() => assert.equal(document.activeElement, opener))
})

test('Schema v3 双模式模板编辑器支持结构化与 JSON 双向无损同步、规则调序与出站引用禁删', async () => {
  api((_method, _url, body) =>
    body.draft ? { ...template, draft: JSON.stringify(body.draft), version: 5 } : undefined,
  )
  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)

  // 切换到自定义出站标签页并新增一个 SOCKS5 出站
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  fireEvent.click(screen.getByRole('button', { name: /添加自定义出站/ }))
  fireEvent.change(screen.getByLabelText('出站 Tag *'), { target: { value: 'hk-landing' } })
  fireEvent.change(screen.getByLabelText('落地服务器地址 *'), { target: { value: '203.0.113.10' } })

  // 切回路由规则标签页，添加两条规则并测试上下调序与引用联动
  fireEvent.click(screen.getByRole('tab', { name: /路由规则/ }))
  fireEvent.click(screen.getByRole('button', { name: '添加分流规则' }))
  fireEvent.change(screen.getByLabelText('规则 #1 备注'), { target: { value: '第一条规则' } })
  fireEvent.click(screen.getByRole('button', { name: '添加分流规则' }))
  fireEvent.change(screen.getByLabelText('规则 #2 备注'), { target: { value: '第二条规则' } })
  fireEvent.click(screen.getByRole('button', { name: '上移规则 #2' }))
  assert.equal((screen.getByLabelText('规则 #1 备注') as HTMLInputElement).value, '第二条规则')

  // 被规则引用的出站禁止删除
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  assert.equal(
    (screen.getByRole('button', { name: '使用中禁删' }) as HTMLButtonElement).disabled,
    true,
  )

  // 切换到全局策略 JSON 源码视图，注入未知扩展字段 mux 与 ruleTag，再切回结构化视图修改后保存
  fireEvent.click(screen.getByRole('tab', { name: '策略 JSON 源码' }))
  const jsonArea = screen.getByLabelText('策略 JSON 源码') as HTMLTextAreaElement
  const parsed = JSON.parse(jsonArea.value)
  parsed.domain_strategy = 'IPOnDemand'
  parsed.custom_outbounds[0].mux = { enabled: true, concurrency: 8 }
  parsed.custom_rules[0].ruleTag = 'custom-tag-1'
  fireEvent.change(jsonArea, { target: { value: JSON.stringify(parsed, null, 2) } })

  fireEvent.click(screen.getByRole('tab', { name: '结构化视图' }))
  fireEvent.click(screen.getByRole('tab', { name: /路由规则/ }))
  assert.equal(
    (screen.getByLabelText('路由域名解析策略（routing.domainStrategy）') as HTMLSelectElement).value,
    'IPOnDemand',
  )

  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const savedDraft = (
    calls.find((c) => c.method === 'patch')!.data as {
      draft: {
        schema_version: number
        domain_strategy: string
        custom_outbounds: Record<string, unknown>[]
        custom_rules: Record<string, unknown>[]
      }
    }
  ).draft
  assert.equal(savedDraft.schema_version, 3)
  assert.equal(savedDraft.domain_strategy, 'IPOnDemand')
  assert.deepEqual(savedDraft.custom_outbounds[0].mux, { enabled: true, concurrency: 8 })
  assert.equal(savedDraft.custom_rules[0].ruleTag, 'custom-tag-1')
})

test('节点绑定支持按出站 Tag 重映射与参数替换，并清理残留失效覆盖', async () => {
  const v3Policy = {
    schema_version: 3,
    domain_strategy: 'IPIfNonMatch',
    ipv4_force_domains: [],
    ipv6_force_domains: [],
    ipv6_failure_policy: 'strict',
    custom_outbounds: [
      {
        tag: 'hk-landing',
        protocol: 'socks',
        remark: '香港落地',
        settings: { servers: [{ address: '203.0.113.10', port: 1080 }] },
      },
      {
        tag: 'warp-egress',
        protocol: 'wireguard',
        remark: 'WARP',
        settings: {
          secretKey: 'enc:v1:00112233445566778899aabbccddeeff',
          peers: [{ endpoint: 'engage.cloudflareclient.com:2408', publicKey: 'pub' }],
        },
      },
    ],
    custom_rules: [{ type: 'field', domain: ['geosite:openai'], outboundTag: 'hk-landing' }],
  }
  api((method, url) => {
    if (method === 'get' && url.endsWith('/routing')) {
      return {
        ...binding,
        overrides: JSON.stringify({
          outbound_overrides: {
            'obsolete-tag': { mode: 'remap', target_tag: 'direct' },
          },
        }),
      }
    }
    if (url.endsWith('/routing/template-preview')) {
      return { ...overview.nodes[0].routing, policy: v3Policy }
    }
  })

  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  // 清理模板已移除的残留出站覆盖
  fireEvent.click(await screen.findByRole('button', { name: '清理失效出站覆盖' }))

  // 将 warp-egress 重映射为 direct-v4，将 hk-landing 替换为本节点自定义地址
  fireEvent.change(screen.getByLabelText('出站覆盖模式 · warp-egress'), {
    target: { value: 'remap' },
  })
  fireEvent.change(screen.getByLabelText('重映射目标出站 · warp-egress'), {
    target: { value: 'direct-v4' },
  })
  fireEvent.change(screen.getByLabelText('出站覆盖模式 · hk-landing'), {
    target: { value: 'replace' },
  })
  fireEvent.change(screen.getByLabelText('落地服务器地址 *'), {
    target: { value: '198.51.100.88' },
  })

  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  const savedOverrides = JSON.parse(
    (calls.find((c) => c.method === 'put')!.data as RoutingBinding).overrides,
  )
  assert.equal('obsolete-tag' in savedOverrides.outbound_overrides, false)
  assert.deepEqual(savedOverrides.outbound_overrides['warp-egress'], {
    mode: 'remap',
    target_tag: 'direct-v4',
  })
  assert.equal(
    savedOverrides.outbound_overrides['hk-landing'].outbound.settings.servers[0].address,
    '198.51.100.88',
  )
})

test('Phase 2 场景片段库即插即用与未发布草稿远端沙箱预检闭环', async () => {
  api((method, url, body) => {
    if (method === 'post' && url === '/nodes/1/preflight-draft') {
      const policy = body.policy as {
        custom_outbounds: { tag: string }[]
        custom_rules: { domain?: string[]; ip?: string[]; outboundTag: string }[]
      }
      assert.equal(policy.custom_outbounds[0].tag, 'warp-egress')
      assert.equal(policy.custom_rules.length, 2)
      assert.equal(policy.custom_rules[0].outboundTag, 'warp-egress')
      return {
        valid: true,
        xray_version: '26.3.27',
        requires_ipv6: false,
        elapsed_ms: 420,
        warnings: [],
        redacted_config: '{\n  "outbounds": [{"tag": "warp-egress"}]\n}',
        error: '',
      }
    }
  })

  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)

  // 1. 插入 Cloudflare WARP 出站片段并补填 secretKey
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  fireEvent.click(screen.getByRole('button', { name: /场景出站片段/ }))
  fireEvent.click(screen.getByRole('button', { name: /Cloudflare WARP/ }))
  fireEvent.change(screen.getByLabelText(/本机私钥/), {
    target: { value: 'test-wg-secret' },
  })

  // 2. 插入 Telegram 域名与 IP 配套规则片段并选择目标出站 warp-egress
  fireEvent.click(screen.getByRole('tab', { name: /路由规则/ }))
  fireEvent.click(screen.getByRole('button', { name: /插入规则片段/ }))
  fireEvent.click(screen.getByRole('button', { name: /Telegram 域名与 IP 集合/ }))
  const modal = await screen.findByRole('dialog', { name: /插入规则片段 · Telegram/ })
  assert.equal(
    (within(modal).getByLabelText(/选择片段命中的目标出站/) as HTMLSelectElement).value,
    'warp-egress',
  )
  fireEvent.click(within(modal).getByRole('button', { name: '确认插入规则' }))
  assert.ok(screen.getByLabelText('规则 #2 备注'))

  // 3. 在未保存、未发布状态下直接发起远端节点沙箱预检
  fireEvent.click(screen.getByRole('button', { name: /节点沙箱预检/ }))
  const pfModal = await screen.findByRole('dialog', { name: '未发布草稿远端沙箱预检' })
  fireEvent.click(within(pfModal).getByRole('button', { name: /运行远端沙箱预检/ }))

  await within(pfModal).findByText('远端 Xray 沙箱预检通过')
  assert.ok(within(pfModal).getByText('Xray 26.3.27'))
  assert.match(
    (within(pfModal).getByLabelText('沙箱预检合成配置预览') as HTMLTextAreaElement).value,
    /warp-egress/,
  )
  // 确认未产生任何模板保存或绑定写入请求
  assert.equal(
    calls.filter((c) => c.method === 'post' && c.url !== '/nodes/1/preflight-draft').length,
    0,
  )
  assert.equal(calls.filter((c) => c.method === 'patch' || c.method === 'put').length, 0)
})

test('节点路由绑定抽屉支持在保存覆盖前直接对本节点执行沙箱预检', async () => {
  api((method, url, body) => {
    if (method === 'get' && url.endsWith('/routing')) {
      return { ...binding, overrides: '{}' }
    }
    if (method === 'post' && url === '/nodes/1/preflight-draft') {
      const customOv = JSON.parse(body.custom_overrides as string)
      assert.deepEqual(customOv, { ipv4_force_domains: ['domain:sandbox-before-save.example'] })
      return {
        valid: false,
        xray_version: '26.3.27',
        requires_ipv6: false,
        elapsed_ms: 310,
        warnings: [],
        redacted_config: '{\n  "routing": {}\n}',
        error: 'PREFLIGHT_FAILED: geosite.dat missing entry',
      }
    }
  })

  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  await screen.findByLabelText('IPv4 模板域名')
  fireEvent.change(screen.getByLabelText('强制 IPv4 域名'), { target: { value: 'custom' } })
  fireEvent.change(screen.getByLabelText('IPv4 覆盖域名'), {
    target: { value: 'domain:sandbox-before-save.example' },
  })

  fireEvent.click(screen.getByRole('button', { name: /沙箱预检/ }))
  const pfModal = await screen.findByRole('dialog', { name: '远端沙箱预检 · lab-1' })
  fireEvent.click(within(pfModal).getByRole('button', { name: /运行远端沙箱预检/ }))

  await within(pfModal).findByText('沙箱预检未通过')
  assert.ok(within(pfModal).getByText(/geosite\.dat missing entry/))
  assert.equal(calls.filter((c) => c.method === 'put').length, 0)
})

test('Phase 3 模板编辑器支持负载均衡组、Observatory 探测、服务端 DNS 分流与规则均衡组目标联动', async () => {
  api((_method, _url, body) =>
    body.draft ? { ...template, draft: JSON.stringify(body.draft), version: 5 } : undefined,
  )
  mount(<TemplateEditor template={template} onClose={() => {}} onConfigure={() => {}} />)

  // 1. 先创建两个候选自定义出站 hk-1 与 hk-2
  fireEvent.click(screen.getByRole('tab', { name: /自定义出站/ }))
  fireEvent.click(screen.getByRole('button', { name: /添加自定义出站/ }))
  fireEvent.change(screen.getByLabelText('出站 Tag *'), { target: { value: 'hk-1' } })
  fireEvent.change(screen.getByLabelText('落地服务器地址 *'), { target: { value: '203.0.113.11' } })

  fireEvent.click(screen.getByRole('button', { name: /添加自定义出站/ }))
  fireEvent.change(screen.getAllByLabelText('出站 Tag *')[1], { target: { value: 'hk-2' } })
  fireEvent.change(screen.getAllByLabelText('落地服务器地址 *')[1], {
    target: { value: '203.0.113.12' },
  })

  // 2. 切换到「负载均衡」标签页，创建 hk-pool 并勾选 hk-2，验证 Observatory 自动展开与自定义模式
  fireEvent.click(screen.getByRole('tab', { name: /负载均衡/ }))
  fireEvent.click(screen.getByRole('button', { name: /添加负载均衡组/ }))
  fireEvent.change(screen.getByLabelText('均衡组 Tag *'), { target: { value: 'hk-pool' } })
  fireEvent.click(screen.getByRole('button', { name: /hk-2/ }))
  fireEvent.change(screen.getByLabelText('故障回退兜底出站（fallbackTag，可选）'), {
    target: { value: 'direct' },
  })

  // leastPing 默认自动展开健康探测（Observatory）设置
  assert.ok(screen.getByText('健康探测（Observatory）设置'))
  fireEvent.change(screen.getByLabelText('探测参数模式'), { target: { value: 'custom' } })
  fireEvent.change(screen.getByLabelText(/探测周期/), { target: { value: '30s' } })

  // 3. 切换到「DNS 设置」标签页，启用自定义 DNS，添加分域名解锁规则与静态 Hosts
  fireEvent.click(screen.getByRole('tab', { name: /DNS 设置/ }))
  assert.ok(
    screen.getByText(
      '启用自定义 DNS 后，默认直连出站将自动使用 Xray 内置解析结果以使分流 DNS 与 Hosts 生效。',
    ),
  )
  fireEvent.click(screen.getByRole('switch'))
  fireEvent.click(screen.getByRole('button', { name: /添加分域名解锁 DNS 规则/ }))
  fireEvent.change(screen.getByLabelText('上游 DNS 地址 #2 *'), {
    target: { value: '8.8.8.8' },
  })
  fireEvent.click(screen.getByRole('button', { name: /添加静态 Hosts 映射/ }))
  fireEvent.change(screen.getByLabelText('Hosts 域名键 #1'), {
    target: { value: 'domain:internal.example' },
  })
  fireEvent.change(screen.getByLabelText('映射目标 IP #1'), {
    target: { value: '10.0.0.8' },
  })

  // 4. 切回「路由规则」标签页，添加规则并将目标选为负载均衡组 hk-pool
  fireEvent.click(screen.getByRole('tab', { name: /路由规则/ }))
  fireEvent.click(screen.getByRole('button', { name: '添加分流规则' }))
  fireEvent.change(screen.getByLabelText('目标出站 / 均衡组 *'), {
    target: { value: 'balancer:hk-pool' },
  })

  // 5. 保存草稿并验证提交的 Schema v3 完整载荷
  fireEvent.click(screen.getByRole('button', { name: '保存草稿' }))
  await screen.findByText('草稿已保存。')
  const savedDraft = (
    calls.find((c) => c.method === 'patch')!.data as {
      draft: {
        balancers: {
          tag: string
          selector: string[]
          strategy: { type: string }
          fallbackTag: string
        }[]
        observatory: { probeInterval: string; subjectSelector: string[] }
        dns: {
          enabled: boolean
          servers: (string | { address: string; domains: string[] })[]
          hosts: Record<string, string>
        }
        custom_rules: { outboundTag?: string; balancerTag?: string }[]
      }
    }
  ).draft

  assert.equal(savedDraft.balancers.length, 1)
  assert.equal(savedDraft.balancers[0].tag, 'hk-pool')
  assert.deepEqual(savedDraft.balancers[0].selector, ['hk-1', 'hk-2'])
  assert.equal(savedDraft.balancers[0].fallbackTag, 'direct')
  assert.equal(savedDraft.observatory.probeInterval, '30s')
  assert.deepEqual(savedDraft.observatory.subjectSelector, ['hk-1', 'hk-2'])
  assert.equal(savedDraft.dns.enabled, true)
  assert.equal(savedDraft.dns.servers.length, 2)
  assert.equal(savedDraft.dns.hosts['domain:internal.example'], '10.0.0.8')
  assert.equal(savedDraft.custom_rules[0].balancerTag, 'hk-pool')
  assert.equal(savedDraft.custom_rules[0].outboundTag, undefined)
})

test('节点路由绑定抽屉支持本节点服务端 DNS 三态覆盖（继承 / 关闭 / 自定义替换）', async () => {
  const v3DnsPolicy = {
    schema_version: 3,
    domain_strategy: 'IPIfNonMatch',
    ipv4_force_domains: [],
    ipv6_force_domains: [],
    ipv6_failure_policy: 'strict',
    custom_outbounds: [],
    dns: {
      enabled: true,
      queryStrategy: 'UseIPv4',
      servers: ['https://1.1.1.1/dns-query'],
    },
    custom_rules: [],
  }
  api((method, url) => {
    if (method === 'get' && url.endsWith('/routing')) {
      return { ...binding, overrides: '{}' }
    }
    if (url.endsWith('/routing/template-preview')) {
      return { ...overview.nodes[0].routing, policy: v3DnsPolicy }
    }
  })

  mount(<RoutingBindingDrawer scope="node" id={1} name="lab-1" onClose={() => {}} />)
  await screen.findByText(/当前继承模板已启用的自定义 DNS/)

  // 切换为单节点关闭自定义 DNS
  fireEvent.change(screen.getByLabelText('本节点 DNS 模式'), { target: { value: 'disable' } })
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  const firstSaved = JSON.parse(
    (calls.filter((c) => c.method === 'put')[0].data as RoutingBinding).overrides,
  )
  assert.deepEqual(firstSaved.dns_override, { enabled: false })

  // 切换为自定义本节点解锁 DNS IP
  fireEvent.change(screen.getByLabelText('本节点 DNS 模式'), { target: { value: 'custom' } })
  fireEvent.change(screen.getByLabelText('上游 DNS 地址 #1 *'), {
    target: { value: '9.9.9.9' },
  })
  fireEvent.click(screen.getByRole('button', { name: '保存路由' }))
  await screen.findByText(/已保存：1 个节点/)
  const putCalls = calls.filter((c) => c.method === 'put')
  const secondSaved = JSON.parse((putCalls[putCalls.length - 1].data as RoutingBinding).overrides)
  assert.equal(secondSaved.dns_override.enabled, true)
  assert.deepEqual(secondSaved.dns_override.servers, ['9.9.9.9'])
})

test('前端 requiresIPv6 与后端共享测试集保持逐项一致', () => {
  const raw = readFileSync(
    new URL('../../internal/xray/testdata/ipv6_parity_cases.json', import.meta.url),
    'utf8',
  )
  const cases = JSON.parse(raw) as { name: string; want: boolean; policy: RoutingPolicy }[]
  assert.ok(cases.length >= 12)
  for (const tc of cases) {
    assert.equal(
      requiresIPv6(tc.policy),
      tc.want,
      `case ${tc.name} mismatch: expected ${tc.want}`,
    )
  }

  // 自动生成的 Observatory 继承 IPv6 默认探针与非公网/非 443 回退均与后端完全一致
  const autoObsPolicy: RoutingPolicy = {
    schema_version: 3,
    ipv4_force_domains: [],
    ipv6_force_domains: [],
    ipv6_failure_policy: 'strict',
    balancers: [{ tag: 'pool', selector: ['direct-v4'], strategy: { type: 'leastPing' } }],
    custom_rules: [{ type: 'field', port: '443', balancerTag: 'pool' }],
  }
  assert.equal(requiresIPv6(autoObsPolicy), false)
  assert.equal(requiresIPv6(autoObsPolicy, 'https://[2606:4700:4700::1111]/'), true)
  assert.equal(requiresIPv6(autoObsPolicy, 'https://[2606:4700:4700::1111]:443/probe'), true)

  const urlCases = JSON.parse(
    readFileSync(
      new URL('../../internal/xray/testdata/observatory_url_cases.json', import.meta.url),
      'utf8',
    ),
  ) as { valid: string[]; invalid: string[] }
  for (const validURL of urlCases.valid) {
    assert.equal(isValidObservatoryProbeURL(validURL), true, `expected valid probe URL: ${validURL}`)
    assert.equal(
      requiresIPv6(autoObsPolicy, validURL),
      validURL.includes('['),
      `expected requiresIPv6 parity for valid probe URL: ${validURL}`,
    )
  }
  for (const invalidURL of urlCases.invalid) {
    assert.equal(
      isValidObservatoryProbeURL(invalidURL),
      false,
      `expected invalid probe URL: ${invalidURL}`,
    )
    // 任何不合法探针（含 ULA fc00::/7、文档前缀 2001:db8::/32、NAT64 64:ff9b::/96、非 443 端口）必须回退到安全 IPv4 默认值
    assert.equal(
      requiresIPv6(autoObsPolicy, invalidURL),
      false,
      `invalid probe URL ${invalidURL} must fall back to IPv4 default`,
    )
  }
})
