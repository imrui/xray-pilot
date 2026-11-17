import { useMemo } from 'react'
import { useIsFetching, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
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
    <div className="flex min-w-0 items-center justify-between gap-2 rounded-lg border border-[var(--border)] px-3 py-4">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-3">
          <div role="img" aria-label={!node.last_check_at ? '尚未检查' : ok ? '健康' : '异常'} className={`h-2 w-2 shrink-0 rounded-full ${!node.last_check_at ? 'bg-[var(--border-strong)]' : ok ? 'bg-[var(--success)]' : 'bg-[var(--danger)]'}`} />
          <span className="truncate text-sm font-medium" title={`${node.name} · ${node.xray_version ? `Xray ${node.xray_version}` : 'Xray 版本未知'}`}>{node.name}</span>
        </div>
        <div className="mt-1 pl-5 text-xs text-soft">{node.online_user_count} 人在线</div>
      </div>
      <div className="flex shrink-0 flex-col items-end gap-2">
        <span className="font-mono text-xs text-soft">{ok && node.last_latency_ms ? `${node.last_latency_ms}ms` : '—'}</span>
        {syncBadge(node)}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const queryClient = useQueryClient()
  const syncFetching = useIsFetching({ queryKey: ['sync-summary'], exact: true }) > 0
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
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  })
  const logsQuery = useQuery({
    queryKey: ['dashboard-logs'],
    queryFn: () => logApi.list({ page: 1, page_size: 6, important: true }).then((r) => r.data.data!),
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  })

  const nodes = useMemo(() => nodesQuery.data?.list ?? [], [nodesQuery.data])
  const logs = logsQuery.data?.list ?? []
  const sortedNodes = useMemo(
    () => [...nodes].sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true, sensitivity: 'base' })),
    [nodes],
  )
  function refreshOverview() {
    void nodesQuery.refetch()
    void logsQuery.refetch()
    void queryClient.refetchQueries({ queryKey: ['sync-summary'], exact: true })
  }

  const recentActivity = (
        <SurfaceCard className="flex min-w-0 flex-col">
          <div className="flex h-14 shrink-0 items-center justify-between border-b border-[var(--border)] px-5">
            <h3 className="font-semibold">最近操作与异常</h3>
            <Link to="/logs" className="text-sm text-[var(--accent)] transition hover:opacity-80">查看全部</Link>
          </div>
          <div className="flex-1">
            {logsQuery.isError ? (
              <p role="alert" className="px-5 py-10 text-sm text-[var(--danger)]">操作记录加载失败，请点击顶部刷新重试。</p>
            ) : logs.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">{logsQuery.isFetching ? '正在加载…' : '暂无日志记录'}</div>
            ) : (
              logs.map((log) => (
                <div key={log.id} className="flex min-h-20 items-start gap-3 border-b border-[var(--border)] px-5 py-4 last:border-b-0">
                  <div className="min-w-0 flex flex-1 items-start gap-3">
                    {logIcon(log)}
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-1 text-sm"><span className="break-all font-medium">{log.action}</span><time className="text-xs text-faint" dateTime={log.created_at} title={new Date(log.created_at).toLocaleString('zh-CN')}>{new Date(log.created_at).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}</time></div>
                      {log.target && <p className="mt-1 break-all text-xs text-soft">{log.target}</p>}
                      <div className="mt-1 break-words text-sm leading-relaxed text-soft [overflow-wrap:anywhere]">{log.message || '系统事件'}</div>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </SurfaceCard>
  )
  const nodeHealth = (
        <SurfaceCard className="flex min-w-0 flex-col">
          <div className="flex h-14 shrink-0 items-center justify-between border-b border-[var(--border)] px-5">
            <h3 className="font-semibold">节点健康</h3>
            <Btn variant="ghost" className="h-8 px-2" onClick={() => void nodesQuery.refetch()} disabled={nodesQuery.isFetching}>
              <RefreshCcw className={`h-4 w-4 ${nodesQuery.isFetching ? 'animate-spin' : ''}`} />
              刷新
            </Btn>
          </div>
          <div className="flex-1">
            {nodesQuery.isError ? (
              <p role="alert" className="px-5 py-10 text-sm text-[var(--danger)]">节点健康加载失败，请刷新重试。</p>
            ) : sortedNodes.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">{nodesQuery.isFetching ? '正在加载…' : '暂无可用节点数据'}</div>
            ) : (
              <div className="grid grid-cols-1 gap-3 p-4">{sortedNodes.map((node) => <NodeHealthRow key={node.id} node={node} />)}</div>
            )}
          </div>
        </SurfaceCard>
  )
  return (
    <PageShell>
      <TrafficAnalyticsPanel onRefresh={refreshOverview} refreshing={nodesQuery.isFetching || logsQuery.isFetching || syncFetching} recentActivity={recentActivity} nodeHealth={nodeHealth} />
    </PageShell>
  )
}
