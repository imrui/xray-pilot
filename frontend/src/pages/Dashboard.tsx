import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, Link2, RefreshCcw, Server, Users } from 'lucide-react'
import { logApi, nodeApi, userApi } from '@/lib/api'
import type { Node, SyncLog } from '@/types'
import { Btn } from '@/components/ui/Form'
import { Badge } from '@/components/ui/Badge'
import { PageShell, SurfaceCard } from '@/components/ui/Page'

function StatCard({
  title,
  value,
  change,
  changeTone = 'positive',
  icon: Icon,
}: {
  title: string
  value: string | number
  change?: string
  changeTone?: 'positive' | 'negative' | 'neutral'
  icon: typeof Server
}) {
  return (
    <SurfaceCard className="p-5">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm text-soft">{title}</div>
          <div className="mt-3 text-4xl font-semibold tracking-[-0.05em]">{value}</div>
          {change && (
            <div
              className={`mt-3 text-sm ${
                changeTone === 'positive' ? 'text-emerald-400' : changeTone === 'negative' ? 'text-rose-400' : 'text-soft'
              }`}
            >
              {change}
            </div>
          )}
        </div>
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]">
          <Icon className="h-5 w-5" />
        </div>
      </div>
    </SurfaceCard>
  )
}

function logIcon(log: SyncLog) {
  if (log.success) return <div className="flex h-9 w-9 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-400"><Activity className="h-4 w-4" /></div>
  return <div className="flex h-9 w-9 items-center justify-center rounded-full bg-rose-500/10 text-rose-400"><AlertTriangle className="h-4 w-4" /></div>
}

function NodeHealthRow({ node }: { node: Node }) {
  const ok = node.last_check_ok
  return (
    <div className="flex items-center justify-between gap-3 border-b border-[var(--border)] px-5 py-4 last:border-b-0">
      <div className="flex items-center gap-3">
        <div className={`h-2.5 w-2.5 rounded-full ${ok ? 'bg-emerald-400' : 'bg-rose-400'}`} />
        <span className="text-sm font-medium">{node.name}</span>
      </div>
      <div className="flex items-center gap-2">
        {ok ? (
          <span className="font-mono text-xs text-soft">{node.last_latency_ms ? `${node.last_latency_ms}ms` : '—'}</span>
        ) : (
          <Badge label="异常" variant="red" />
        )}
      </div>
    </div>
  )
}

export default function Dashboard() {
  const usersQuery = useQuery({
    queryKey: ['dashboard-users'],
    queryFn: () => userApi.list({ page: 1, page_size: 200 }).then((r) => r.data.data!),
  })

  const nodesQuery = useQuery({
    queryKey: ['dashboard-nodes'],
    queryFn: () => nodeApi.list({ page: 1, page_size: 200 }).then((r) => r.data.data!),
  })

  const logsQuery = useQuery({
    queryKey: ['dashboard-logs'],
    queryFn: () => logApi.list({ page: 1, page_size: 5 }).then((r) => r.data.data!),
  })

  const users = usersQuery.data?.list ?? []
  const nodes = nodesQuery.data?.list ?? []
  const logs = logsQuery.data?.list ?? []

  const activeUsers = useMemo(() => users.filter((u) => u.active).length, [users])
  const healthyNodes = useMemo(() => nodes.filter((n) => n.last_check_ok).length, [nodes])
  const nodeLinks = useMemo(() => users.filter((u) => !!u.subscribe_url).length, [users])
  const unhealthyNodes = useMemo(() => nodes.filter((n) => n.last_check_at && !n.last_check_ok), [nodes])

  return (
    <PageShell className="space-y-6">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <StatCard title="节点总数" value={nodes.length} change={nodes.length ? `${healthyNodes}/${nodes.length} 健康` : '暂无节点'} icon={Server} />
        <StatCard title="活跃用户" value={activeUsers} change={users.length ? `${users.length} 用户总数` : '暂无用户'} icon={Users} />
        <StatCard title="订阅链接" value={nodeLinks} change={nodeLinks ? '已生成订阅分发' : '暂无订阅'} icon={Link2} />
        <StatCard
          title="健康节点"
          value={nodes.length ? `${healthyNodes}/${nodes.length}` : 0}
          change={unhealthyNodes.length ? `${unhealthyNodes.length} 个节点异常` : '所有已检测节点正常'}
          changeTone={unhealthyNodes.length ? 'negative' : 'positive'}
          icon={Activity}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_520px]">
        <SurfaceCard className="overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4">
            <h3 className="text-lg font-semibold">最近操作</h3>
            <a href="/logs" className="text-sm text-[var(--accent)] transition hover:opacity-80">查看全部</a>
          </div>
          <div>
            {logs.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">暂无日志记录</div>
            ) : (
              logs.map((log) => (
                <div key={log.id} className="flex items-center justify-between gap-4 border-b border-[var(--border)] px-5 py-4 last:border-b-0">
                  <div className="flex items-center gap-3">
                    {logIcon(log)}
                    <div>
                      <div className="text-sm font-semibold">{log.action}</div>
                      <div className="mt-1 text-sm text-soft">{log.target || log.message || '系统事件'}</div>
                    </div>
                  </div>
                  <div className="text-xs text-faint">{new Date(log.created_at).toLocaleString('zh-CN')}</div>
                </div>
              ))
            )}
          </div>
        </SurfaceCard>

        <SurfaceCard className="overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-4">
            <h3 className="text-lg font-semibold">节点健康</h3>
            <Btn variant="ghost" onClick={() => void nodesQuery.refetch()}>
              <RefreshCcw className="h-4 w-4" />
              刷新
            </Btn>
          </div>
          <div>
            {nodes.length === 0 ? (
              <div className="px-5 py-10 text-sm text-soft">暂无节点数据</div>
            ) : (
              nodes.slice(0, 6).map((node) => <NodeHealthRow key={node.id} node={node} />)
            )}
          </div>
        </SurfaceCard>
      </div>
    </PageShell>
  )
}
