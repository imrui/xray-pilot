import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CalendarDays, ChevronDown, ChevronUp, Copy, ExternalLink, Filter, PencilLine, Plus, QrCode, RefreshCw, Search, Send, Unplug, X } from 'lucide-react'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { userApi, groupApi } from '@/lib/api'
import { formatBytes } from '@/lib/utils'
import type { FeishuPushResult, User } from '@/types'
import { Badge } from '@/components/ui/Badge'
import { Field, Btn, FieldGroup } from '@/components/ui/Form'
import { QRModal } from '@/components/ui/QRModal'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { Drawer } from '@/components/ui/Drawer'
import { ActionMenu } from '@/components/ui/ActionMenu'
import { useConfirm } from '@/components/ui/ConfirmProvider'
import { Tooltip } from '@/components/ui/Tooltip'
import { pushToast } from '@/lib/notify'

const DEFAULT_PAGE_SIZE = 10
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

interface FormState {
  username: string
  real_name: string
  group_ids: string[]
  expires_at: string
  remark: string
  feishu_enabled: string
  feishu_email: string
  feishu_open_id: string
  feishu_union_id: string
  feishu_chat_id: string
}

const emptyForm = (): FormState => ({
  username: '',
  real_name: '',
  group_ids: [],
  expires_at: '',
  remark: '',
  feishu_enabled: 'false',
  feishu_email: '',
  feishu_open_id: '',
  feishu_union_id: '',
  feishu_chat_id: '',
})

function isFeishuBound(user: User) {
  return Boolean(user.feishu_open_id || user.feishu_chat_id || user.feishu_union_id)
}

function isFeishuMessagingEnabled(user: User) {
  return Boolean(user.feishu_enabled) && isFeishuBound(user)
}

function isFeishuAuthorized(user: User) {
  return Boolean(user.feishu_enabled && user.feishu_email)
}

function getFeishuEmailMeta(user: User) {
  const email = user.feishu_email?.trim() || ''
  const bound = isFeishuBound(user)

  return {
    emailLabel: email,
    emailClassName: email
      ? bound
        ? 'text-[var(--text)]'
        : 'text-orange-600 dark:text-orange-400'
      : 'text-soft',
    tooltipContent: email
      ? bound
        ? (
            <span className="block space-y-1 whitespace-nowrap">
              <span className="block">
                <span className="font-medium text-emerald-600 dark:text-emerald-400">已绑定</span>
                <span className="text-[var(--text)]">，点击可发送飞书订阅</span>
              </span>
              <span className="block">Open ID: {user.feishu_open_id ? 'ok' : '—'}</span>
              <span className="block">Union ID: {user.feishu_union_id ? 'ok' : '—'}</span>
            </span>
          )
        : <span className="block whitespace-nowrap font-medium text-orange-600 dark:text-orange-400">待绑定</span>
      : null,
  }
}

function maskIdentifier(value?: string) {
  if (!value) return '—'
  if (value.length <= 12) return value
  return `${value.slice(0, 6)}...${value.slice(-4)}`
}

function describeFeishuPush(result: FeishuPushResult) {
  const parts = [
    `发送 ${result.sent}`,
    `跳过 ${result.skipped}`,
    `失败 ${result.failed}`,
  ]
  return parts.join(' · ')
}

function toApiDateTime(value: string) {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toISOString()
}

function toDateTimeLocalValue(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value.slice(0, 16)
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
  return local.toISOString().slice(0, 16)
}

function splitDateTimeLocalValue(value: string) {
  if (!value) return { date: '', time: '' }
  const [date = '', time = ''] = value.split('T')
  return { date, time: time.slice(0, 5) }
}

function joinDateTimeLocalValue(date: string, time: string) {
  if (!date) return ''
  return `${date}T${time || '00:00'}`
}

function formatDateTimeLocalDisplay(value: string) {
  if (!value) return '永久有效'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value.replace('T', ' ')
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).replace(/\//g, '/')
}

function isLocalDateTimeExpired(value: string) {
  if (!value) return false
  const date = new Date(value)
  return !Number.isNaN(date.getTime()) && date.getTime() < Date.now()
}

