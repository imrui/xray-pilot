import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { QrCode, RefreshCw } from 'lucide-react'
import { userApi, groupApi } from '@/lib/api'
import type { User } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn } from '@/components/ui/Form'
import { QRModal } from '@/components/ui/QRModal'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

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
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; user?: User }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [qrUser, setQrUser] = useState<User | null>(null)

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
      if (modal.user) {
        return userApi.update(modal.user.id, {
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
      closeModal()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const toggle = useMutation({ mutationFn: (id: number) => userApi.toggle(id), onSuccess: invalidate })
  const remove = useMutation({ mutationFn: (id: number) => userApi.remove(id), onSuccess: invalidate })
  const resetToken = useMutation({ mutationFn: (id: number) => userApi.resetToken(id), onSuccess: invalidate })

  const openCreate = () => {
    setForm(emptyForm())
    setErr('')
    setModal({ open: true })
  }

  const openEdit = (u: User) => {
    setForm({
      username: u.username,
      real_name: u.real_name,
      group_id: String(u.group_id ?? ''),
      expires_at: u.expires_at ? u.expires_at.slice(0, 16) : '',
      remark: u.remark,
    })
    setErr('')
    setModal({ open: true, user: u })
  }

  const closeModal = () => setModal({ open: false })

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((prev) => ({ ...prev, [k]: e.target.value }))

  const isExpired = (u: User) => (u.expires_at ? new Date(u.expires_at) < new Date() : false)

  const columns = [
    { key: 'id', label: 'ID' },
    {
      key: 'username',
      label: '用户名',
      render: (u: User) => (
        <div>
          <span className="font-semibold">{u.username}</span>
          {u.real_name && <span className="ml-2 text-xs text-soft">{u.real_name}</span>}
        </div>
      ),
    },
    {
      key: 'group_name',
      label: '分组',
      render: (u: User) => (u.group_name ? <Badge label={u.group_name} variant="blue" /> : <span className="text-soft">未分组</span>),
    },
    {
      key: 'expires_at',
      label: '有效期',
      render: (u: User) => {
        if (!u.expires_at) return <span className="text-xs text-soft">永久</span>
        const expired = isExpired(u)
        return <span className={`text-xs ${expired ? 'text-rose-500' : 'text-soft'}`}>{expired ? '已过期 · ' : ''}{new Date(u.expires_at).toLocaleDateString()}</span>
      },
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
        <div className="flex flex-wrap gap-2">
          <button onClick={() => openEdit(u)} className="text-xs text-soft transition hover:text-[var(--text)]">编辑</button>
          <button onClick={() => setQrUser(u)} className="flex items-center gap-1 text-xs text-[var(--accent)] transition hover:brightness-110">
            <QrCode className="h-3 w-3" />
            订阅
          </button>
          <button
            onClick={() => {
              if (confirm(`重置 ${u.username} 的订阅链接？旧链接将失效`)) resetToken.mutate(u.id)
            }}
            className="flex items-center gap-1 text-xs text-amber-500 transition hover:brightness-110"
          >
            <RefreshCw className="h-3 w-3" />
            重置链接
          </button>
          <button onClick={() => toggle.mutate(u.id)} className="text-xs text-soft transition hover:text-[var(--text)]">
            {u.active ? '禁用' : '启用'}
          </button>
          <button
            onClick={() => {
              if (confirm(`确认删除用户 ${u.username}？`)) remove.mutate(u.id)
            }}
            className="text-xs text-rose-500 transition hover:brightness-110"
          >
            删除
          </button>
        </div>
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="用户管理"
        description="集中管理订阅用户、所属分组和有效期，重点突出分发状态和运维动作。"
        actions={<Btn onClick={openCreate}>新增用户</Btn>}
        stats={[
          { label: '总用户数', value: data?.total ?? 0 },
          { label: '分页尺寸', value: PAGE_SIZE },
          { label: '可选分组', value: groups?.length ?? 0 },
        ]}
      />

      <SurfaceCard className="p-4">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>

      <Modal
        open={modal.open}
        onClose={closeModal}
        title={modal.user ? '编辑用户' : '新增用户'}
        footer={
          <>
            <Btn variant="secondary" onClick={closeModal}>取消</Btn>
            <Btn loading={save.isPending} onClick={() => save.mutate()}>保存</Btn>
          </>
        }
      >
        <div className="space-y-4">
          {!modal.user && <Field label="用户名 *" value={form.username} onChange={f('username')} placeholder="如：alice" />}
          <Field label="真实姓名" value={form.real_name} onChange={f('real_name')} />
          <SelectField
            label="所属分组"
            value={form.group_id}
            onChange={(v) => setForm((p) => ({ ...p, group_id: v }))}
            options={groupOptions}
            placeholder="不分组"
          />
          <Field label="过期时间（留空 = 永久有效）" type="datetime-local" value={form.expires_at} onChange={f('expires_at')} />
          <Field label="备注" value={form.remark} onChange={f('remark')} />
          {err && <p className="text-sm text-rose-500">{err}</p>}
        </div>
      </Modal>

      {qrUser && <QRModal open={!!qrUser} onClose={() => setQrUser(null)} url={qrUser.subscribe_url} title={`${qrUser.username} 的订阅`} />}
    </PageShell>
  )
}
