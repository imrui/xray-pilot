// @vitest-environment jsdom
import { afterEach, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import Nodes from '@/pages/Nodes'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'
import request from '@/lib/axios'
import { useAuthStore } from '@/store/auth'

const rawHash = 'a'.repeat(64)
const adminNotice = '远端配置与纳管基线不一致，请联系超级管理员核对并接管基线'
const originalAdapter = request.defaults.adapter
let client: QueryClient
afterEach(() => {
  cleanup()
  client.clear()
  request.defaults.adapter = originalAdapter
})

function mountNodes(reason: string, role: 'super_admin' | 'admin' = 'super_admin', initialCheck?: string) {
  useAuthStore.getState().setSession('test', role, role)
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const errorAt = '2026-11-19T10:00:00.500Z'
  let checked = initialCheck
  let stale = false
  const post = vi.fn()
  const getData = (url: string, body: string) => {
    const node = { id: 7, name: '台北节点', ip: '192.0.2.7', active: true, sync_status: 'failed',
      last_check_at: checked, last_check_ok: !!checked, last_sync_at: errorAt }
    if (url === '/nodes') return { list: [node], total: 1 }
    if (url === '/profiles') return { list: [], total: 0 }
    if (url === '/nodes/7/keys') return []
    if (url === '/nodes/filter-options') return { regions: [], owners: [] }
    if (url === '/routing-overview') return { templates: [], nodes: [{ ...node, routing: null, last_sync_error: reason, last_sync_error_at: errorAt }] }
    if (url === '/nodes/7/config-state') return { last_error: reason }
    if (url.startsWith('/config-deployments?')) return { total: 1, list: [
      { id: 41, node_id: 7, error: reason, status: 'failed', actor: 'admin:test', created_at: errorAt },
    ] }
    post(url, body ? JSON.parse(body) : undefined)
    if (url === '/nodes/7/test-ssh') {
      checked = errorAt
      return { ok: true, latency_ms: 4 }
    }
    if (url === '/nodes/7/inspect-baseline') return {
      config_path: '/etc/xray/custom.json', observed_raw_sha256: rawHash, observed_hash: 'remote-hash',
      recorded_baseline_hash: 'recorded-hash', desired_hash: 'desired-hash', baseline_matched: false,
      remote_valid_json: true, remote_bytes: 90, remote_redacted: '{"id":"<masked>"}', desired_redacted: '{"password":"<masked>"}',
    }
    if (url === '/nodes/7/adopt-baseline') {
      if (stale) throw new Error('远端文件在核对后已发生变化，请重新核对')
      return [{ id: 42 }]
    }
    throw new Error(`意外请求 ${url}`)
  }
  request.defaults.adapter = async config => ({
    config, data: { code: 0, data: getData(config.url!, config.data) }, headers: {}, status: 200, statusText: 'OK',
  })
  render(<QueryClientProvider client={client}><MemoryRouter><ConfirmProvider><Nodes /></ConfirmProvider></MemoryRouter></QueryClientProvider>)
  return { post, changeRemote: () => { stale = true } }
}

test.each(['PREFLIGHT_FAILED', 'IPV6_UNAVAILABLE', 'VERSION_UNSUPPORTED', 'PATH_MISMATCH', 'ROLLED_BACK',
  'ROLLBACK_FAILED', 'AUTHORIZATION_STOPPED', 'HEALTH_FAILED', 'HEALTH_PROBE_UNAVAILABLE', '生成配置失败', '协议配置不完整'])(
  '%s 引导管理员查看对应节点的同步记录', async reason => {
    mountNodes(reason)
    fireEvent.click(await screen.findByRole('button', { name: '查看失败原因' }))
    const history = await screen.findByRole('dialog', { name: '节点同步记录' })
    expect(await within(history).findByText(reason)).toBeTruthy()
    expect(screen.queryByText('建议先测试 SSH')).toBeNull()
  })

test.each([undefined, '2026-11-19T10:00:00.499Z'])('未检测或旧的成功记录 %s 在新 SSH 检测成功后更新提示', async check => {
  const { post } = mountNodes('SSH 连接失败', 'admin', check)
  fireEvent.click(await screen.findByRole('button', { name: '建议先测试 SSH' }))
  expect(await screen.findByRole('button', { name: '查看失败原因' })).toBeTruthy()
  expect(screen.queryByText('建议先测试 SSH')).toBeNull()
  expect(post).toHaveBeenCalledWith('/nodes/7/test-ssh', undefined)
})

test('超级管理员从同步记录核对远端基线，取消后可以再次确认接管', async () => {
  const { post } = mountNodes('DRIFT_CONFLICT')
  await screen.findByRole('button', { name: '基线冲突，去核对' })
  expect(screen.queryByText('建议先测试 SSH')).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '同步记录' }))
  fireEvent.click(await screen.findByRole('button', { name: '去核对远端基线' }))
  const diagnostics = await screen.findByRole('dialog', { name: '节点诊断与恢复 · 台北节点' })
  expect(screen.queryByRole('dialog', { name: '节点同步记录' })).toBeNull()
  fireEvent.click(within(diagnostics).getByRole('button', { name: '核对远端配置' }))
  expect(await screen.findByText('/etc/xray/custom.json')).toBeTruthy()
  for (const hash of [rawHash, 'remote-hash', 'recorded-hash', 'desired-hash']) expect(screen.getByText(hash)).toBeTruthy()
  fireEvent.click(screen.getByText('对比脱敏配置'))
  expect(screen.getByText('{"id":"<masked>"}')).toBeTruthy()
  expect(screen.getByText('{"password":"<masked>"}')).toBeTruthy()
  const adopt = screen.getByRole('button', { name: '确认以面板配置覆盖远端' })
  fireEvent.click(adopt)
  fireEvent.click(await screen.findByRole('button', { name: '取消' }))
  expect(post.mock.calls.map(call => call[0])).toEqual(['/nodes/7/inspect-baseline'])
  fireEvent.click(adopt)
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  fireEvent.click(await screen.findByRole('button', { name: '确认覆盖并同步' }))
  await screen.findByText(/同步任务 #42 已入队/)
  expect(post).toHaveBeenCalledWith('/nodes/7/adopt-baseline', { observed_raw_sha256: rawHash, request_key: expect.stringMatching(/^.{8,100}$/) })
  for (const key of ['routing-overview', 'routing-jobs', 'nodes', 'config-state']) expect(invalidate).toHaveBeenCalledWith({ queryKey: [key] })
})

