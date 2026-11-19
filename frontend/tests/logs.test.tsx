import './dom-setup.mjs'
import React from 'react'
import assert from 'node:assert/strict'
import { test } from 'node:test'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import request from '../src/lib/axios'
import Logs from '../src/pages/Logs'

test('操作日志跨页搜索保留来源筛选，更改关键字后从首页展示匹配结果', async (t) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const adapter = request.defaults.adapter
  t.after(() => {
    cleanup()
    queryClient.clear()
    request.defaults.adapter = adapter
  })
  const calls: { actor?: string; page: number; keyword?: string }[] = []
  const entries = Array.from({ length: 24 }, (_, i) => ({
    id: i + 1,
    actor: i === 23 ? 'admin:alice' : 'system:feishu-webhook',
    message: i === 23 ? '旧事件：管理员' : i === 0 ? '旧事件：回调' : `日志 ${i + 1}`,
    action: 'refresh',
    success: true,
    created_at: '2026-11-17T05:00:00Z',
  })).reverse()
  request.defaults.adapter = async config => {
    const { actor, page, page_size, keyword } = config.params
    calls.push({ actor, page, keyword })
    const matches = entries.filter(item =>
      (!actor || item.actor.startsWith(actor)) && (!keyword || item.message.includes(keyword)),
    )
    const list = matches.slice((page - 1) * page_size, page * page_size)
    return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, data: { list, total: matches.length } } }
  }
  render(
    <QueryClientProvider client={queryClient}><Logs /></QueryClientProvider>,
  )
  await screen.findAllByText('旧事件：管理员')
  fireEvent.click(screen.getByRole('button', { name: '飞书回调' }))
  await screen.findAllByText('日志 23')
  assert.equal(screen.queryAllByText('旧事件：回调').length, 0)
  assert.equal(screen.queryAllByText('旧事件：管理员').length, 0)
  fireEvent.click(screen.getByRole('button', { name: '下一页' }))
  await screen.findAllByText('旧事件：回调')
  assert.equal(calls.at(-1)?.page, 2)
  fireEvent.change(screen.getByPlaceholderText('搜索操作、来源、目标或消息'), { target: { value: ' 旧事件 ' } })
  await waitFor(() => {
    assert.deepEqual(calls.at(-1), { actor: 'system:feishu-webhook', page: 1, keyword: '旧事件' })
  })
  await screen.findAllByText('旧事件：回调')
  assert.equal(screen.queryAllByText('旧事件：管理员').length, 0)
  fireEvent.click(screen.getByRole('button', { name: '清空搜索' }))
  await screen.findAllByText('日志 23')
  assert.equal(screen.queryAllByText('旧事件：回调').length, 0)
})

test('日志长消息可展开，飞书筛选下清理仍需确认全部来源和保留天数', async (t) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { gcTime: 0 } } })
  const adapter = request.defaults.adapter
  t.after(() => {
    cleanup()
    queryClient.clear()
    request.defaults.adapter = adapter
  })
  const message = '推送超时\n下游回调未响应，请检查连接。'
  const cleanupDays: number[] = []
  request.defaults.adapter = async config => {
    let data: unknown
    if (config.url === '/logs/cleanup') {
      cleanupDays.push(JSON.parse(config.data).days)
      data = { deleted: 3 }
    } else {
      data = { total: 1, list: [{ id: 1, action: 'feishu_push', target: 'test', actor: 'system:feishu-webhook',
        success: false, message, duration_ms: 3000, created_at: '2026-11-17T05:00:00Z' }] }
    }
    return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, data } }
  }
  render(<QueryClientProvider client={queryClient}><Logs /></QueryClientProvider>)
  fireEvent.click(screen.getByRole('button', { name: '飞书回调' }))
  const table = within(await screen.findByRole('table'))
  assert.equal(table.getAllByText('超时').length, 1)
  const summary = table.getByText('推送超时…').closest('summary')!
  const details = summary.parentElement as HTMLDetailsElement
  assert.equal(details.open, false)
  fireEvent.click(summary)
  assert.equal(details.open, true)
  assert.equal(details.querySelector('p')?.textContent, message)

  fireEvent.click(screen.getByRole('button', { name: '清理历史日志' }))
  let dialog = within(screen.getByRole('dialog', { name: '清理历史日志' }))
  assert.match(dialog.getByText(/永久删除/).textContent ?? '', /所有来源/)
  fireEvent.click(dialog.getByRole('button', { name: '取消' }))
  assert.deepEqual(cleanupDays, [])
  fireEvent.click(screen.getByRole('button', { name: '清理历史日志' }))
  dialog = within(screen.getByRole('dialog', { name: '清理历史日志' }))
  fireEvent.change(dialog.getByLabelText('保留最近'), { target: { value: 'custom' } })
  fireEvent.change(dialog.getByLabelText('保留天数'), { target: { value: '1.5' } })
  assert.equal((dialog.getByRole('button', { name: '确认清理' }) as HTMLButtonElement).disabled, true)
  fireEvent.change(dialog.getByLabelText('保留天数'), { target: { value: '14' } })
  assert.match(dialog.getByText(/永久删除/).textContent ?? '', /14 天前/)
  fireEvent.click(dialog.getByRole('button', { name: '确认清理' }))
  await waitFor(() => assert.equal(screen.queryAllByRole('dialog').length, 0))
  assert.deepEqual(cleanupDays, [14])
})
