import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCcw, Trash2 } from 'lucide-react'
import { logApi } from '@/lib/api'
import type { SyncLog } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { FilterChip, ListToolbar } from '@/components/ui/ListToolbar'
import { Badge } from '@/components/ui/Badge'
import { Btn, Field } from '@/components/ui/Form'
import { Modal } from '@/components/ui/Modal'
import { PageShell } from '@/components/ui/Page'
import { pushToast } from '@/lib/notify'

const DEFAULT_PAGE_SIZE = 20
const CLEANUP_PRESETS = [1, 3, 7, 30, 90, 180]
// actor 前缀筛选：与后端 entity.SyncLog 的 actor 格式约定一致
const ACTOR_FILTERS: Array<{ value: string; label: string }> = [
  { value: '', label: '全部来源' },
  { value: 'admin:', label: '管理员操作' },
  { value: 'system:scheduler:', label: '后台调度' },
  { value: 'system:install-token:', label: '节点接入' },
  { value: 'system:feishu-webhook', label: '飞书回调' },
]

export default function Logs() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const [cleanupMode, setCleanupMode] = useState<string>('7')
  const [cleanupDays, setCleanupDays] = useState('7')
  const [actorFilter, setActorFilter] = useState('')
  const [search, setSearch] = useState('')
  const [keyword, setKeyword] = useState('')

  useEffect(() => {
    const next = search.trim()
    if (keyword === next) return
    const timer = setTimeout(() => {
      setPage(1)
      setKeyword(next)
    }, 300)
    return () => clearTimeout(timer)
  }, [keyword, search])

  const { data, isLoading, isError, refetch, isFetching } = useQuery({
    queryKey: ['logs', page, pageSize, actorFilter, keyword],
    queryFn: () => logApi.list({
      page,
      page_size: pageSize,
      actor: actorFilter || undefined,
      keyword: keyword || undefined,
    }).then((r) => r.data.data!),
    refetchInterval: 10_000,
  })

  const cleanup = useMutation({
    mutationFn: (days: number) => logApi.cleanup(days),
    onSuccess: (res, days) => {
      const summary = res.data.data
      pushToast({
        title: '日志清理完成',
        description: `已清理 ${days} 天前的 ${summary?.deleted ?? 0} 条日志。`,
        variant: 'success',
      })
      void qc.invalidateQueries({ queryKey: ['logs'] })
      setPage(1)
      setCleanupOpen(false)
    },
  })

  const effectiveCleanupDays = cleanupMode === 'custom' ? Number(cleanupDays) : Number(cleanupMode)
  const validDays = Number.isInteger(effectiveCleanupDays) && effectiveCleanupDays >= 1 && effectiveCleanupDays <= 3650

  const columns = [
    {
      key: 'created_at',
      label: '时间',
      render: (l: SyncLog) => <time dateTime={l.created_at} className="tabular-nums text-soft">{new Date(l.created_at).toLocaleString('zh-CN')}</time>,
    },
    { key: 'action', label: '操作', render: (l: SyncLog) => <span className="wrap-anywhere font-mono">{l.action}</span> },
    { key: 'actor', label: '来源', render: (l: SyncLog) => <ActorCell actor={l.actor} /> },
    { key: 'target', label: '目标', render: (l: SyncLog) => <span className="wrap-anywhere text-soft">{l.target || '—'}</span> },
    { key: 'success', label: '结果', render: (l: SyncLog) => <Badge label={l.success ? '成功' : classifyFailureReason(l.message)} variant={l.success ? 'green' : 'red'} /> },
    { key: 'duration_ms', label: '耗时', render: (l: SyncLog) => <span className="whitespace-nowrap tabular-nums text-soft">{l.duration_ms ? `${l.duration_ms} ms` : '—'}</span> },
    {
      key: 'message',
      label: '消息',
      render: (l: SyncLog) => <LogMessage message={l.message} />,
    },
  ]

  return (
    <PageShell>
      <ListToolbar
        searchValue={search}
        searchPlaceholder="搜索操作、来源、目标或消息"
        onSearchChange={value => {
          setSearch(value)
          if (!value.trim()) {
            setKeyword('')
            setPage(1)
          }
        }}
        filters={ACTOR_FILTERS.map(filter => (
          <FilterChip
            key={filter.value}
            active={actorFilter === filter.value}
            onClick={() => {
              setActorFilter(filter.value)
              setPage(1)
            }}
          >
            {filter.label}
          </FilterChip>
        ))}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Btn variant="secondary" loading={isFetching} title="每 10 秒自动刷新" onClick={() => void refetch()}>
              <RefreshCcw className="size-4" />刷新
            </Btn>
            <Btn variant="secondary" onClick={() => setCleanupOpen(true)}>
              <Trash2 className="size-4" />清理历史日志
            </Btn>
          </div>
        }
      />
      {isError && <p role="alert" className="text-sm text-[var(--danger)]">日志加载失败，请刷新重试。</p>}
      {(data || !isError) && (
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} labelClassName="text-sm text-soft" />
      )}
      <Pagination page={page} pageSize={pageSize} total={data?.total} onChange={setPage} onPageSizeChange={setPageSize} />

      <Modal
        open={cleanupOpen}
        onClose={() => { if (!cleanup.isPending) setCleanupOpen(false) }}
        size="sm"
        title="清理历史日志"
        footer={
          <>
            <Btn variant="secondary" disabled={cleanup.isPending} onClick={() => setCleanupOpen(false)}>取消</Btn>
            <Btn variant="danger" loading={cleanup.isPending} disabled={!validDays} onClick={() => cleanup.mutate(effectiveCleanupDays)}>
              确认清理
            </Btn>
          </>
        }
      >
        <div className="space-y-4">
          <label className="block space-y-2 text-sm font-medium">
            <span>保留最近</span>
            <select
              value={cleanupMode}
              disabled={cleanup.isPending}
              onChange={e => {
                setCleanupMode(e.target.value)
                if (e.target.value !== 'custom') setCleanupDays(e.target.value)
              }}
              className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] focus:border-[var(--accent)]"
            >
              {CLEANUP_PRESETS.map(days => <option key={days} value={days}>{days} 天</option>)}
              <option value="custom">自定义天数</option>
            </select>
          </label>
          {cleanupMode === 'custom' && (
            <Field
              label="保留天数"
              type="number"
              min={1}
              max={3650}
              step={1}
              value={cleanupDays}
              disabled={cleanup.isPending}
              onChange={e => setCleanupDays(e.target.value)}
              error={validDays ? undefined : '请输入 1 到 3650 之间的整数。'}
            />
          )}
          <p className="text-sm leading-6 text-[var(--danger)]">
            {validDays ? `${effectiveCleanupDays} 天前` : '超出保留期限'}的操作日志将永久删除，范围包含所有来源和搜索结果以外的记录。
          </p>
        </div>
      </Modal>
    </PageShell>
  )
}

