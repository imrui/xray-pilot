import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { logApi } from '@/lib/api'
import type { SyncLog } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'
import { PageHeader, PageShell, SurfaceCard } from '@/components/ui/Page'

const PAGE_SIZE = 50

export default function Logs() {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['logs', page],
    queryFn: () => logApi.list({ page, page_size: PAGE_SIZE }).then((r) => r.data.data!),
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
        description="实时查看节点同步、健康检测和系统动作，默认每 10 秒自动刷新。"
        stats={[
          { label: '刷新周期', value: '10s' },
          { label: '分页尺寸', value: PAGE_SIZE },
          { label: '当前页记录', value: data?.list?.length ?? 0 },
        ]}
      />

      <SurfaceCard className="p-4">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </SurfaceCard>
    </PageShell>
  )
}
