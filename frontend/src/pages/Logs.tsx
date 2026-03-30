import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Clock3, RefreshCcw, Trash2 } from 'lucide-react'
import { logApi } from '@/lib/api'
import type { SyncLog } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Btn } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'
import { useConfirm } from '@/components/ui/ConfirmProvider'
import { pushToast } from '@/lib/notify'

const DEFAULT_PAGE_SIZE = 20
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]
const CLEANUP_PRESETS = [1, 3, 7, 30, 90, 180]

export default function Logs() {
  const confirm = useConfirm()
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)
  const [cleanupMode, setCleanupMode] = useState<string>('7')
  const [cleanupDays, setCleanupDays] = useState('7')

  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['logs', page, pageSize],
    queryFn: () => logApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
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
      void refetch()
    },
  })

  const effectiveCleanupDays = cleanupMode === 'custom' ? Number(cleanupDays) : Number(cleanupMode)

  const handleCleanup = async () => {
    if (!Number.isFinite(effectiveCleanupDays) || effectiveCleanupDays < 1 || effectiveCleanupDays > 3650) {
      pushToast({
        title: '清理天数无效',
        description: '请输入 1 到 3650 之间的天数。',
        variant: 'warning',
      })
      return
    }

    const ok = await confirm({
      title: `清理 ${effectiveCleanupDays} 天前的日志？`,
      description: '该操作只删除历史操作日志，不影响当前节点、用户和协议配置。',
      confirmText: '确认清理',
      cancelText: '取消',
      tone: 'danger',
    })
    if (!ok) return
    cleanup.mutate(effectiveCleanupDays)
  }

  const columns = [
    {
      key: 'created_at',
      label: '时间',
      render: (l: SyncLog) => <span className="font-mono text-xs tabular-nums text-soft">{new Date(l.created_at).toLocaleString('zh-CN')}</span>,
    },
    { key: 'action', label: '操作', render: (l: SyncLog) => <span className="font-mono text-xs">{l.action}</span> },
    { key: 'target', label: '目标', render: (l: SyncLog) => <span className="text-xs text-soft">{l.target}</span> },
    { key: 'success', label: '结果', render: (l: SyncLog) => <Badge label={l.success ? '成功' : classifyFailureReason(l.message)} variant={l.success ? 'green' : 'red'} /> },
    { key: 'duration_ms', label: '耗时', render: (l: SyncLog) => <span className="text-xs text-soft">{l.duration_ms ? `${l.duration_ms}ms` : '—'}</span> },
    {
      key: 'message',
      label: '消息',
      render: (l: SyncLog) => (
        <div className="max-w-xl space-y-1 text-xs">
          {!l.success && <span className="font-medium text-[var(--text)]">{classifyFailureReason(l.message)}</span>}
          <span className="block whitespace-normal break-all text-soft">{l.message || '—'}</span>
        </div>
      ),
    },
  ]

  return (
    <PageShell>
      <PageHeader
        title="操作日志"
        description="集中查看同步、健康检测和系统动作。日志页保持偏冷静的信息密度，方便快速定位失败动作和耗时异常。"
        actions={
          <Btn variant="secondary" loading={isFetching} onClick={() => void refetch()}>
            <RefreshCcw className="h-4 w-4" />
            立即刷新
          </Btn>
        }
      />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <SurfaceCard className="p-4">
          <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
          <div className="mt-4 flex flex-col gap-3 border-t border-[var(--border)] pt-4 md:flex-row md:items-center md:justify-between">
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
            <Pagination page={page} pageSize={pageSize} total={data?.total ?? 0} onChange={setPage} />
          </div>
        </SurfaceCard>

        <SurfaceCard className="p-5">
          <div className="space-y-3">
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <Activity className="h-4 w-4 text-[var(--accent)]" />
                观察重点
              </div>
              <p className="mt-2 text-sm leading-6 text-soft">优先看失败记录、异常耗时，以及重复出现的节点或动作名，这些通常能最快暴露部署问题。</p>
            </div>
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <Clock3 className="h-4 w-4 text-[var(--accent)]" />
                自动刷新
              </div>
              <p className="mt-2 text-sm leading-6 text-soft">页面默认每 10 秒拉取一次最新日志，适合在同步节点、测试 SSH 或排查订阅问题时并排观察。</p>
              <div className="mt-3 inline-flex items-center rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 py-1 text-xs font-medium text-soft">
                刷新周期：10s
              </div>
            </div>
            <div className="rounded-2xl border border-[var(--border)] bg-[var(--panel-muted)] p-4">
              <div className="flex items-center gap-2 text-sm font-semibold">
                <Trash2 className="h-4 w-4 text-[var(--accent)]" />
                清理日志
              </div>
              <p className="mt-2 text-sm leading-6 text-soft">建议保留最近 7 到 30 天日志用于排障。更久的历史记录可以定期清理，避免日志表持续增长。</p>
              <div className="mt-3 space-y-3">
                <select
                  value={cleanupMode}
                  onChange={(e) => {
                    setCleanupMode(e.target.value)
                    if (e.target.value !== 'custom') setCleanupDays(e.target.value)
                  }}
                  className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"
                >
                  {CLEANUP_PRESETS.map((days) => (
                    <option key={days} value={String(days)}>
                      清理 {days} 天前日志
                    </option>
                  ))}
                  <option value="custom">自定义天数</option>
                </select>
                {cleanupMode === 'custom' && (
                  <input
                    type="number"
                    min={1}
                    max={3650}
                    value={cleanupDays}
                    onChange={(e) => setCleanupDays(e.target.value)}
                    placeholder="输入 1 - 3650"
                    className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] outline-none focus:border-[var(--accent)]"
                  />
                )}
                <Btn variant="secondary" loading={cleanup.isPending} onClick={() => void handleCleanup()}>
                  <Trash2 className="h-4 w-4" />
                  清理历史日志
                </Btn>
              </div>
            </div>
          </div>
        </SurfaceCard>
      </div>
    </PageShell>
  )
}

function classifyFailureReason(message: string) {
  const normalized = message.toLowerCase()
  if (normalized.includes('ssh')) return 'SSH 失败'
  if (normalized.includes('timeout') || normalized.includes('超时')) return '超时'
  if (normalized.includes('配置') || normalized.includes('private_key') || normalized.includes('协议') || normalized.includes('解析')) return '配置错误'
  if (normalized.includes('漂移')) return '配置漂移'
  return '失败'
}
