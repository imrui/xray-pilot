// @vitest-environment jsdom
import { test, afterEach, vi } from 'vitest'
import assert from 'node:assert/strict'
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Layout from '@/components/Layout'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'
import Users from '@/pages/Users'
import request from '@/lib/axios'
import * as clipboard from '@/lib/clipboard'

const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
const originalAdapter = request.defaults.adapter
afterEach(() => {
  cleanup()
  client.clear()
  request.defaults.adapter = originalAdapter
  vi.restoreAllMocks()
})

test('两类订阅独立分列，按所选格式复制地址并保存二维码', async () => {
  const primary = 'https://panel.example/sub/office'
  const relay = 'http://192.168.0.2:2096/sub/u002abc'
  const copy = vi.spyOn(clipboard, 'copyText').mockResolvedValue(true)
  // jsdom 无图形上下文，在此核对下载的文件与所选格式。
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
  const png = 'data:image/png;base64,iVBORw0KGgo='
  const exportPNG = vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue(png)
  const downloads: string[] = []
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
    assert.equal(this.href, png)
    downloads.push(this.download)
  })
  let enabled = true
  request.defaults.adapter = async config => {
    const user = { id: 1, username: '瑞/office', subscribe_url: primary, group_ids: [], group_names: [] }
    const data = config.url === '/users' ? { total: 2, relay_enabled: enabled, list: [
      { ...user, relay_subscription_url: enabled ? relay : undefined }, { ...user, id: 2, username: 'guest' },
    ] } : { list: [], total: 0 }
    return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, data } }
  }
  render(<QueryClientProvider client={client}><MemoryRouter><ConfirmProvider><Users /></ConfirmProvider></MemoryRouter></QueryClientProvider>)
  const row = within(await screen.findByRole('row', { name: /office/ }))
  const headings = screen.getAllByRole('columnheader').map(th => th.textContent)
  assert.equal(headings[headings.indexOf('订阅') + 1], '内网中转订阅')
  assert.ok(screen.getByText('未配置'))
  for (const [label, url] of [['订阅', primary], ['内网中转订阅', relay]]) {
    const group = within(row.getByRole('group', { name: `瑞/office 的${label}链接` }))
    fireEvent.click(group.getByRole('button', { name: `复制 瑞/office 的${label}链接` }))
    await group.findByText('已复制')
    assert.equal(copy.mock.lastCall?.[0], url)
  }
  for (const { label, base, prefix, formats } of [
    { label: '订阅', base: primary, prefix: 'subscription', formats: [['订阅', ''], ['V2Ray', 'v2ray'], ['Clash', 'clash'], ['sing-box', 'singbox']] },
    { label: '内网中转订阅', base: relay, prefix: 'intranet-relay', formats: [['订阅', ''], ['JSON', 'json'], ['Clash', 'clash']] },
  ]) {
    const open = row.getByRole('button', { name: `查看 瑞/office 的${label}二维码` })
    fireEvent.click(open)
    const modal = within(await screen.findByRole('dialog', { name: `瑞/office 的${label}二维码` }))
    assert.equal(modal.getAllByRole('radio').length, formats.length)
    assert.ok((modal.getByRole('radio', { name: '订阅' }) as HTMLInputElement).checked)
    for (const [name, format] of formats) {
      fireEvent.click(modal.getByRole('radio', { name }))
      const url = format ? `${base}?format=${format}` : base
      assert.ok(modal.getByText(url))
      fireEvent.click(modal.getByRole('button', { name: '复制地址' }))
      await modal.findByText('已复制')
      assert.equal(copy.mock.lastCall?.[0], url)
      fireEvent.click(modal.getByRole('button', { name: '保存二维码' }))
      assert.equal(exportPNG.mock.lastCall?.[0], 'image/png')
      assert.equal(downloads.at(-1), `xray-pilot-瑞_office-${prefix}-${format || 'default'}.png`)
    }
    fireEvent.click(modal.getByRole('button', { name: '关闭对话框' }))
    fireEvent.click(open)
    assert.ok((screen.getByRole('radio', { name: '订阅' }) as HTMLInputElement).checked)
    fireEvent.click(screen.getByRole('button', { name: '关闭对话框' }))
  }
  enabled = false
  await act(() => client.invalidateQueries({ queryKey: ['users'] }))
  await waitFor(() => assert.ok(!screen.queryByRole('columnheader', { name: '内网中转订阅' })))
  assert.ok(row.getByRole('link', { name: '打开 瑞/office 的订阅页' }))
})