test('远端字节有变动时清除已核对内容，要求重新核对', async () => {
  const { changeRemote } = mountNodes('DRIFT_CONFLICT')
  fireEvent.click(await screen.findByRole('button', { name: '基线冲突，去核对' }))
  fireEvent.click(await screen.findByRole('button', { name: '核对远端配置' }))
  fireEvent.click(await screen.findByRole('button', { name: '确认以面板配置覆盖远端' }))
  changeRemote()
  fireEvent.click(await screen.findByRole('button', { name: '确认覆盖并同步' }))
  await screen.findByText('远端文件在核对后已发生变化，请重新核对')
  expect(screen.queryByRole('button', { name: '确认以面板配置覆盖远端' })).toBeNull()
  expect(screen.getByRole('button', { name: '核对远端配置' })).toBeTruthy()
})

test('普通管理员在诊断和同步记录中看到接管权限提示', async () => {
  const { post } = mountNodes('DRIFT_CONFLICT', 'admin')
  fireEvent.click(await screen.findByRole('button', { name: '基线冲突，去核对' }))
  expect(await screen.findByText(adminNotice)).toBeTruthy()
  expect(screen.queryByRole('button', { name: '核对远端配置' })).toBeNull()
  fireEvent.click(screen.getByRole('button', { name: '关闭抽屉' }))
  fireEvent.click(screen.getByRole('button', { name: '同步记录' }))
  await waitFor(() => expect(screen.getByText(adminNotice)).toBeTruthy())
  expect(screen.queryByRole('button', { name: '用最新配置重试' })).toBeNull()
  expect(screen.queryByRole('button', { name: '去核对远端基线' })).toBeNull()
  expect(post).not.toHaveBeenCalled()
})
