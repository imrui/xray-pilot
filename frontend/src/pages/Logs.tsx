import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, Clock3, RefreshCcw } from 'lucide-react'
import { logApi } from '@/lib/api'
import type { SyncLog } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { Btn } from '@/components/ui/Form'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

const DEFAULT_PAGE_SIZE = 20
const PAGE_SIZE_OPTIONS = [10, 20, 50, 100]

export default function Logs() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE)

  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ['logs', page, pageSize],
    queryFn: () => logApi.list({ page, page_size: pageSize }).then((r) => r.data.data!),
    refetchInterval: 10_000,
  })

  const columns = [
    {
      key: 'created_at',
      label: '时间',
      render: (l: SyncLog) => <span className="font-mono text-xs tabular-nums text-soft">{new Date(l.created_at).toLocaleString('zh-CN')}</span>,
    },
    { key: 'action', label: '操作', render: (l: SyncLog) => <span className="font-mono text-xs">{l.action}</span> },
    { key: 'target', label: '目标', render: (l: SyncLog) => <span className="text-xs text-soft">{l.target}</span> },
    { key: 'success', label: '结果', render: (l: SyncLog) => <Badge label={l.success ? '成功' : '失败'} variant={l.success ? 'green' : 'red'} /> },
    { key: 'duration_ms', label: '耗时', render: (l: SyncLog) => <span className="text-xs text-soft">{l.duration_ms ? `${l.duration_ms}ms` : '—'}</span> },
    {
      key: 'message',
      label: '消息',
      render: (l: SyncLog) => (
        <span className="block max-w-xs truncate text-xs text-soft" title={l.message}>
          {l.message || '—'}
        </span>
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
          </div>
        </SurfaceCard>
      </div>
    </PageShell>
  )
}
