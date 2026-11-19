// @vitest-environment jsdom
import { test, afterEach } from 'vitest'
import assert from 'node:assert/strict'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Layout from '@/components/Layout'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'
import Users from '@/pages/Users'
import request from '@/lib/axios'

const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
const originalAdapter = request.defaults.adapter
afterEach(() => {
  cleanup()
  client.clear()
  request.defaults.adapter = originalAdapter
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
