import './dom-setup.mjs'
import React from 'react'
import { afterEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'
import request from '../src/lib/axios'
import Dashboard from '../src/pages/Dashboard'
import { TrafficChart } from '../src/components/traffic/TrafficChart'
import { TrafficRankingChart } from '../src/components/traffic/TrafficRankingChart'
import { TrafficStatsInfo } from '../src/components/traffic/TrafficStatsInfo'
import type { TrafficAnalytics } from '../src/types/traffic'

let chartWidth = 1100
Object.defineProperty(globalThis, 'ResizeObserver', { configurable: true, value: class {
  constructor(private callback: (entries: unknown[]) => void) {}
  observe() { this.callback([{ contentRect: { width: chartWidth } }]) }
  unobserve() {}
  disconnect() {}
} })
let clients: QueryClient[] = []
afterEach(() => { cleanup(); clients.forEach(client => client.clear()); clients = []; chartWidth = 1100 })

const analytics: TrafficAnalytics = {
  start: '2026-11-10T16:00:00Z', end: '2026-11-17T05:00:00Z', timezone: 'Asia/Shanghai', interval: 'day',
  up_bytes: 2048, down_bytes: 4096, active_users: 19, active_nodes: 10,
  enabled_nodes: 9, total_nodes: 9, total_users: 24, enabled_users: 24, healthy_nodes: 8,
  successful_polls: 100, failed_polls: 2, last_poll_at: null, retention_start: null, truncated: false,
  nodes: [{ id: 1, name: 'tw02' }], users: [{ id: 2, name: 'alice' }],
  ranking: [{ id: 1, name: 'tw02', up_bytes: 2048, down_bytes: 4096 }],
  series: [{ id: '1', name: 'tw02', points: [{ time: '2026-11-10T16:00:00Z', up_bytes: 1024, down_bytes: 2048 }, { time: '2026-11-11T16:00:00Z', up_bytes: null, down_bytes: null }, { time: '2026-11-12T16:00:00Z', up_bytes: 1024, down_bytes: 2048 }] }],
}

function mountDashboard(failAnalytics = () => false) {
  const calls: { url: string; params: Record<string, unknown> }[] = []
  request.defaults.adapter = async config => {
    const url = config.url ?? ''
    calls.push({ url, params: config.params ?? {} })
    let data: unknown
    if (url === '/traffic/analytics') {
      if (failAnalytics()) throw new Error('test analytics unavailable')
      data = { ...analytics, ...(config.params.dimension === 'user' ? { ranking: [{ id: 2, name: 'alice', up_bytes: 2048, down_bytes: 4096 }] } : {}) }
    }
    else if (url === '/nodes') {
      const all = Array.from({ length: 9 }, (_, i) => ({ id: i + 1, name: `health-${i + 1}`, active: true, sync_status: 'synced', online_user_count: i, last_check_at: '2026-11-17T05:00:00Z', last_check_ok: true, last_latency_ms: 21 }))
      data = { total: 9, list: config.params.page === 1 ? all.slice(0, 5) : all.slice(5) }
    } else if (url === '/logs') data = { total: 6, list: Array.from({ length: 6 }, (_, i) => ({ id: i + 1, action: `test-action-${i + 1}`, target: 'node:tw02', message: i === 5 ? 'long_error_without_spaces_'.repeat(8) : '同步成功', success: i !== 5, created_at: '2026-11-17T05:00:00Z' })) }
    else throw new Error(`Unexpected request: ${url}`)
    return { config, data: { code: 0, data }, status: 200, statusText: 'OK', headers: {} }
  }
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  clients.push(client)
  const view = render(<QueryClientProvider client={client}><MemoryRouter><Dashboard /></MemoryRouter></QueryClientProvider>)
  return { ...view, calls }
}

test('汇总合并为八张卡片，日期、筛选、刷新按顺序展示，图表与健康列表完整加载', async () => {
  const { container, calls } = mountDashboard()
  await screen.findByRole('heading', { name: '节点用量排名' })
  await screen.findByText('health-9')
  const summary = screen.getByLabelText('运行与区间统计')
  assert.equal(within(summary).getAllByRole('heading').length, 8)
  assert.ok(within(summary).getByText('8 / 9'))
  assert.ok(within(summary).getByText('24 / 24'))
  assert.ok(within(summary).getByText('6.00 KiB'))
  assert.equal(screen.queryByText('运行概况'), null)
  const filters = screen.getByLabelText('统计筛选')
  assert.ok(filters.compareDocumentPosition(summary) & Node.DOCUMENT_POSITION_FOLLOWING)
  const date = within(filters).getByRole('button', { name: /选择日期范围/ })
  const node = within(filters).getByRole('combobox', { name: '筛选节点' })
  const refresh = within(filters).getByRole('button', { name: '刷新' })
  assert.ok(date.compareDocumentPosition(node) & Node.DOCUMENT_POSITION_FOLLOWING)
  assert.ok(node.compareDocumentPosition(refresh) & Node.DOCUMENT_POSITION_FOLLOWING)
  assert.ok(screen.getByRole('group', { name: /节点用量排名柱状图/ }))
  assert.ok(screen.getByRole('complementary', { name: '当前节点健康' }))
  assert.equal(container.querySelectorAll('table').length, 0)
  assert.ok(screen.getByText('test-action-6'))
  assert.equal(calls.filter(call => call.url === '/nodes').length, 2)
  for (const title of ['最近操作与异常', '节点健康']) {
    const card = screen.getByRole('heading', { name: title }).parentElement!.parentElement!
    assert.equal(card.querySelector('[class*="overflow-auto"], [class*="overflow-y-auto"]'), null)
    assert.ok(!card.className.includes('h-[400px]'))
  }
})

test('统计请求失败时健康与操作记录仍可见，重试可恢复卡片与图表', async () => {
  let failed = true
  mountDashboard(() => failed)
  await screen.findByRole('alert')
  await screen.findByText('health-9')
  assert.ok(screen.getByText('test-action-6'))
  failed = false
  fireEvent.click(screen.getByRole('button', { name: '重试' }))
  await screen.findByRole('button', { name: /^tw02，上行/ })
  assert.equal(screen.queryByRole('alert'), null)
  assert.ok(within(screen.getByLabelText('运行与区间统计')).getByText('6.00 KiB'))
})

test('柱形点击切换节点和用户明细，方向排序和日期快捷选项保留', async () => {
  const { calls } = mountDashboard()
  fireEvent.click(await screen.findByRole('button', { name: /^tw02，上行/ }))
  await screen.findByRole('heading', { name: '用户用量排名' })
  await waitFor(() => assert.ok(calls.some(call => call.url === '/traffic/analytics' && call.params.node_id === 1 && call.params.dimension === 'user')))
  assert.equal((screen.getByRole('combobox', { name: '筛选节点' }) as HTMLSelectElement).value, '1')
  fireEvent.change(screen.getByRole('combobox', { name: '流量方向' }), { target: { value: 'down' } })
  assert.ok(screen.getByText(/按下行降序/))
  fireEvent.click(screen.getByRole('button', { name: '清除筛选' }))
  await waitFor(() => assert.equal((screen.getByRole('combobox', { name: '筛选节点' }) as HTMLSelectElement).value, ''))
  fireEvent.click(screen.getByRole('button', { name: '自定义' }))
  const dialog = screen.getByRole('dialog', { name: '选择日期范围' })
  assert.ok(within(dialog).getByRole('button', { name: '昨天' }))
  fireEvent.click(within(dialog).getByRole('button', { name: '近 30 天' }))
  fireEvent.click(within(dialog).getByRole('button', { name: '应用' }))
  await waitFor(() => assert.equal(screen.queryByRole('dialog'), null))
  assert.equal(screen.getByRole('button', { name: '近 30 天' }).getAttribute('aria-pressed'), 'true')
})

test('趋势完整日期使用 UTC+8，小时补充时间，缺采断点保留', () => {
  const { container, rerender } = render(<TrafficChart series={analytics.series} direction="total" interval="day" />)
  assert.ok(screen.getByText('2026-11-11'))
  assert.ok(screen.getByText('2026-11-13'))
  const path = container.querySelector('path')!.getAttribute('d')!
  assert.equal((path.match(/M/g) ?? []).length, 2)
  assert.equal((path.match(/L/g) ?? []).length, 0)
  rerender(<TrafficChart series={analytics.series} direction="total" interval="hour" />)
  assert.ok(screen.getAllByText('00:00').length > 0)
})

test('统计说明点击后以浮层展示，支持 Esc、关闭按钮、重复点击与外部点击关闭', async () => {
  const { container } = mountDashboard()
  const trigger = await screen.findByRole('button', { name: '统计说明 2 次采集失败' })
  assert.equal(trigger.getAttribute('aria-expanded'), 'false')
  fireEvent.mouseEnter(trigger)
  assert.equal(screen.queryByRole('dialog', { name: '统计说明' }), null)
  fireEvent.click(trigger)
  let dialog = await screen.findByRole('dialog', { name: '统计说明' })
  assert.equal(trigger.getAttribute('aria-expanded'), 'true')
  // The content is portalled outside the dashboard, so it cannot expand the chart card.
  assert.equal(container.contains(dialog), false)
  assert.ok(within(dialog).getByText(/成功 100 次，失败 2 次/))
  assert.ok(within(dialog).getByText(/历史缺采无法补算/))
  fireEvent.keyDown(dialog, { key: 'Escape' })
  await waitFor(() => assert.equal(screen.queryByRole('dialog', { name: '统计说明' }), null))
  await waitFor(() => assert.equal(document.activeElement, trigger))
  fireEvent.click(trigger)
  dialog = await screen.findByRole('dialog', { name: '统计说明' })
  fireEvent.click(within(dialog).getByRole('button', { name: '关闭统计说明' }))
  await waitFor(() => assert.equal(trigger.getAttribute('aria-expanded'), 'false'))
  fireEvent.click(trigger)
  await screen.findByRole('dialog', { name: '统计说明' })
  fireEvent.click(trigger)
  await waitFor(() => assert.equal(trigger.getAttribute('aria-expanded'), 'false'))
  fireEvent.click(trigger)
  await screen.findByRole('dialog', { name: '统计说明' })
  // DismissableLayer installs its outside-pointer listener on the next task.
  await new Promise(resolve => setTimeout(resolve, 0))
  fireEvent.pointerDown(screen.getByRole('heading', { name: '节点用量排名' }), { pointerType: 'mouse' })
  await waitFor(() => assert.equal(screen.queryByRole('dialog', { name: '统计说明' }), null))
})

test('统计说明正确展示小时粒度与最近采集，零失败和加载中不显示错误计数', async () => {
  const { rerender } = render(<TrafficStatsInfo data={{ ...analytics, interval: 'hour', failed_polls: 0, last_poll_at: '2026-11-17T05:00:00Z' }} />)
  const trigger = screen.getByRole('button', { name: '统计说明' })
  fireEvent.click(trigger)
  const dialog = await screen.findByRole('dialog', { name: '统计说明' })
  assert.ok(within(dialog).getByText(/^按小时汇总/))
  assert.ok(within(dialog).getByText(/成功 100 次，失败 0 次/))
  assert.ok(within(dialog).getByText(/最近成功采集：.*13:00:00/))
  rerender(<TrafficStatsInfo />)
  assert.ok(screen.getByRole('button', { name: '统计说明' }))
  assert.equal(within(dialog).queryByText(/区间采集/), null)
})

test('窄屏排名分页不丢对象，长名称可读且支持键盘选择，零用量安全显示', () => {
  chartWidth = 280
  const rows = Array.from({ length: 8 }, (_, i) => ({ id: i + 1, name: `long-node-name-${i + 1}`, up_bytes: i * 1024, down_bytes: (8 - i) * 1024 }))
  let selected = 0
  const { container, rerender } = render(<TrafficRankingChart rows={rows} dimension="node" direction="up" onSelect={id => { selected = id }} />)
  const chart = screen.getByRole('group', { name: /节点用量排名/ })
  assert.equal(within(chart).getAllByRole('button').length, 3)
  fireEvent.keyDown(within(chart).getAllByRole('button')[0], { key: 'Enter' })
  assert.equal(selected, 8)
  fireEvent.click(screen.getByRole('button', { name: '排名下一页' }))
  assert.ok(screen.getByText('4–6 / 8 项'))
  fireEvent.click(screen.getByRole('button', { name: '排名下一页' }))
  assert.ok(screen.getByText('7–8 / 8 项'))
  assert.equal((screen.getByRole('button', { name: '排名下一页' }) as HTMLButtonElement).disabled, true)
  rerender(<TrafficRankingChart rows={[{ id: 1, name: 'zero', up_bytes: 0, down_bytes: 0 }]} dimension="user" direction="total" onSelect={() => {}} />)
  assert.ok(screen.getByRole('button', { name: /zero，上行 0 B/ }))
  assert.ok(!container.innerHTML.includes('NaN'))
  rerender(<TrafficRankingChart rows={[]} dimension="user" direction="total" onSelect={() => {}} />)
  assert.ok(screen.getByText('该范围暂无流量记录'))
})
