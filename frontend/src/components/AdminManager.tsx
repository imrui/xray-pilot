import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Copy, Dices, KeyRound, Plus, ShieldCheck, Trash2 } from 'lucide-react'
import { adminApi } from '@/lib/api'
import type { Admin, AdminRole } from '@/types'
import { useAuthStore } from '@/store/auth'
import { Modal } from '@/components/ui/Modal'
import { Btn, Field, SelectField } from '@/components/ui/Form'
import { Badge } from '@/components/ui/Badge'
import { useConfirm } from '@/components/ui/ConfirmProvider'
import { HintTip } from '@/components/ui/HintTip'
import { Section } from '@/pages/settings/shared'
import { pushToast } from '@/lib/notify'
import { copyText } from '@/lib/clipboard'
import { generatePassword } from '@/lib/keygen'

const ROLE_OPTIONS: Array<{ value: AdminRole; label: string }> = [
  { value: 'admin', label: '管理员' },
  { value: 'super_admin', label: '超级管理员' },
]

const roleLabel = (r: AdminRole) => (r === 'super_admin' ? '超级管理员' : '管理员')

// 管理员账号列表 + 新建 / 改角色 / 启停 / 重置密码 / 删除。仅 super_admin 可见（Settings 页按角色渲染）。
// 护栏（最后一个超级管理员；对自己不能删/禁/改角色/重置密码）由后端强制，前端只做置灰提示。
export function AdminManager() {
  const qc = useQueryClient()
  const confirm = useConfirm()
  const selfID = useAuthStore((s) => s.username)
  const [createOpen, setCreateOpen] = useState(false)
  const [resetTarget, setResetTarget] = useState<Admin | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admins'],
    queryFn: () => adminApi.list().then((r) => r.data.data ?? []),
  })
  const invalidate = () => void qc.invalidateQueries({ queryKey: ['admins'] })

  const update = useMutation({
    mutationFn: ({ id, ...patch }: { id: number; role?: AdminRole; active?: boolean }) => adminApi.update(id, patch),
    onSuccess: invalidate,
  })
  const remove = useMutation({
    mutationFn: (id: number) => adminApi.remove(id),
    onSuccess: () => {
      pushToast({ title: '管理员已删除', variant: 'success' })
      invalidate()
    },
  })

  const admins = data ?? []

  return (
    <Section
      title="管理员账号"
      description="登录面板的管理员。首次启动从 config.yaml 的 admins 段种入，之后以此处为准。"
      actions={
        <>
          <HintTip content="超级管理员可管理其他账号，对自己只能修改密码。普通管理员拥有全部业务操作权限，仅可修改自己的密码。系统至少保留一个启用的超级管理员。" />
          <Btn variant="secondary" className="shrink-0 whitespace-nowrap" onClick={() => setCreateOpen(true)}>
            <Plus className="h-4 w-4" />
            新建管理员
          </Btn>
        </>
      }
    >
      <div className="border-t border-[var(--border)]">
        {isLoading ? (
          <div className="py-6 text-sm text-soft">加载中…</div>
        ) : (
          <ul className="divide-y divide-[var(--border)]">
            {admins.map((a) => {
              const isSelf = a.username === selfID
              return (
                <li key={a.id} className="flex flex-wrap items-center justify-between gap-3 py-3 text-sm">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-[var(--accent)]">
                      <ShieldCheck className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-medium">{a.username}</span>
                        {isSelf && <Badge label="当前登录" variant="blue" />}
                        {!a.active && <Badge label="已禁用" variant="red" />}
                      </div>
                      <div className="mt-0.5 text-xs text-soft">
                        {a.last_login_at ? `最近登录 ${new Date(a.last_login_at).toLocaleString('zh-CN')}` : '尚未登录'}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <select
                      value={a.role}
                      disabled={isSelf || update.isPending}
                      onChange={(e) => update.mutate({ id: a.id, role: e.target.value as AdminRole })}
                      className="h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-xs text-[var(--text)] outline-none focus:border-[var(--accent)] disabled:opacity-60"
                      title={isSelf ? '不能修改自己的角色' : '角色'}
                    >
                      {ROLE_OPTIONS.map((o) => (
                        <option key={o.value} value={o.value}>{o.label}</option>
                      ))}
                    </select>
                    <Btn
                      variant="ghost"
                      disabled={isSelf || update.isPending}
                      title={isSelf ? '不能禁用自己' : a.active ? '禁用' : '启用'}
                      onClick={() => update.mutate({ id: a.id, active: !a.active })}
                    >
                      {a.active ? '禁用' : '启用'}
                    </Btn>
                    <Btn
                      variant="ghost"
                      disabled={isSelf}
                      title={isSelf ? '自己的密码请在头像菜单中修改' : '重置密码'}
                      onClick={() => setResetTarget(a)}
                    >
                      <KeyRound className="h-4 w-4" />
                    </Btn>
                    <Btn
                      variant="ghost"
                      disabled={isSelf || remove.isPending}
                      title={isSelf ? '不能删除自己' : '删除'}
                      onClick={async () => {
                        const ok = await confirm({
                          title: `删除管理员「${a.username}」？`,
                          description: `该账号（${roleLabel(a.role)}）将立即失去登录能力，操作日志中的历史记录保留。`,
                          confirmText: '删除',
                          cancelText: '取消',
                          tone: 'danger',
                        })
                        if (ok) remove.mutate(a.id)
                      }}
                    >
                      <Trash2 className="h-4 w-4 text-[var(--danger)]" />
                    </Btn>
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </div>

      <CreateAdminModal open={createOpen} onClose={() => setCreateOpen(false)} onCreated={invalidate} />
      <ResetPasswordModal target={resetTarget} onClose={() => setResetTarget(null)} />
    </Section>
  )
}

function CreateAdminModal({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<AdminRole>('admin')
  // 创建成功后保留一份凭据供复制（密码只在这一刻可见，关闭后不可再取）
  const [created, setCreated] = useState<{ username: string; password: string; role: AdminRole } | null>(null)

  const reset = () => {
    setUsername('')
    setPassword('')
    setRole('admin')
    setCreated(null)
  }
  const close = () => {
    reset()
    onClose()
  }

  const create = useMutation({
    mutationFn: () => adminApi.create({ username: username.trim(), password, role }),
    onSuccess: () => {
      setCreated({ username: username.trim(), password, role })
      onCreated()
    },
  })
  const canSubmit = username.trim() !== '' && password.length >= 6

  return (
    <Modal
      open={open}
      onClose={close}
      title={created ? '管理员已创建' : '新建管理员'}
      size="sm"
      footer={
        created ? (
          <Btn onClick={close}>完成</Btn>
        ) : (
          <>
            <Btn variant="secondary" onClick={close}>取消</Btn>
            <Btn loading={create.isPending} disabled={!canSubmit} onClick={() => create.mutate()}>创建</Btn>
          </>
        )
      }
    >
      {created ? (
        <div className="space-y-4">
          <p className="text-sm text-soft">
            {created.username}（{roleLabel(created.role)}）已创建。初始密码仅此处可见，请立即复制并转交本人；对方登录后可自行修改。
          </p>
          <CredentialRow label="用户名" value={created.username} />
          <CredentialRow label="初始密码" value={created.password} mono />
          <CopyAllButton text={`用户名：${created.username}\n密码：${created.password}`} />
        </div>
      ) : (
        <div className="space-y-4">
          <Field label="用户名" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1">
              <Field label="初始密码（至少 6 位）" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
            </div>
            <Btn variant="secondary" className="shrink-0" title="生成 16 位随机密码" onClick={() => setPassword(generatePassword())}>
              <Dices className="h-4 w-4" />
              随机
            </Btn>
          </div>
          <SelectField label="角色" value={role} onChange={(v) => setRole(v as AdminRole)} options={ROLE_OPTIONS} />
        </div>
      )}
    </Modal>
  )
}

// 单条凭据：值 + 复制按钮，复制成功后按钮短暂变为对勾
function CredentialRow({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    const ok = await copyText(value)
    if (!ok) {
      pushToast({ title: '复制失败', description: '请手动选中文本复制。', variant: 'warning' })
      return
    }
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <div className="flex items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-2">
      <div className="min-w-0 flex-1">
        <div className="text-[11px] font-medium uppercase tracking-[0.12em] text-faint">{label}</div>
        <div className={mono ? 'truncate font-mono text-sm' : 'truncate text-sm'}>{value}</div>
      </div>
      <Btn variant="ghost" className="shrink-0" title={`复制${label}`} onClick={() => void copy()}>
        {copied ? <Check className="h-4 w-4 text-[var(--success)]" /> : <Copy className="h-4 w-4" />}
      </Btn>
    </div>
  )
}

function CopyAllButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Btn
      variant="secondary"
      className="w-full"
      onClick={async () => {
        const ok = await copyText(text)
        if (!ok) {
          pushToast({ title: '复制失败', description: '请手动选中文本复制。', variant: 'warning' })
          return
        }
        setCopied(true)
        setTimeout(() => setCopied(false), 1500)
      }}
    >
      {copied ? <Check className="h-4 w-4 text-[var(--success)]" /> : <Copy className="h-4 w-4" />}
      {copied ? '已复制' : '复制用户名和密码'}
    </Btn>
  )
}

function ResetPasswordModal({ target, onClose }: { target: Admin | null; onClose: () => void }) {
  const [password, setPassword] = useState('')
  // 重置成功后保留凭据供复制；关闭即清空
  const [done, setDone] = useState<{ username: string; password: string } | null>(null)
  const close = () => {
    setPassword('')
    setDone(null)
    onClose()
  }
  const reset = useMutation({
    mutationFn: () => adminApi.setPassword(target!.id, password),
    onSuccess: () => setDone({ username: target!.username, password }),
  })
  return (
    <Modal
      open={target !== null}
      onClose={close}
      title={done ? '密码已重置' : `重置密码 · ${target?.username ?? ''}`}
      size="sm"
      footer={
        done ? (
          <Btn onClick={close}>完成</Btn>
        ) : (
          <>
            <Btn variant="secondary" onClick={close}>取消</Btn>
            <Btn loading={reset.isPending} disabled={password.length < 6} onClick={() => reset.mutate()}>重置</Btn>
          </>
        )
      }
    >
      {done ? (
        <div className="space-y-4">
          <p className="text-sm text-soft">{done.username} 的密码已更新，新密码仅此处可见，请复制后转交本人。对方已登录的会话在 token 过期前仍然有效。</p>
          <CredentialRow label="用户名" value={done.username} />
          <CredentialRow label="新密码" value={done.password} mono />
          <CopyAllButton text={`用户名：${done.username}\n密码：${done.password}`} />
        </div>
      ) : (
        <div className="space-y-3">
          <p className="text-sm text-soft">超级管理员重置密码无需旧密码。</p>
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1">
              <Field label="新密码（至少 6 位）" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
            </div>
            <Btn variant="secondary" className="shrink-0" title="生成 16 位随机密码" onClick={() => setPassword(generatePassword())}>
              <Dices className="h-4 w-4" />
              随机
            </Btn>
          </div>
        </div>
      )}
    </Modal>
  )
}
