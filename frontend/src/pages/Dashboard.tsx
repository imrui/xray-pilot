import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, RefreshCcw } from 'lucide-react'
import { logApi, nodeApi } from '@/lib/api'
import type { Node, SyncLog } from '@/types'
import { Btn } from '@/components/ui/Form'
import { Badge } from '@/components/ui/Badge'
import { PageShell, SurfaceCard } from '@/components/ui/Page'
import { TrafficAnalyticsPanel } from '@/components/traffic/TrafficAnalyticsPanel'

function logIcon(log: SyncLog) {
  if (log.success) return <div className="flex h-9 w-9 items-center justify-center rounded-full bg-[var(--success-soft)] text-[var(--success)]"><Activity className="h-4 w-4" /></div>
  return <div className="flex h-9 w-9 items-center justify-center rounded-full bg-[var(--danger-soft)] text-[var(--danger)]"><AlertTriangle className="h-4 w-4" /></div>
}

function syncBadge(node: Node) {
  switch (node.sync_status) {
    case 'synced':
      return <Badge label="已同步" variant="green" />
    case 'failed':
      return <Badge label="同步失败" variant="red" />
    case 'drifted':
    case 'pending':
    default:
      return <Badge label="待同步" variant="yellow" />
  }
}

function NodeHealthRow({ node }: { node: Node }) {
  const ok = node.last_check_ok
  return (
    <div className="flex items-center justify-between gap-4 border-b border-[var(--border)] px-5 py-4 last:border-b-0">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-3">
          <div className={`h-2.5 w-2.5 rounded-full ${!node.last_check_at ? 'bg-[var(--border-strong)]' : ok ? 'bg-[var(--success)]' : 'bg-[var(--danger)]'}`} />
          <span className="text-sm font-medium">{node.name}</span>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-soft">
          <span>{node.online_user_count} 人</span>
          <span className="text-faint">|</span>
          <span className="inline-flex items-center gap-1.5">
            <span className={`h-2 w-2 rounded-full ${node.xray_version ? 'bg-[var(--success)]' : 'bg-[var(--border-strong)]'}`} />
            <span>{node.xray_version ? `Xray ${node.xray_version}` : 'Xray —'}</span>
          </span>
        </div>
      </div>
      <div className="flex min-w-[74px] flex-col items-end gap-2">
        <span className="font-mono text-xs text-soft">{ok && node.last_latency_ms ? `${node.last_latency_ms}ms` : '—'}</span>
        <div className="scale-[0.9] origin-right">
          {syncBadge(node)}
        </div>
      </div>
    </div>
  )
}

export default function Dashboard() {
  const nodesQuery = useQuery({
    queryKey: ['dashboard-nodes'],
    queryFn: async () => {
      const list: Node[] = []
      for (let page = 1; ; page++) {
        const data = (await nodeApi.list({ page, page_size: 200, health_dashboard: true })).data.data!
        list.push(...(data.list ?? []))
        if (!data.list?.length || list.length >= data.total) return { list }
      }
    },
  })
  const logsQuery = useQuery({
    queryKey: ['dashboard-logs'],
    queryFn: () => logApi.list({ page: 1, page_size: 6, important: true }).then((r) => r.data.data!),
  })

  const nodes = useMemo(() => nodesQuery.data?.list ?? [], [nodesQuery.data])
  const logs = logsQuery.data?.list ?? []
  const sortedNodes = useMemo(
    () => [...nodes].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' })),
    [nodes],
  )

  return (
    <PageShell className="space-y-5">
      <TrafficAnalyticsPanel />

      <div className="grid gap-4 xl:grid-cols-3">
        <SurfaceCard className="flex h-full max-h-[420px] flex-col overflow-hidden xl:col-span-2">
          <div className="flex h-14 items-center justify-between border-b border-[var(--border)] px-5">
            <h3 className="text-lg font-semibold">最近操作与异常</h3>
            <a href="/logs" className="text-sm text-[var(--accent)] transition hover:opacity-80">查看全部</a>
          </div>
          <div className="flex-1 overflow-auto">
            {logs.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">暂无日志记录</div>
            ) : (
              logs.map((log) => (
                <div key={log.id} className="flex items-start justify-between gap-4 border-b border-[var(--border)] px-5 py-4 last:border-b-0">
                  <div className="min-w-0 flex items-start gap-3">
                    {logIcon(log)}
                    <div className="min-w-0">
                      <div className="text-sm font-semibold">{log.action}</div>
                      <div className="mt-1 text-sm text-soft">{log.message || log.target || '系统事件'}</div>
                    </div>
                  </div>
                  <div className="shrink-0 text-right">
                    <div className="text-xs text-faint">{new Date(log.created_at).toLocaleString('zh-CN')}</div>
                    <div className="mt-1 text-xs text-faint">{log.target || '系统事件'}</div>
                  </div>
                </div>
              ))
            )}
          </div>
        </SurfaceCard>

        <SurfaceCard className="flex h-full max-h-[420px] flex-col overflow-hidden">
          <div className="flex h-14 items-center justify-between border-b border-[var(--border)] px-5">
            <h3 className="text-lg font-semibold">节点健康</h3>
            <Btn variant="ghost" onClick={() => void nodesQuery.refetch()}>
              <RefreshCcw className="h-4 w-4" />
              刷新
            </Btn>
          </div>
          <div className="flex-1 overflow-auto">
            {sortedNodes.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">暂无可用节点数据</div>
            ) : (
              sortedNodes.map((node) => <NodeHealthRow key={node.id} node={node} />)
            )}
          </div>
        </SurfaceCard>
      </div>
    </PageShell>
  )
}
