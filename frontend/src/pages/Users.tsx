import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ExternalLink, PencilLine, QrCode } from 'lucide-react'
import { userApi, groupApi } from '@/lib/api'
import type { User } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn, FieldGroup } from '@/components/ui/Form'
import { QRModal } from '@/components/ui/QRModal'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { BulkBar, FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { useConfirm } from '@/components/ui/ConfirmProvider'

const PAGE_SIZE = 20

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

export default function Users() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [drawer, setDrawer] = useState<{ open: boolean; user?: User }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [qrUser, setQrUser] = useState<User | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'inactive' | 'expired'>('all')
  const [selectedIds, setSelectedIds] = useState<number[]>([])

  const { data, isLoading } = useQuery({
    queryKey: ['users', page],
    queryFn: () => userApi.list({ page, page_size: PAGE_SIZE }).then((r) => r.data.data!),
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
  const filteredUsers = (data?.list ?? []).filter((u) => {
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

    return matchesKeyword && matchesStatus
  })
  const allVisibleSelected = filteredUsers.length > 0 && filteredUsers.every((u) => selectedIds.includes(u.id))
  const dirty = JSON.stringify(form) !== JSON.stringify(initialForm) && drawer.open

  const toggleSelect = (id: number) =>
    setSelectedIds((prev) => (prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]))

  const toggleSelectVisible = () =>
    setSelectedIds((prev) => {
      if (allVisibleSelected) {
        return prev.filter((id) => !filteredUsers.some((u) => u.id === id))
      }
      return Array.from(new Set([...prev, ...filteredUsers.map((u) => u.id)]))
    })

  const selectedUsers = filteredUsers.filter((u) => selectedIds.includes(u.id))

  const runBulk = async (runner: (u: User) => Promise<unknown>) => {
    await Promise.all(selectedUsers.map(runner))
    setSelectedIds([])
    invalidate()
  }

  const columns = [
    {
      key: 'select',
      label: (
        <input
          type="checkbox"
          checked={allVisibleSelected}
          onChange={toggleSelectVisible}
          aria-label="选择当前页可见用户"
          className="h-4 w-4 rounded border-[var(--border-strong)]"
        />
      ) as unknown as string,
      render: (u: User) => (
        <input
          type="checkbox"
          checked={selectedIds.includes(u.id)}
          onChange={() => toggleSelect(u.id)}
          aria-label={`选择用户 ${u.username}`}
          className="h-4 w-4 rounded border-[var(--border-strong)]"
        />
      ),
    },
    {
      key: 'username',
      label: '用户',
      render: (u: User) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-semibold">{u.username}</span>
            {u.real_name && <span className="text-xs text-soft">{u.real_name}</span>}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {u.group_name ? <Badge label={u.group_name} variant="blue" /> : <span className="text-xs text-soft">未分组</span>}
            {u.expires_at ? (
              <span className={`text-xs ${isExpired(u) ? 'text-rose-500' : 'text-soft'}`}>{isExpired(u) ? '已过期' : new Date(u.expires_at).toLocaleDateString()}</span>
            ) : (
              <span className="text-xs text-soft">永久有效</span>
            )}
          </div>
        </div>
      ),
    },
    {
      key: 'active',
      label: '状态',
      render: (u: User) => <Badge label={u.active ? '启用' : '禁用'} variant={u.active ? 'green' : 'gray'} />,
    },
    {
      key: 'actions',
      label: '操作',
      render: (u: User) => (
        <div className="flex items-center justify-end gap-2">
          <button
            onClick={() => setQrUser(u)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-[var(--accent)] transition hover:bg-[var(--panel)]"
          >
            <QrCode className="h-3.5 w-3.5" />
            订阅
          </button>
          <button
            onClick={() => openEdit(u)}
            className="inline-flex items-center gap-1 rounded-full border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-1.5 text-xs font-semibold text-soft transition hover:bg-[var(--panel)] hover:text-[var(--text)]"
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
                    description: '旧订阅地址会立即失效，客户端需要使用新的链接。',
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
                    description: '此操作不可撤销，用户订阅信息会被永久移除。',
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
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="用户管理"
        description="列表只保留常用动作，编辑和配置进入右侧抽屉，减少表格区的视觉噪音。"
        actions={<Btn onClick={openCreate}>新增用户</Btn>}
        stats={[
          { label: '总用户数', value: data?.total ?? 0 },
          { label: '分页尺寸', value: PAGE_SIZE },
          { label: '可选分组', value: groups?.length ?? 0 },
        ]}
      />

      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索用户名、真实姓名或分组"
        onSearchChange={setSearch}
        filters={
          <>
            <FilterChip active={statusFilter === 'all'} onClick={() => setStatusFilter('all')}>全部</FilterChip>
            <FilterChip active={statusFilter === 'active'} onClick={() => setStatusFilter('active')}>启用</FilterChip>
            <FilterChip active={statusFilter === 'inactive'} onClick={() => setStatusFilter('inactive')}>禁用</FilterChip>
            <FilterChip active={statusFilter === 'expired'} onClick={() => setStatusFilter('expired')}>已过期</FilterChip>
          </>
        }
        meta={`当前页匹配 ${filteredUsers.length} / ${(data?.list ?? []).length} 条`}
        bulkBar={
          selectedUsers.length > 0 ? (
            <BulkBar>
              <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="text-sm text-soft">已选择 <span className="font-semibold text-[var(--text)]">{selectedUsers.length}</span> 个用户</div>
                <div className="flex flex-wrap gap-2">
                  <Btn variant="secondary" onClick={() => runBulk((u) => userApi.resetToken(u.id))}>批量重置订阅链接</Btn>
                  <Btn variant="secondary" onClick={() => runBulk((u) => userApi.toggle(u.id))}>批量切换启用状态</Btn>
                  <Btn
                    variant="danger"
                    onClick={async () => {
                      const ok = await confirm({
                        title: `删除已选中的 ${selectedUsers.length} 个用户？`,
                        description: '批量删除不可撤销，建议先确认筛选和勾选范围。',
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
            </BulkBar>
          ) : null
        }
      />

      <SurfaceCard className="p-4">
        <Table columns={columns} data={filteredUsers} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Drawer
        open={drawer.open}
        onClose={closeDrawer}
        title={drawer.user ? `编辑用户 · ${drawer.user.username}` : '新增用户'}
        description={drawer.user ? '在不离开列表上下文的情况下调整分组、有效期和订阅信息。' : '快速创建新的订阅用户，并直接补充分组和有效期。'}
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
            <FieldGroup title="快捷动作" description="把高频配置动作前置，不需要先关闭编辑抽屉。">
              <div className="flex flex-wrap gap-2">
                <Btn variant="secondary" onClick={() => setQrUser(drawer.user!)}><ExternalLink className="h-4 w-4" />查看订阅二维码</Btn>
                <Btn
                  variant="secondary"
                  onClick={async () => {
                    const ok = await confirm({
                      title: `重置 ${drawer.user!.username} 的订阅链接？`,
                      description: '旧订阅地址会立即失效，客户端需要重新导入新链接。',
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