function LogMessage({ message }: { message: string }) {
  const text = message?.trim()
  if (!text) return <span className="text-faint">—</span>
  const firstLine = text.split(/\r?\n/, 1)[0]
  if (text.length <= 88 && firstLine === text) {
    return <span className="block max-w-xl whitespace-normal wrap-anywhere text-soft">{text}</span>
  }
  return (
    <details className="group max-w-xl whitespace-normal text-sm leading-6">
      <summary className="cursor-pointer wrap-anywhere rounded-sm focus-visible:outline-[var(--accent)]">
        <span className="group-open:hidden">{firstLine.slice(0, 88)}…</span>
        <span className="hidden group-open:inline">收起完整消息</span>
      </summary>
      <p className="mt-1 whitespace-pre-wrap wrap-anywhere text-left text-soft">{message}</p>
    </details>
  )
}

// ActorCell 把 actor 字符串渲染为「类型徽章 + 主体」；空值为 v0.5.0 之前的历史记录
function ActorCell({ actor }: { actor: string }) {
  if (!actor) return <span className="text-sm text-faint">—</span>
  if (actor.startsWith('admin:')) {
    return (
      <span title={actor} className="inline-flex min-w-0 flex-wrap items-center gap-1.5 text-sm">
        <Badge label="管理员" variant="blue" />
        <span className="wrap-anywhere font-mono">{actor.slice('admin:'.length)}</span>
      </span>
    )
  }
  if (actor.startsWith('system:scheduler:')) {
    return (
      <span title={actor} className="inline-flex min-w-0 flex-wrap items-center gap-1.5 text-sm">
        <Badge label="调度" variant="gray" />
        <span className="wrap-anywhere font-mono text-soft">{actor.slice('system:scheduler:'.length)}</span>
      </span>
    )
  }
  if (actor.startsWith('system:install-token:')) {
    return (
      <span title={actor} className="inline-flex min-w-0 flex-wrap items-center gap-1.5 text-sm">
        <Badge label="接入" variant="gray" />
        <span className="wrap-anywhere font-mono text-soft">{actor.slice('system:install-token:'.length)}</span>
      </span>
    )
  }
  if (actor === 'system:feishu-webhook') {
    return <Badge label="飞书" variant="gray" />
  }
  return <span className="wrap-anywhere font-mono text-sm text-soft">{actor}</span>
}

function classifyFailureReason(message: string) {
  const normalized = message.toLowerCase()
  if (normalized.includes('drift_conflict') || normalized.includes('漂移')) return '配置漂移'
  if (normalized.includes('ssh')) return 'SSH 失败'
  if (normalized.includes('timeout') || normalized.includes('超时')) return '超时'
  if (normalized.includes('配置') || normalized.includes('private_key') || normalized.includes('协议') || normalized.includes('解析')) return '配置错误'
  return '失败'
}
