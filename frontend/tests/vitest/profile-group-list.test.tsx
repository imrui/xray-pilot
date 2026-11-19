// @vitest-environment jsdom
import { afterEach, expect, test } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ConfirmProvider } from '@/components/ui/ConfirmProvider'
import request from '@/lib/axios'
import Groups from '@/pages/Groups'
import Profiles from '@/pages/Profiles'
import { profileApi } from '@/lib/api'

const originalAdapter = request.defaults.adapter
const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
afterEach(() => {
  cleanup()
  client.clear()
  request.defaults.adapter = originalAdapter
})

test.each([
  ['SOCKS5', ['socks']],
  ['vless + ws + tls', ['vless-ws-tls']],
  ['Trojan + TLS', ['trojan']],
  ['+ TLS', ['vless-ws-tls', 'trojan']],
])('协议关键词 %s 同时搜索显示标签和自定义名称', async (keyword, protocols) => {
  request.defaults.adapter = async (config) => {
    const params = new URLSearchParams(request.getUri(config).split('?')[1])
    expect(params.get('keyword')).toBe(keyword)
    expect(params.get('protocol')).toBe('http')
    expect(params.getAll('keyword_protocol')).toEqual(protocols)
    return { config, headers: {}, status: 200, statusText: 'OK', data: { code: 0, data: { list: [], total: 0 } } }
  }
  await profileApi.list({ keyword, protocol: 'http' })
})

test.each(['profiles', 'groups'])('%s 的筛选和清空从第一页请求匹配记录', async (resource) => {
  const requests: Record<string, unknown>[] = []
  request.defaults.adapter = async (config) => {
    const params = config.params ?? {}
    const targeted = config.url === '/' + resource
    if (targeted) requests.push(params)
    return {
      config, headers: {}, status: 200, statusText: 'OK',
      data: { code: 0, data: { total: targeted && !params.keyword ? 23 : 0, list: [] } },
    }
  }
  const profiles = resource === 'profiles'
  render(<QueryClientProvider client={client}>
    <ConfirmProvider>{profiles ? <Profiles /> : <Groups />}</ConfirmProvider>
  </QueryClientProvider>)
  await screen.findByText('共 23 条')
  fireEvent.click(screen.getByRole('button', { name: '下一页' }))
  await waitFor(() => expect(requests.at(-1)?.page).toBe(2))
  if (profiles) {
    fireEvent.change(screen.getByRole('combobox', { name: '筛选协议类型' }), { target: { value: 'socks' } })
  } else {
    fireEvent.click(screen.getByRole('button', { name: '禁用' }))
  }
  const filter = profiles ? { protocol: 'socks' } : { active: false }
  await waitFor(() => expect(requests.at(-1)).toMatchObject({ page: 1, ...filter }))
  const input = screen.getByRole('textbox', { name: profiles ? '搜索协议名或协议类型' : '搜索分组名或描述' })
  fireEvent.change(input, { target: { value: '实验室' } })
  await screen.findByText('共 0 条')
  expect(requests.at(-1)).toMatchObject({ keyword: '实验室', page: 1, ...filter })
  fireEvent.click(screen.getByRole('button', { name: '清空搜索' }))
  await screen.findByText('共 23 条')
  expect(requests.at(-1)).toMatchObject({ keyword: undefined, page: 1, ...filter })
})
