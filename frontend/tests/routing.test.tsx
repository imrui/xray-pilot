import './dom-setup.mjs'
import React from 'react'
import { test, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import { render, screen, fireEvent, waitFor, cleanup, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import { ConfirmProvider } from '../src/components/ui/ConfirmProvider'
import Routing from '../src/pages/Routing'
import Profiles from '../src/pages/Profiles'
import { Btn } from '../src/components/ui/Form'
import { Drawer } from '../src/components/ui/Drawer'
import { TemplateNodes, TemplateEditor } from '../src/components/routing/Templates'
import { RoutingBindingDrawer } from '../src/components/routing/BindingEditor'
import { SyncHistory } from '../src/components/routing/SyncTools'
import { submitNodeSync } from '../src/lib/node-sync'
import { requestID } from '../src/lib/request-id'
import { useAuthStore } from '../src/store/auth'
import request from '../src/lib/axios'
import {
  emptyPolicy,
  configConfirmed,
  normalizeBinding,
  upgradeRoutingPolicy,
  requiresIPv6,
} from '../src/lib/routing-model'
import type { RoutingBinding, RoutingTemplate } from '../src/lib/routing-model'

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
  udp443_policy: 'block',
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
    let data = handler?.(method, url, body)
    if (data === undefined) {
      if (url === '/routing-overview') data = overview
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
  assert.equal(payload.schema_version, 2)
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
  assert.equal(converted.schema_version, 2)
  const unversioned = upgradeRoutingPolicy(
    JSON.stringify({ ipv6_force_domains: ['domain:old.example'], ipv6_failure_policy: 'strict' }),
  )
  assert.deepEqual(unversioned.ipv4_force_domains, ['domain:old.example'])
  assert.deepEqual(unversioned.ipv6_force_domains, [])
  assert.equal(requiresIPv6(converted), false)
  assert.equal(requiresIPv6({ ...emptyPolicy, ipv6_force_domains: ['domain:v6.example'] }), true)
  assert.equal(requiresIPv6({ ...emptyPolicy, custom_rules: [{ outboundTag: 'direct-v6' }] }), true)
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
  assert.equal(bodies[0].udp443_policy, 'block')
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
  const mode = await screen.findByLabelText('配置方式')
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
  assert.equal((screen.getByLabelText('配置方式') as HTMLSelectElement).value, 'template')
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
