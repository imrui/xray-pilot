// @vitest-environment jsdom
import { afterEach, expect, test } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import request from '@/lib/axios'
import { ConfirmContext } from '@/lib/confirm-context'
import AdminsSection from '@/pages/settings/AdminsSection'
import { useAuthStore } from '@/store/auth'

const adapter = request.defaults.adapter
const auth = useAuthStore.getState()
const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
afterEach(() => {
  cleanup()
  client.clear()
  request.defaults.adapter = adapter
  useAuthStore.setState(auth)
})

test('管理员翻页后修改和删除对应账号，删除末页最后一名后回到上一页', async () => {
  let admins = Array.from({ length: 11 }, (_, i) => ({
    id: 101 + i,
    username: `admin-${i + 1}`,
    role: i === 0 ? 'super_admin' : 'admin',
    active: true,
  }))
  const mutations: string[] = []
  request.defaults.adapter = async config => {
    if (config.method === 'put') {
      mutations.push(`put ${config.url}`)
      const patch = JSON.parse(config.data)
      admins = admins.map(admin => config.url === `/admins/${admin.id}` ? { ...admin, ...patch } : admin)
    }
    if (config.method === 'delete') {
      mutations.push(`delete ${config.url}`)
      admins = admins.filter(admin => config.url !== `/admins/${admin.id}`)
    }
    return { config, headers: {}, status: 200, statusText: 'OK', data: { code: 0, data: admins } }
  }
  useAuthStore.setState({ username: 'admin-1', role: 'super_admin' })
  render(<QueryClientProvider client={client}>
    <ConfirmContext.Provider value={async options => options.title === '删除管理员「admin-11」？'}>
      <AdminsSection />
    </ConfirmContext.Provider>
  </QueryClientProvider>)

  await screen.findByText('共 11 条')
  expect(screen.getAllByRole('listitem')).toHaveLength(10)
  expect((screen.getByTitle('不能删除自己') as HTMLButtonElement).disabled).toBe(true)
  expect(screen.queryByText('admin-11')).toBeNull()
  fireEvent.change(screen.getByLabelText('每页条数'), { target: { value: '20' } })
  expect(screen.getAllByRole('listitem')).toHaveLength(11)
  fireEvent.change(screen.getByLabelText('每页条数'), { target: { value: '10' } })
  fireEvent.click(screen.getByRole('button', { name: '下一页' }))
  const row = screen.getByRole('listitem')
  expect(within(row).getByText('admin-11')).toBeTruthy()
  fireEvent.change(within(row).getByRole('combobox'), { target: { value: 'super_admin' } })
  await waitFor(() => expect(admins.at(-1)?.role).toBe('super_admin'))
  fireEvent.click(within(row).getByTitle('删除'))
  await screen.findByText('共 10 条')
  await screen.findByText('admin-1')
  expect(screen.getByLabelText('第 1 页')).toBeTruthy()
  expect(screen.getAllByRole('listitem')).toHaveLength(10)
  expect(mutations).toEqual(['put /admins/111', 'delete /admins/111'])
})
