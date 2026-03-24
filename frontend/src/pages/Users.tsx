import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Copy, Check } from 'lucide-react'
import { userApi, groupApi } from '@/lib/api'
import type { User } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Modal } from '@/components/ui/Modal'
import { Badge } from '@/components/ui/Badge'
import { Field, SelectField, Btn } from '@/components/ui/Form'

const PAGE_SIZE = 20

interface FormState {
  username: string
  password: string
  real_name: string
  department: string
  group_id: string
  remark: string
}

const emptyForm = (): FormState => ({
  username: '', password: '', real_name: '', department: '', group_id: '', remark: '',
})

export default function Users() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [modal, setModal] = useState<{ open: boolean; user?: User }>({ open: false })
  const [form, setForm] = useState<FormState>(emptyForm())
  const [err, setErr] = useState('')
  const [copied, setCopied] = useState<number | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['users', page],
    queryFn: () => userApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
  })

  const { data: groups } = useQuery({
    queryKey: ['groups-all'],
    queryFn: () => groupApi.list({ page: 1, page_size: 100 }).then(r => r.data.data?.list ?? []),
  })

  const groupOptions = (groups ?? []).map(g => ({ value: g.id, label: g.name }))

  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] })

  const save = useMutation({
    mutationFn: () => {
      const groupId = form.group_id ? Number(form.group_id) : undefined
      if (modal.user) {
        return userApi.update(modal.user.id, {
          password: form.password || undefined,
          real_name: form.real_name,
          department: form.department,
          group_id: groupId ?? null,
          remark: form.remark,
        })
      }
      return userApi.create({ ...form, group_id: groupId })
    },
    onSuccess: () => { invalidate(); closeModal() },
    onError: (e: Error) => setErr(e.message),
  })

  const toggle = useMutation({
    mutationFn: (id: number) => userApi.toggle(id),
    onSuccess: invalidate,
  })

  const remove = useMutation({
    mutationFn: (id: number) => userApi.remove(id),
    onSuccess: invalidate,
  })

  const openCreate = () => { setForm(emptyForm()); setErr(''); setModal({ open: true }) }
  const openEdit = (u: User) => {
    setForm({ username: u.username, password: '', real_name: u.real_name,
      department: u.department, group_id: String(u.group_id ?? ''), remark: u.remark })
    setErr('')
    setModal({ open: true, user: u })
  }
  const closeModal = () => setModal({ open: false })

  const copyURL = (u: User) => {
    navigator.clipboard.writeText(u.subscribe_url)
    setCopied(u.id)
    setTimeout(() => setCopied(null), 2000)
  }

  const f = (k: keyof FormState) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm(prev => ({ ...prev, [k]: e.target.value }))

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'username', label: '用户名', render: (u: User) => <span className="font-medium">{u.username}</span> },
    { key: 'real_name', label: '姓名' },
    { key: 'department', label: '部门' },
    { key: 'group_name', label: '分组', render: (u: User) => u.group_name ? <Badge label={u.group_name} variant="blue" /> : <span className="text-gray-400">—</span> },
    { key: 'active', label: '状态', render: (u: User) => <Badge label={u.active ? '启用' : '禁用'} variant={u.active ? 'green' : 'gray'} /> },
    {
      key: 'subscribe_url', label: '订阅',
      render: (u: User) => (
        <button onClick={() => copyURL(u)} className="text-blue-500 hover:text-blue-700 flex items-center gap-1 text-xs">
          {copied === u.id ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
          复制链接
        </button>
      ),
    },
    {
      key: 'actions', label: '操作',
      render: (u: User) => (
        <div className="flex gap-2">
          <button onClick={() => openEdit(u)} className="text-xs text-gray-600 hover:text-gray-900">编辑</button>
          <button onClick={() => toggle.mutate(u.id)} className="text-xs text-blue-500 hover:text-blue-700">
            {u.active ? '禁用' : '启用'}
          </button>
          <button onClick={() => { if (confirm(`确认删除用户 ${u.username}？`)) remove.mutate(u.id) }}
            className="text-xs text-red-500 hover:text-red-700">删除</button>
        </div>
      ),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-xl font-semibold text-gray-900">用户管理</h2>
          <p className="text-sm text-gray-500 mt-0.5">管理订阅用户及其分组 · 共 {data?.total ?? 0} 人</p>
        </div>
        <Btn onClick={openCreate}>新增用户</Btn>
      </div>

      <div className="bg-white rounded-lg border border-gray-200">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </div>

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
        <div className="space-y-3">
          {!modal.user && <Field label="用户名 *" value={form.username} onChange={f('username')} placeholder="登录用户名" />}
          <Field label={modal.user ? '新密码（留空不修改）' : '密码 *'} type="password" value={form.password} onChange={f('password')} />
          <Field label="真实姓名" value={form.real_name} onChange={f('real_name')} />
          <Field label="部门" value={form.department} onChange={f('department')} />
          <SelectField label="所属分组" value={form.group_id} onChange={v => setForm(p => ({ ...p, group_id: v }))}
            options={groupOptions} placeholder="不分组" />
          <Field label="备注" value={form.remark} onChange={f('remark')} />
          {err && <p className="text-sm text-red-500">{err}</p>}
        </div>
      </Modal>
    </div>
  )
}
