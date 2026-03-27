import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, Filter, PencilLine, Plus, QrCode, Search } from 'lucide-react'
import { userApi, groupApi } from '@/lib/api'
import type { User } from '@/types'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn, FieldGroup } from '@/components/ui/Form'
import { QRModal } from '@/components/ui/QRModal'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/components/ui/ConfirmProvider'

const DEFAULT_PAGE_SIZE = 10
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

interface FormState {
  username: string
  real_name: string
  group_id: string
  expires_at: string
  remark: string
}

const emptyForm = (): FormState => ({
  username: '',
  real_name: '',
  group_id: '',
  expires_at: '',
  remark: '',
})

function Switch({ checked, onChange }: { checked: boolean; onChange: (next: boolean) => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      className={`relative h-6 w-11 rounded-full border transition ${
        checked ? 'border-emerald-500 bg-emerald-500' : 'border-[var(--border-strong)] bg-slate-200 dark:border-[var(--border)] dark:bg-white/10'
      }`}
    >
      <span
        className={`absolute top-0.5 h-4.5 w-4.5 rounded-full bg-white shadow transition ${
          checked ? 'left-[22px]' : 'left-0.5'
        }`}
      />
    </button>
  )
}

export default function Users() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [drawer, setDrawer] = useState<{ open: boolean; user?: User }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [qrUser, setQrUser] = useState<User | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive' | 'expired'>('all')
  const [groupFilter, setGroupFilter] = useState<string>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['users', page, pageSize],
    queryFn: () => userApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
  })

  const { data: groups } = useQuery({
    queryKey: ['groups-all'],
    queryFn: () => groupApi.list({ page: 1, page_size: 100 }).then((r) => r.data.data?.list ?? []),
  })

  const groupOptions = (groups ?? []).map((g) => ({ value: g.id, label: g.name }))
  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] })

  const save = useMutation({
    mutationFn: () => {
      const groupId = form.group_id ? Number(form.group_id) : undefined
      const expiresAt = form.expires_at || null
      if (drawer.user) {
        return userApi.update(drawer.user.id, {
          real_name: form.real_name,
          group_id: groupId ?? null,
          expires_at: expiresAt,
          remark: form.remark,
        })
      }
      return userApi.create({
        username: form.username,
        real_name: form.real_name,
        group_id: groupId,
        expires_at: expiresAt,
        remark: form.remark,
      })
    },
    onSuccess: () => {
      invalidate()
      closeDrawer()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const toggle = useMutation({ mutationFn: (id: number) => userApi.toggle(id), onSuccess: invalidate })
  const remove = useMutation({ mutationFn: (id: number) => userApi.remove(id), onSuccess: invalidate })
  const resetToken = useMutation({ mutationFn: (id: number) => userApi.resetToken(id), onSuccess: invalidate })

  const openCreate = () => {
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true })
  }

  const openEdit = (u: User) => {
    const next = {
      username: u.username,
      real_name: u.real_name,
      group_id: String(u.group_id ?? ''),
      expires_at: u.expires_at ? u.expires_at.slice(0, 16) : '',
      remark: u.remark,
    }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setDrawer({ open: true, user: u })
  }

  const closeDrawer = () => setDrawer({ open: false })
  const confirmCloseDrawer = async () => {
    if (!dirty) return true
    return confirm({
      title: '放弃未保存的用户修改？',
      description: '你在当前抽屉里还有未保存内容，关闭后这些修改会丢失。',
      confirmText: '放弃修改',
      cancelText: '继续编辑',
      tone: 'danger',
    })
  }

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const isExpired = (u: User) => (u.expires_at ? new Date(u.expires_at) < new Date() : false)

  const filteredUsers = useMemo(() => {
    return (data?.list ?? []).filter((u) => {
      const keyword = search.trim().toLowerCase()
      const matchesKeyword =
        keyword === '' ||
        u.username.toLowerCase().includes(keyword) ||
        u.real_name.toLowerCase().includes(keyword) ||
        (u.group_name ?? '').toLowerCase().includes(keyword)

      const matchesStatus =
        statusFilter === 'all' ||
        (statusFilter === 'active' && u.active) ||
        (statusFilter === 'inactive' && !u.active) ||
        (statusFilter === 'expired' && isExpired(u))

      const matchesGroup = groupFilter === 'all' || String(u.group_id ?? '') === groupFilter

      return matchesKeyword && matchesStatus && matchesGroup
    })
  }, [data?.list, search, statusFilter, groupFilter])

  const allVisibleSelected = filteredUsers.length > 0 && filteredUsers.every((u) => selectedIds.includes(u.id))
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm) && drawer.open

  const toggleSelect = (id: number) =>
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))

  const toggleSelectVisible = () =>
    setSelectedIds((prev) => {
      if (allVisibleSelected) return prev.filter((id) => !filteredUsers.some((u) => u.id === id))
      return Array.from(new Set([...prev, ...filteredUsers.map((u) => u.id)]))
    })

  const selectedUsers = filteredUsers.filter((u) => selectedIds.includes(u.id))

  const runBulk = async (runner: (u: User) => Promise<unknown>) => {
    await Promise.all(selectedUsers.map(runner))
    setSelectedIds([])
    invalidate()
  }

  return (
    <PageShell className="space-y-6">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="flex flex-1 flex-col gap-3 md:flex-row">
          <label className="relative min-w-0 flex-1 md:max-w-[380px]">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="搜索用户名、邮箱..."
              className="h-11 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] pl-10 pr-3 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
            />
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
              className="w-full bg-transparent text-sm outline-none"
            >
              <option value="all">全部状态</option>
              <option value="active">正常</option>
              <option value="inactive">禁用</option>
              <option value="expired">已过期</option>
            </select>
          </label>
          <label className="flex h-11 min-w-[130px] items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm">
            <Filter className="h-4 w-4 text-faint" />
            <select
              value={groupFilter}
              onChange={(e) => setGroupFilter(e.target.value)}
              className="w-full bg-transparent text-sm outline-none"
            >
              <option value="all">全部分组</option>
              {groupOptions.map((g) => (
                <option key={g.value} value={String(g.value)}>
                  {g.label}
                </option>
              ))}
            </select>
          </label>
        </div>

        <div className="flex items-center gap-2">
          <Btn onClick={openCreate}>
            <Plus className="h-4 w-4" />
            新增用户
          </Btn>
        </div>
      </div>

      {selectedUsers.length > 0 && (
        <SurfaceCard className="p-3">
          <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
            <div className="text-sm text-soft">
              已选择 <span className="font-semibold text-[var(--text)]">{selectedUsers.length}</span> 个用户
            </div>
            <div className="flex flex-wrap gap-2">
              <Btn variant="secondary" onClick={() => runBulk((u) => userApi.resetToken(u.id))}>批量重置订阅</Btn>
              <Btn variant="secondary" onClick={() => runBulk((u) => userApi.toggle(u.id))}>批量切换启用</Btn>
              <Btn
                variant="danger"
                onClick={async () => {
                  const ok = await confirm({
                    title: `删除已选中的 ${selectedUsers.length} 个用户？`,
                    description: '该操作不可撤销。',
                    confirmText: '批量删除',
                    cancelText: '取消',
                    tone: 'danger',
                  })
                  if (!ok) return
                  void runBulk((u) => userApi.remove(u.id))
                }}
              >
                批量删除
              </Btn>
            </div>
          </div>
        </SurfaceCard>
      )}

      <SurfaceCard className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full min-w-[1024px] text-left text-sm">
            <thead className="border-b border-[var(--border)] bg-[var(--panel-muted)]">
              <tr>
                <th className="w-10 px-4 py-3">
                  <input
                    type="checkbox"
                    checked={allVisibleSelected}
                    onChange={toggleSelectVisible}
                    className="h-4 w-4 rounded border-[var(--border-strong)]"
                    aria-label="选择当前页可见用户"
                  />
                </th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">用户名</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">分组</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">到期时间</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">状态</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">启用</th>
                <th className="px-4 py-3 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-faint">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr>
                  <td colSpan={7} className="px-4 py-10 text-center text-soft">加载中…</td>
                </tr>
              ) : filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan={7} className="px-4 py-10 text-center text-soft">暂无用户数据</td>
                </tr>
              ) : (
                filteredUsers.map((u) => (
                  <tr key={u.id} className="transition hover:bg-white/5">
                    <td className="px-4 py-3.5">
                      <input
                        type="checkbox"
                        checked={selectedIds.includes(u.id)}
                        onChange={() => toggleSelect(u.id)}
                        className="h-4 w-4 rounded border-[var(--border-strong)]"
                        aria-label={`选择用户 ${u.username}`}
                      />
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="font-semibold">{u.username}</div>
                      <div className="text-xs text-soft">{u.real_name || '—'}</div>
                    </td>
                    <td className="px-4 py-3.5">
                      {u.group_name ? <Badge label={u.group_name} variant="blue" /> : <span className="text-xs text-soft">未分组</span>}
                    </td>
                    <td className="px-4 py-3.5">
                      {u.expires_at ? (
                        <span className={isExpired(u) ? 'text-rose-400' : 'text-soft'}>
                          {new Date(u.expires_at).toLocaleDateString('zh-CN')}
                        </span>
                      ) : (
                        <span className="text-soft">永久</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5">
                      <Badge label={u.active ? '正常' : '已禁用'} variant={u.active ? 'green' : 'red'} />
                    </td>
                    <td className="px-4 py-3.5">
                      <Switch checked={u.active} onChange={() => toggle.mutate(u.id)} />
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => setQrUser(u)}
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-[var(--accent)] transition hover:bg-[var(--panel-muted)]"
                        >
                          <QrCode className="h-3.5 w-3.5" />
                          订阅
                        </button>
                        <button
                          onClick={() => openEdit(u)}
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                        >
                          <PencilLine className="h-3.5 w-3.5" />
                          编辑
                        </button>
                        <ActionMenu
                          items={[
                            {
                              label: '重置订阅链接',
                              onSelect: async () => {
                                const ok = await confirm({
                                  title: `重置 ${u.username} 的订阅链接？`,
                                  description: '旧订阅地址会立即失效。',
                                  confirmText: '确认重置',
                                  cancelText: '取消',
                                })
                                if (ok) resetToken.mutate(u.id)
                              },
                            },
                            {
                              label: u.active ? '禁用用户' : '启用用户',
                              onSelect: () => toggle.mutate(u.id),
                            },
                            {
                              label: '删除用户',
                              danger: true,
                              onSelect: async () => {
                                const ok = await confirm({
                                  title: `删除用户 ${u.username}？`,
                                  description: '该操作不可撤销。',
                                  confirmText: '删除用户',
                                  cancelText: '取消',
                                  tone: 'danger',
                                })
                                if (ok) remove.mutate(u.id)
                              },
                            },
                          ]}
                        />
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </SurfaceCard>

      <div className="flex items-center justify-between text-sm text-soft">
        <span>共 {data?.total ?? 0} 条</span>
        <div className="flex items-center gap-2">
          <label className="inline-flex items-center gap-2 text-sm text-soft">
            分页
            <select
              value={pageSize}
              onChange={(e) => {
                setPageSize(Number(e.target.value))
                setPage(1)
              }}
              className="h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"
            >
              {PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size} / 页
                </option>
              ))}
            </select>
          </label>
          <Btn variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>上一页</Btn>
          <span>{page}</span>
          <Btn
            variant="secondary"
            disabled={page >= Math.max(1, Math.ceil((data?.total ?? 0) / pageSize))}
            onClick={() => setPage((p) => p + 1)}
          >
            下一页
          </Btn>
        </div>
      </div>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.user ? `编辑用户 · ${drawer.user.username}` : '新增用户'}
        description={drawer.user ? '调整分组、有效期和订阅信息。' : '快速创建新的订阅用户。'}
        dirty={dirty}
        saving={save.isPending}
        onBeforeClose={confirmCloseDrawer}
        footer={
          <>
            <Btn variant="secondary" onClick={closeDrawer}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="space-y-4">
          <FieldGroup title="基础信息" description="用户标识、展示名称和所属分组。">
            {!drawer.user && <Field label="用户名 *" value={form.username} onChange={f('username')} placeholder="如：alice" />}
            <Field label="真实姓名" value={form.real_name} onChange={f('real_name')} />
            <SelectField
              label="所属分组"
              value={form.group_id}
              onChange={(v) => setForm((p) => ({ ...p, group_id: v }))}
              options={groupOptions}
              placeholder="不分组"
            />
          </FieldGroup>

          <FieldGroup title="订阅策略" description="有效期为空时视为永久有效。">
            <Field label="过期时间" type="datetime-local" value={form.expires_at} onChange={f('expires_at')} />
            <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="例如：测试账号 / 临时用户" />
          </FieldGroup>

          {drawer.user && (
            <FieldGroup title="快捷动作" description="把高频动作前置，不需要先关闭编辑抽屉。">
              <div className="flex flex-wrap gap-2">
                <Btn variant="secondary" onClick={() => setQrUser(drawer.user!)}><ExternalLink className="h-4 w-4" />查看订阅二维码</Btn>
                <Btn
                  variant="secondary"
                  onClick={async () => {
                    const ok = await confirm({
                      title: `重置 ${drawer.user!.username} 的订阅链接？`,
                      description: '旧订阅地址会立即失效。',
                      confirmText: '确认重置',
                      cancelText: '取消',
                    })
                    if (ok) resetToken.mutate(drawer.user!.id)
                  }}
                >
                  重置订阅链接
                </Btn>
              </div>
            </FieldGroup>
          )}

          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Drawer>

      {qrUser && <QRModal open={!!qrUser} onClose={() => setQrUser(null)} url={qrUser.subscribe_url} title={`${qrUser.username} 的订阅`} />}
    </PageShell>
  )
}