function openNativePicker(input: HTMLInputElement | null) {
  if (!input) return
  input.focus()
  ;(input as HTMLInputElement & { showPicker?: () => void }).showPicker?.()
}

function renderUserGroups(user: User) {
  const groups = user.groups ?? []
  if (groups.length === 0) return <span className="text-xs text-soft">未分组</span>

  const visible = groups.slice(0, 2)
  const hiddenCount = groups.length - visible.length

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {visible.map((group) => (
        <Badge key={group.id} label={group.name} variant="blue" />
      ))}
      {hiddenCount > 0 && (
        <Tooltip
          side="right"
          content={
            <div className="max-h-48 space-y-1 overflow-y-auto">
              {groups.map((group) => (
                <div key={group.id}>{group.name}</div>
              ))}
            </div>
          }
        >
          <span className="inline-flex h-7 items-center rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-xs text-soft">
            +{hiddenCount}
          </span>
        </Tooltip>
      )}
    </div>
  )
}

function GroupMultiSelect({
  options,
  selectedValues,
  onChange,
}: {
  options: Array<{ value: number; label: string }>
  selectedValues: string[]
  onChange: (next: string[]) => void
}) {
  const selectedOptions = options.filter((option) => selectedValues.includes(String(option.value)))
  const visible = selectedOptions.slice(0, 2)
  const hiddenCount = selectedOptions.length - visible.length

  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild>
        <button
          type="button"
          className="flex min-h-11 w-full items-center justify-between rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-left text-sm text-[var(--text)] transition hover:bg-[var(--panel-muted)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
        >
          <div className="flex min-h-6 flex-1 flex-wrap items-center gap-1.5">
            {selectedOptions.length === 0 ? (
              <span className="text-soft">选择一个或多个分组</span>
            ) : (
              <>
                {visible.map((option) => (
                  <Badge key={option.value} label={option.label} variant="blue" />
                ))}
                {hiddenCount > 0 && (
                  <span className="inline-flex h-7 items-center rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-2 text-xs text-soft">
                    +{hiddenCount}
                  </span>
                )}
              </>
            )}
          </div>
          <div className="ml-3 flex items-center gap-1.5">
            {selectedOptions.length > 0 && (
              <span
                role="button"
                tabIndex={0}
                onClick={(event) => {
                  event.preventDefault()
                  event.stopPropagation()
                  onChange([])
                }}
                onKeyDown={(event) => {
                  if (event.key !== 'Enter' && event.key !== ' ') return
                  event.preventDefault()
                  event.stopPropagation()
                  onChange([])
                }}
                className="inline-flex h-7 w-7 items-center justify-center rounded-md text-faint transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                aria-label="清空已选分组"
              >
                <X className="h-4 w-4" />
              </span>
            )}
            <ChevronDown className="h-4 w-4 shrink-0 text-faint" />
          </div>
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          sideOffset={8}
          align="start"
          className="z-50 w-[var(--radix-dropdown-menu-trigger-width)] max-w-[min(32rem,90vw)] rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] p-2 shadow-[var(--shadow-soft)]"
        >
          <div className="flex items-center justify-between px-2 py-1">
            <div className="text-xs text-soft">已选 {selectedOptions.length} 个分组</div>
            {selectedOptions.length > 0 && (
              <button
                type="button"
                onClick={() => onChange([])}
                className="inline-flex h-7 items-center gap-1 rounded-md px-2 text-xs text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
              >
                <X className="h-3.5 w-3.5" />
                清空
              </button>
            )}
          </div>
          <div className="mt-1 max-h-64 overflow-y-auto">
            {options.length === 0 ? (
              <div className="px-2 py-3 text-sm text-soft">暂无可选分组</div>
            ) : (
              options.map((option) => {
                const checked = selectedValues.includes(String(option.value))
                return (
                  <DropdownMenu.CheckboxItem
                    key={option.value}
                    checked={checked}
                    onSelect={(event) => event.preventDefault()}
                    onCheckedChange={(nextChecked) => {
                      const nextValue = String(option.value)
                      onChange(
                        nextChecked
                          ? [...selectedValues, nextValue]
                          : selectedValues.filter((value) => value !== nextValue),
                      )
                    }}
                    className="flex cursor-pointer items-center justify-between rounded-lg px-3 py-2 text-sm text-[var(--text)] outline-none transition hover:bg-[var(--panel-muted)] data-[highlighted]:bg-[var(--panel-muted)]"
                  >
                    <span>{option.label}</span>
                    <span className={`h-4 w-4 rounded border ${checked ? 'border-emerald-500 bg-emerald-500' : 'border-[var(--border-strong)] bg-transparent'}`} />
                  </DropdownMenu.CheckboxItem>
                )
              })
            )}
          </div>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function ExpiryDateTimeField({
  label,
  value,
  onChange,
}: {
  label: string
  value: string
  onChange: (next: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [draftValue, setDraftValue] = useState(value)
  const dateInputRef = useRef<HTMLInputElement | null>(null)
  const timeInputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (!open) setDraftValue(value)
  }, [open, value])

  const originalValue = value
  const currentDisplay = formatDateTimeLocalDisplay(value)
  const currentExpired = isLocalDateTimeExpired(value)
  const draftExpired = isLocalDateTimeExpired(draftValue)
  const { date: draftDate, time: draftTime } = splitDateTimeLocalValue(draftValue)

  const applyToday = () => {
    const now = new Date()
    const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000).toISOString().slice(0, 16)
    setDraftValue(local)
  }

  const resetDraft = () => setDraftValue(originalValue)

  return (
    <div className="flex flex-col space-y-1.5">
      <label className="text-[12px] font-medium text-soft">{label}</label>

      <DropdownMenu.Root modal={false} open={open} onOpenChange={setOpen}>
        <DropdownMenu.Trigger asChild>
          <button
            type="button"
            className={`flex h-10 w-full items-center justify-between rounded-md border bg-[var(--panel-strong)] px-3 text-left text-sm transition-all duration-200 focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)] ${
              currentExpired
                ? 'border-rose-300 text-rose-500 focus:border-rose-400'
                : 'border-[var(--border)] text-[var(--text)] focus:border-[var(--accent)]'
            }`}
          >
            <span className={value ? '' : 'text-faint'}>{currentDisplay}</span>
            <span className="ml-3 flex items-center gap-1">
              {value && (
                <span
                  role="button"
                  tabIndex={0}
                  onClick={(event) => {
                    event.preventDefault()
                    event.stopPropagation()
                    onChange('')
                    setDraftValue('')
                    setOpen(false)
                  }}
                  onKeyDown={(event) => {
                    if (event.key !== 'Enter' && event.key !== ' ') return
                    event.preventDefault()
                    event.stopPropagation()
                    onChange('')
                    setDraftValue('')
                    setOpen(false)
                  }}
                  className="inline-flex h-7 w-7 items-center justify-center rounded-md text-faint transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                  aria-label="清空过期时间"
                >
                  <X className="h-4 w-4" />
                </span>
              )}
              <CalendarDays className="h-4 w-4 shrink-0 text-faint" />
            </span>
          </button>
        </DropdownMenu.Trigger>

        <DropdownMenu.Portal>
          <DropdownMenu.Content
            sideOffset={8}
            align="start"
            onCloseAutoFocus={(event) => event.preventDefault()}
            className="z-50 w-[min(28rem,calc(100vw-2rem))] rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] p-3 shadow-[var(--shadow-soft)]"
          >
            <div className="space-y-3">
              <div className="grid gap-3 sm:grid-cols-[1.45fr_1fr]">
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-soft">日期</span>
                  <button
                    type="button"
                    onClick={() => openNativePicker(dateInputRef.current)}
                    className="relative h-10 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] text-left transition hover:border-[var(--accent)]/40 focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                  >
                    <input
                      ref={dateInputRef}
                      type="date"
                      value={draftDate}
                      onChange={(event) => setDraftValue(joinDateTimeLocalValue(event.target.value, draftTime))}
                      onClick={() => openNativePicker(dateInputRef.current)}
                      onFocus={() => openNativePicker(dateInputRef.current)}
                      className="h-full w-full cursor-pointer rounded-md bg-transparent px-3 text-sm text-[var(--text)] outline-none"
                    />
                  </button>
                </label>
                <label className="flex flex-col gap-1.5">
                  <span className="text-xs font-medium text-soft">时间</span>
                  <button
                    type="button"
                    onClick={() => openNativePicker(timeInputRef.current)}
                    className="relative h-10 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] text-left transition hover:border-[var(--accent)]/40 focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
                  >
                    <input
                      ref={timeInputRef}
                      type="time"
                      step={60}
                      value={draftTime}
                      onChange={(event) => setDraftValue(joinDateTimeLocalValue(draftDate, event.target.value))}
                      onClick={() => openNativePicker(timeInputRef.current)}
                      onFocus={() => openNativePicker(timeInputRef.current)}
                      className="h-full w-full cursor-pointer rounded-md bg-transparent px-3 text-sm text-[var(--text)] outline-none"
                    />
                  </button>
                </label>
              </div>

              <div className={`text-xs ${draftExpired ? 'text-rose-500' : 'text-soft'}`}>
                {draftValue
                  ? draftExpired
                    ? '该时间早于当前时间，保存后用户会被视为已过期。'
                    : `当前选择：${formatDateTimeLocalDisplay(draftValue)}`
                  : '未设置过期时间时，用户将保持永久有效。'}
              </div>

              <div className="flex items-center justify-between gap-2 border-t border-[var(--border)] pt-3">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={resetDraft}
                    className="inline-flex h-9 items-center rounded-md px-3 text-sm text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                  >
                    重置
                  </button>
                  <button
                    type="button"
                    onClick={applyToday}
                    className="inline-flex h-9 items-center rounded-md px-3 text-sm text-[var(--accent)] transition hover:bg-[var(--panel-muted)]"
                  >
                    今天
                  </button>
                </div>
                <button
                  type="button"
                  onClick={() => {
                    onChange(draftValue)
                    setOpen(false)
                  }}
                  className="inline-flex h-9 items-center rounded-md border border-[var(--accent)] bg-[var(--accent)] px-3 text-sm font-medium text-white transition hover:bg-[var(--accent-strong)]"
                >
                  确定
                </button>
              </div>
            </div>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </div>
  )
}

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
  const [copiedUserId, setCopiedUserId] = useState<number | null>(null)
  const [showFeishuDetails, setShowFeishuDetails] = useState(false)

  const { data, isLoading } = useQuery({
    queryKey: ['users', page, pageSize],
    queryFn: () => userApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
  })

  const { data: groups } = useQuery({
    queryKey: ['groups-all'],
    queryFn: () => groupApi.list({ page: 1, page_size: 100 }).then((r) => r.data.data?.list ?? []),
  })

  const sortedGroups = useMemo(
    () => [...(groups ?? [])].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' })),
    [groups],
  )
  const groupOptions = sortedGroups.map((g) => ({ value: g.id, label: g.name }))
  const invalidate = () => qc.invalidateQueries({ queryKey: ['users'] })

  useEffect(() => {
    if (copiedUserId == null) return
    const timer = setTimeout(() => setCopiedUserId(null), 1800)
    return () => clearTimeout(timer)
  }, [copiedUserId])

  const save = useMutation({
    mutationFn: () => {
      const groupIds = form.group_ids.map(Number).filter((value) => !Number.isNaN(value))
      const expiresAt = toApiDateTime(form.expires_at)
      if (drawer.user) {
        return userApi.update(drawer.user.id, {
          username: form.username,
          real_name: form.real_name,
          group_ids: groupIds,
          expires_at: expiresAt,
          remark: form.remark,
          feishu_enabled: form.feishu_enabled === 'true',
          feishu_email: form.feishu_email,
          feishu_open_id: form.feishu_open_id,
          feishu_union_id: form.feishu_union_id,
          feishu_chat_id: form.feishu_chat_id,
        })
      }
      return userApi.create({
        username: form.username,
        real_name: form.real_name,
        group_ids: groupIds,
        expires_at: expiresAt,
        remark: form.remark,
        feishu_enabled: form.feishu_enabled === 'true',
        feishu_email: form.feishu_email,
        feishu_open_id: form.feishu_open_id,
        feishu_union_id: form.feishu_union_id,
        feishu_chat_id: form.feishu_chat_id,
      })
    },
    onSuccess: () => {
      invalidate()
      closeDrawer()
    },
    onError: (e: Error) => setErr(e.message),
  })

  const toggle = useMutation({ mutationFn: (id: number) => userApi.toggle(id), onSuccess: invalidate })
  const toggleFeishuEnabled = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      userApi.update(id, { feishu_enabled: enabled }),
    onSuccess: (_, variables) => {
      invalidate()
      pushToast({
        title: variables.enabled ? '飞书消息已开启' : '飞书消息已关闭',
        description: variables.enabled ? '该用户后续可接收飞书消息。' : '该用户将暂停飞书消息能力，但绑定信息会保留。',
        variant: 'success',
      })
    },
  })
  const remove = useMutation({ mutationFn: (id: number) => userApi.remove(id), onSuccess: invalidate })
  const resetToken = useMutation({ mutationFn: (id: number) => userApi.resetToken(id), onSuccess: invalidate })
  const pushFeishuSingle = useMutation({
    mutationFn: (id: number) => userApi.pushFeishu(id),
    onSuccess: (res) => {
      const result = res.data.data
      if (!result) return
      pushToast({
        title: '飞书订阅已处理',
        description: describeFeishuPush(result),
        variant: result.failed > 0 ? 'warning' : 'success',
      })
    },
  })
  const pushFeishuBatch = useMutation({
    mutationFn: (userIDs: number[]) => userApi.pushFeishuBatch(userIDs),
    onSuccess: (res) => {
      const result = res.data.data
      if (!result) return
      pushToast({
        title: '批量飞书推送已完成',
        description: describeFeishuPush(result),
        variant: result.failed > 0 ? 'warning' : 'success',
      })
    },
  })

  const openCreate = () => {
    const next = emptyForm()
    setForm(next)
    setInitialForm(next)
    setErr('')
    setShowFeishuDetails(false)
    setDrawer({ open: true })
  }

  const openEdit = (u: User) => {
    const next = {
      username: u.username,
      real_name: u.real_name,
      group_ids: (u.group_ids ?? []).map(String),
      expires_at: toDateTimeLocalValue(u.expires_at),
      remark: u.remark,
      feishu_enabled: u.feishu_enabled ? 'true' : 'false',
      feishu_email: u.feishu_email ?? '',
      feishu_open_id: u.feishu_open_id ?? '',
      feishu_union_id: u.feishu_union_id ?? '',
      feishu_chat_id: u.feishu_chat_id ?? '',
    }
    setForm(next)
    setInitialForm(next)
    setErr('')
    setShowFeishuDetails(false)
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
        (u.feishu_email ?? '').toLowerCase().includes(keyword) ||
        (u.group_names ?? []).join(' ').toLowerCase().includes(keyword)

      const matchesStatus =
        statusFilter === 'all' ||
        (statusFilter === 'active' && u.active) ||
        (statusFilter === 'inactive' && !u.active) ||
        (statusFilter === 'expired' && isExpired(u))

      const matchesGroup = groupFilter === 'all' || (u.group_ids ?? []).some((id) => String(id) === groupFilter)

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
  const selectedBoundUsers = selectedUsers.filter(isFeishuMessagingEnabled)

  const runBulk = async (runner: (u: User) => Promise<unknown>) => {
    await Promise.all(selectedUsers.map(runner))
    setSelectedIds([])
    invalidate()
  }

  const handleBulkToggle = async () => {
    const willDisable = selectedUsers.filter((u) => u.active)
    if (willDisable.length > 0) {
      const ok = await confirm({
        title: `禁用所选用户中的 ${willDisable.length} 个启用用户？`,
        description: '禁用后这些用户的订阅与节点连接会立即失效。',
        confirmText: '确认禁用',
        cancelText: '取消',
        tone: 'danger',
      })
      if (!ok) return
    }
    await runBulk((u) => userApi.toggle(u.id))
  }

  const copySubscribeUrl = async (user: User) => {
    await navigator.clipboard.writeText(user.subscribe_url)
    setCopiedUserId(user.id)
    pushToast({
      title: '订阅链接已复制',
      description: `${user.username} 的订阅地址已复制到剪贴板。`,
      variant: 'success',
    })
  }

  const toggleUserActive = async (user: User) => {
    if (user.active) {
      const ok = await confirm({
        title: `禁用用户 ${user.username}？`,
        description: '禁用后该用户的订阅与节点连接会立即失效。',
        confirmText: '确认禁用',
        cancelText: '取消',
        tone: 'danger',
      })
      if (!ok) return
    }
    toggle.mutate(user.id)
  }

  const confirmAndPushFeishu = async (user: User) => {
    const ok = await confirm({
      title: `给 ${user.username} 发送飞书订阅？`,
      description: '会通过当前绑定的飞书身份把订阅信息推送给该用户。',
      confirmText: '确认发送',
      cancelText: '取消',
    })
    if (!ok) return
    pushFeishuSingle.mutate(user.id)
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
              placeholder="搜索用户名、飞书邮箱..."
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
              <Btn variant="secondary" onClick={() => void handleBulkToggle()}>批量切换启用</Btn>
              {selectedBoundUsers.length > 0 && (
                <Btn
                  variant="secondary"
                  loading={pushFeishuBatch.isPending}
                  onClick={() => pushFeishuBatch.mutate(selectedBoundUsers.map((u) => u.id))}
                >
                  <Send className="h-4 w-4" />
                  批量发送飞书订阅
                </Btn>
              )}
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
          <table className="w-full min-w-[1140px] text-left text-sm">
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
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">累计流量</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">飞书消息</th>
                <th className="px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">飞书邮箱</th>
                <th className="px-4 py-3 text-right text-[11px] font-medium uppercase tracking-[0.12em] text-faint">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border)]">
              {isLoading ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-soft">加载中…</td>
                </tr>
              ) : filteredUsers.length === 0 ? (
                <tr>
                  <td colSpan={9} className="px-4 py-10 text-center text-soft">暂无用户数据</td>
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
                      <div className="flex items-center gap-2">
                        <span className="font-semibold">{u.username}</span>
                        <span className="text-xs text-faint">#{u.id}</span>
                      </div>
                      <div className="text-xs text-soft">
                        {u.real_name || '--'}
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      {renderUserGroups(u)}
                    </td>
                    <td className="px-4 py-3.5">
                      {u.expires_at ? (
                        <span className={isExpired(u) ? 'text-rose-400' : 'text-soft'}>
                          {formatDateTimeLocalDisplay(toDateTimeLocalValue(u.expires_at))}
                        </span>
                      ) : (
                        <span className="text-soft">永久</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5">
                      <Switch checked={u.active} onChange={() => void toggleUserActive(u)} />
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="space-y-0.5 text-xs leading-tight">
                        <div className="text-soft" title="累计上行字节">↑ {formatBytes(u.traffic_up_bytes)}</div>
                        <div className="text-soft" title="累计下行字节">↓ {formatBytes(u.traffic_down_bytes)}</div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <Switch
                        checked={Boolean(u.feishu_enabled)}
                        onChange={(next) => toggleFeishuEnabled.mutate({ id: u.id, enabled: next })}
                      />
                    </td>
                    <td className="px-4 py-3.5">
                      {(() => {
                        const feishu = getFeishuEmailMeta(u)
                        if (!feishu.emailLabel) return null

                        const emailNode = isFeishuMessagingEnabled(u) ? (
                          <button
                            type="button"
                            onClick={() => void confirmAndPushFeishu(u)}
                            className={`inline-block max-w-[220px] truncate text-left text-xs underline-offset-2 transition hover:underline ${feishu.emailClassName}`}
                          >
                            {feishu.emailLabel}
                          </button>
                        ) : (
                          <span className={`inline-block max-w-[220px] truncate text-xs ${feishu.emailClassName}`}>
                            {feishu.emailLabel}
                          </span>
                        )

                        return feishu.tooltipContent ? (
                          <Tooltip content={feishu.tooltipContent} side="right" className="whitespace-nowrap">
                            {emailNode}
                          </Tooltip>
                        ) : emailNode
                      })()}
                    </td>
                    <td className="px-4 py-3.5">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => void copySubscribeUrl(u)}
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                        >
                          <Copy className="h-3.5 w-3.5" />
                          {copiedUserId === u.id ? '已复制' : '复制'}
                        </button>
                        <button
                          onClick={() => setQrUser(u)}
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-[var(--accent)] transition hover:bg-[var(--panel-muted)]"
                        >
                          <QrCode className="h-3.5 w-3.5" />
                          二维码
                        </button>
                        <a
                          href={u.subscribe_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          订阅页
                        </a>
                        <button
                          onClick={() => openEdit(u)}
                          className="inline-flex h-9 items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-xs font-medium text-soft transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
                        >
                          <PencilLine className="h-3.5 w-3.5" />
                          编辑
                        </button>
                        <ActionMenu
                          items={[
                            ...(isFeishuMessagingEnabled(u)
                              ? [{
                                  label: '发送飞书订阅',
                                  onSelect: () => void confirmAndPushFeishu(u),
                                }]
                              : []),
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
                              onSelect: () => void toggleUserActive(u),
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
          <FieldGroup title="基础信息" description="用户标识、后台识别与订阅分组。">
            <Field label="用户名（客户端显示） *" value={form.username} onChange={f('username')} placeholder="如：alice01" />
            <Field label="真实姓名（后台识别）" value={form.real_name} onChange={f('real_name')} placeholder="后台识别用，可留空" />
            <div className="space-y-2">
              <div className="text-sm font-medium text-[var(--text)]">订阅分组</div>
              <GroupMultiSelect
                options={groupOptions}
                selectedValues={form.group_ids}
                onChange={(next) => setForm((prev) => ({ ...prev, group_ids: Array.from(new Set(next)).sort() }))}
              />
              <div className="text-xs text-soft">用户可以同时属于多个分组，将获得这些分组下节点的合集。</div>
            </div>
          </FieldGroup>

          <FieldGroup title="订阅策略" description="有效期为空时视为永久有效。">
            <ExpiryDateTimeField
              label="过期时间"
              value={form.expires_at}
              onChange={(next) => setForm((prev) => ({ ...prev, expires_at: next }))}
            />
            <Field label="备注" value={form.remark} onChange={f('remark')} placeholder="可选备注" />
          </FieldGroup>

          <FieldGroup title="飞书消息" description="可选能力。关闭后不发送飞书消息，但绑定信息会保留。">
            <div className="flex items-center justify-between rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2.5">
              <div>
                <div className="text-[12px] font-medium text-soft">启用飞书消息</div>
                <div className="mt-1 text-xs text-soft">开启后支持机器人识别和后台主动推送。</div>
              </div>
              <Switch
                checked={form.feishu_enabled === 'true'}
                onChange={(next) => setForm((p) => ({ ...p, feishu_enabled: next ? 'true' : 'false' }))}
              />
            </div>
            {form.feishu_enabled === 'true' && (
              <div className="space-y-4">
                <Field label="飞书邮箱" value={form.feishu_email} onChange={f('feishu_email')} placeholder="name@company.com" />
                {drawer.user ? (
                  <div className="flex flex-wrap gap-2">
                    <Btn
                      variant="secondary"
                      onClick={() => {
                        setErr('')
                        userApi.bindFeishu(drawer.user!.id, form.feishu_email)
                          .then((res) => {
                            const patch = res.data.data
                            if (!patch) return
                            const nextUser = {
                              ...drawer.user!,
                              ...patch,
                            } as User
                            pushToast({
                              title: nextUser.feishu_identity_ready ? '飞书绑定成功' : '飞书邮箱已校验',
                              description: nextUser.feishu_identity_ready
                                ? `Open ID ${maskIdentifier(nextUser.feishu_open_id)} · Union ID ${maskIdentifier(nextUser.feishu_union_id)}`
                                : `${nextUser.username} 已授权飞书邮箱，等待首次私聊机器人完成身份绑定。`,
                              variant: 'success',
                            })
                            openEdit(nextUser)
                            invalidate()
                          })
                          .catch((e: Error) => setErr(e.message))
                      }}
                      disabled={!form.feishu_email}
                    >
                      <Send className="h-4 w-4" />
                      检测飞书邮箱
                    </Btn>
                  </div>
                ) : (
                  <p className="text-xs text-soft">请先保存用户，再执行飞书绑定。</p>
                )}
                <div className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)]">
                  <button
                    type="button"
                    onClick={() => setShowFeishuDetails((prev) => !prev)}
                    className="flex w-full items-center justify-between px-3 py-2 text-sm font-medium text-[var(--text)]"
                  >
                    <span>查看绑定详情</span>
                    {showFeishuDetails ? <ChevronUp className="h-4 w-4 text-soft" /> : <ChevronDown className="h-4 w-4 text-soft" />}
                  </button>
                  {showFeishuDetails && (
                    <div className="space-y-4 border-t border-[var(--border)] p-3">
                      <div className="grid gap-4 md:grid-cols-2">
                        <Field label="Feishu Open ID" value={form.feishu_open_id} onChange={f('feishu_open_id')} placeholder="可手动填写" />
                        <Field label="Feishu Union ID" value={form.feishu_union_id} onChange={f('feishu_union_id')} placeholder="可手动填写" />
                      </div>
                      <Field label="Feishu Chat ID" value={form.feishu_chat_id} onChange={f('feishu_chat_id')} placeholder="可手动填写" />
                      {drawer.user && isFeishuAuthorized(drawer.user) && (
                        <div className="flex justify-start">
                          <Btn
                            variant="secondary"
                            onClick={() => {
                              setErr('')
                              userApi.unbindFeishu(drawer.user!.id)
                                .then((res) => {
                                  const patch = res.data.data
                                  if (!patch) return
                                  const nextUser = {
                                    ...drawer.user!,
                                    ...patch,
                                  } as User
                                  pushToast({
                                    title: '飞书绑定已清除',
                                    description: `${nextUser.username} 的飞书身份信息已移除。`,
                                    variant: 'success',
                                  })
                                  openEdit(nextUser)
                                  invalidate()
                                })
                                .catch((e: Error) => setErr(e.message))
                            }}
                          >
                            <Unplug className="h-4 w-4" />
                            清除飞书身份
                          </Btn>
                        </div>
                      )}
                    </div>
                  )}
                </div>
                {drawer.user && (
                  <p className="text-xs text-soft">
                    当前状态：
                    {isFeishuMessagingEnabled(drawer.user)
                      ? ` 已完成身份绑定${drawer.user.feishu_bound_at ? `，最近更新时间 ${new Date(drawer.user.feishu_bound_at).toLocaleString('zh-CN')}` : ''}`
                      : isFeishuAuthorized(drawer.user)
                        ? ' 已授权飞书邮箱，等待首次私聊完成身份绑定'
                        : ' 未绑定飞书'}
                  </p>
                )}
              </div>
            )}
          </FieldGroup>

          {drawer.user && (
            <FieldGroup title="快捷动作" description="把高频动作前置，不需要先关闭编辑抽屉。">
              <div className="flex flex-wrap gap-2">
                <Btn variant="secondary" onClick={() => void copySubscribeUrl(drawer.user!)}>
                  <Copy className="h-4 w-4" />
                  {copiedUserId === drawer.user!.id ? '已复制' : '复制订阅链接'}
                </Btn>
                <Btn variant="secondary" onClick={() => setQrUser(drawer.user!)}>
                  <QrCode className="h-4 w-4" />
                  查看订阅二维码
                </Btn>
                <a
                  href={drawer.user!.subscribe_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-[var(--border-strong)] bg-[var(--panel-strong)] px-4 text-sm font-medium text-[var(--text)] transition-all hover:border-[var(--accent)]/40 hover:bg-[var(--panel-muted)] focus:outline-none focus-visible:ring-2 focus-visible:ring-slate-400"
                >
                  <ExternalLink className="h-4 w-4" />
                  打开订阅页
                </a>
                {isFeishuMessagingEnabled(drawer.user!) && (
                  <Btn
                    variant="secondary"
                    loading={pushFeishuSingle.isPending}
                    onClick={() => void confirmAndPushFeishu(drawer.user!)}
                  >
                    <Send className="h-4 w-4" />
                    发送飞书订阅
                  </Btn>
                )}
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
                  <RefreshCw className="h-4 w-4" />
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