test.each(['新增用户', '编辑'])('%s时随中转 ID 预览当前配置的完整地址', async action => {
  const prefix = 'https://relay.example:8443/team/sub/'
  let enabled = true
  const user = { id: 1, username: 'office', real_name: '', remark: '', group_ids: [], relay_subscription_id: 'u002abc' }
  request.defaults.adapter = async config => {
    const data = config.url === '/users'
      ? { total: 1, list: [user], relay_enabled: enabled, relay_prefix: enabled ? prefix : undefined }
      : { total: 0, list: [] }
    return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, data } }
  }
  render(<QueryClientProvider client={client}><MemoryRouter><ConfirmProvider><Users /></ConfirmProvider></MemoryRouter></QueryClientProvider>)
  await screen.findByText('office')
  fireEvent.click(screen.getByRole('button', { name: action }))
  const drawer = within(await screen.findByRole('dialog'))
  const input = drawer.getByRole('textbox', { name: '内网中转订阅 ID' })
  const preview = () => drawer.queryByText(text => text.startsWith(prefix))
  assert.equal(preview()?.textContent ?? null, action === '编辑' ? prefix + 'u002abc' : null)
  for (const id of ['', '.', '..', 'a/b', 'a?b']) {
    fireEvent.change(input, { target: { value: id } })
    assert.ok(!preview())
  }
  fireEvent.change(input, { target: { value: ' Ab_01.test-x ' } })
  assert.equal(preview()?.textContent, prefix + 'Ab_01.test-x')
  assert.ok(!drawer.queryByRole('link', { name: prefix + 'Ab_01.test-x' }))
  enabled = false
  await act(() => client.invalidateQueries({ queryKey: ['users'] }))
  await waitFor(() => assert.ok(!preview()))
})

test('列表和顶栏都能找到原本在第二页的用户，清空后恢复第一页', async () => {
  const users = Array.from({ length: 12 }, (_, i) => ({
    id: i + 1, username: `staff-${i + 1}`, active: true, group_ids: [], group_names: [],
  }))
  request.defaults.adapter = async config => {
    let data: unknown = {}
    if (config.url === '/users') {
      const { keyword = '', page, page_size } = config.params
      const matches = users.filter(user => user.username.includes(keyword))
      data = { total: matches.length, list: matches.slice((page - 1) * page_size, page * page_size) }
    } else if (config.url === '/groups') {
      data = { total: 0, list: [] }
    }
    return { config, status: 200, statusText: 'OK', headers: {}, data: { code: 0, data } }
  }
  render(<QueryClientProvider client={client}><MemoryRouter><ConfirmProvider><Routes>
    <Route element={<Layout />}>
      <Route index element={<Users />} />
      <Route path="users" element={<Users />} />
    </Route>
  </Routes></ConfirmProvider></MemoryRouter></QueryClientProvider>)
  await screen.findByText('共 12 条')
  assert.equal(screen.queryByText('staff-12'), null)
  fireEvent.change(screen.getByRole('textbox', { name: /^搜索用户名/ }), { target: { value: 'staff-12' } })
  await screen.findByText('staff-12')
  assert.ok(screen.getByText('共 1 条'))
  fireEvent.click(screen.getByRole('button', { name: '清空搜索' }))
  await screen.findByText('共 12 条')
  assert.equal(screen.queryByText('staff-12'), null)
  fireEvent.change(screen.getByPlaceholderText('搜索节点、用户...'), { target: { value: 'staff-11' } })
  fireEvent.click(screen.getByRole('button', { name: '在用户中搜索' }))
  await screen.findByText('staff-11')
  assert.equal((screen.getByRole('textbox', { name: /^搜索用户名/ }) as HTMLInputElement).value, 'staff-11')
})
