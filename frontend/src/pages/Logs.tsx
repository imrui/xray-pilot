import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { logApi } from '@/lib/api'
import type { SyncLog } from '@/types'
import { Table, Pagination } from '@/components/ui/Table'
import { Badge } from '@/components/ui/Badge'

const PAGE_SIZE = 50

export default function Logs() {
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: ['logs', page],
    queryFn: () => logApi.list({ page, page_size: PAGE_SIZE }).then(r => r.data.data!),
    refetchInterval: 10_000,
  })

  const columns = [
    {
      key: 'created_at', label: '时间',
      render: (l: SyncLog) => (
        <span className="text-xs text-slate-500 tabular-nums">
          {new Date(l.created_at).toLocaleString('zh-CN')}
        </span>
      ),
    },
    { key: 'action', label: '操作', render: (l: SyncLog) => <span className="font-mono text-xs text-slate-700">{l.action}</span> },
    { key: 'target', label: '目标', render: (l: SyncLog) => <span className="text-xs text-slate-600">{l.target}</span> },
    {
      key: 'success', label: '结果',
      render: (l: SyncLog) => <Badge label={l.success ? '成功' : '失败'} variant={l.success ? 'green' : 'red'} />,
    },
    { key: 'duration_ms', label: '耗时', render: (l: SyncLog) => l.duration_ms ? `${l.duration_ms}ms` : '—' },
    {
      key: 'message', label: '消息',
      render: (l: SyncLog) => (
        <span className="text-xs text-slate-600 max-w-xs truncate block" title={l.message}>
          {l.message || '—'}
        </span>
      ),
    },
  ]

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">操作日志</h1>
        <p className="text-sm text-slate-500 mt-0.5">节点同步与健康检测记录 · 每 10s 自动刷新</p>
      </div>

      <div className="bg-white rounded-xl shadow-sm border border-slate-200 overflow-hidden">
        <Table columns={columns} data={data?.list ?? []} loading={isLoading} />
        <Pagination page={page} pageSize={PAGE_SIZE} total={data?.total ?? 0} onChange={setPage} />
      </div>
    </div>
  )
}
