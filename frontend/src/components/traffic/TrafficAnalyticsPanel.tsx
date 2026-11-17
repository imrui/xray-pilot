import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Activity, AlertTriangle, ArrowDown, ArrowUp, Database, Network, RefreshCcw, Server, Users, X } from 'lucide-react'
import { trafficApi } from '@/lib/api'
import { formatBytes } from '@/lib/utils'
import { presetRange, TRAFFIC_TIMEZONE, type DateSelection } from '@/lib/trafficDates'
import type { TrafficDimension, TrafficDirection, TrafficOption } from '@/types/traffic'
import { SurfaceCard } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'
import { DateRangePicker } from './DateRangePicker'
import { TrafficChart } from './TrafficChart'
import { TrafficRankingChart } from './TrafficRankingChart'

const control = 'h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm'

function ObjectFilter({ label, options, value, onChange }: { label: string; options: TrafficOption[]; value: number | undefined; onChange: (id: number | undefined) => void }) {
  const [search, setSearch] = useState('')
  const shown = options.filter(o => o.id === value || o.name.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="flex w-full min-w-0 items-center gap-2 sm:w-60">
      <span className="shrink-0 text-sm text-soft">{label}</span>
      <div className="flex min-w-0 flex-1 overflow-hidden rounded-md border border-[var(--border)] bg-[var(--panel-strong)] focus-within:border-[var(--accent)]">
        <input className="h-9 w-16 min-w-0 border-r border-[var(--border)] bg-transparent px-2 text-sm outline-none focus-visible:bg-[var(--panel-muted)]" type="search" aria-label={`搜索${label}`} placeholder="搜索" value={search} onChange={e => setSearch(e.target.value)} />
        <select className="h-9 min-w-0 flex-1 bg-transparent px-2 text-sm focus-visible:outline-[var(--accent)]" aria-label={`筛选${label}`} value={value ?? ''} onChange={e => onChange(e.target.value ? Number(e.target.value) : undefined)}>
          <option value="">全部{label}</option>
          {value && !options.some(o => o.id === value) && <option value={value}>已删除{label} #{value}</option>}
          {shown.map(o => <option key={o.id} value={o.id}>{o.name}</option>)}
        </select>
      </div>
    </div>
  )
}

export function TrafficAnalyticsPanel({ onRefresh, refreshing = false, recentActivity, nodeHealth }: { onRefresh: () => void; refreshing?: boolean; recentActivity?: ReactNode; nodeHealth?: ReactNode }) {
  // Keep the selected range fixed until the administrator changes it or refreshes.
  const [range, setRange] = useState<DateSelection>(() => presetRange('7d'))
  const [dimension, setDimension] = useState<TrafficDimension>('node')
  const [direction, setDirection] = useState<TrafficDirection>('total')
  const [nodeId, setNodeId] = useState<number>()
  const [userId, setUserId] = useState<number>()
  const query = useQuery({
    queryKey: ['traffic-analytics', range.start, range.end, dimension, nodeId, userId],
    queryFn: ({ signal }) => trafficApi.analytics({ start: range.start, end: range.end, dimension, node_id: nodeId, user_id: userId }, signal).then(r => r.data.data!),
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  })
  const data = query.data
  const isRefreshing = refreshing || query.isFetching
  const total = data ? data.up_bytes + data.down_bytes : undefined
  const ranking = data?.ranking ?? []

  function changeRange(value: DateSelection) {
    setRange(value)
  }

  function refresh() {
    const nextRange = range.preset === 'custom' ? range : presetRange(range.preset)
    if (nextRange.start === range.start && nextRange.end === range.end) {
      void query.refetch()
    } else {
      setRange(nextRange)
    }
    onRefresh()
  }

  return (
    <div className="space-y-4">
      <SurfaceCard>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-3 p-4" aria-label="统计筛选">
          <DateRangePicker value={range} onChange={changeRange} />
          <div className="flex w-full min-w-0 flex-wrap items-center gap-3 sm:w-auto">
            <ObjectFilter label="节点" value={nodeId} options={data?.nodes ?? []} onChange={setNodeId} />
            <ObjectFilter label="用户" value={userId} options={data?.users ?? []} onChange={setUserId} />
            {(nodeId || userId) && <button className="inline-flex items-center gap-1 rounded py-1 text-sm text-[var(--accent)]" onClick={() => { setNodeId(undefined); setUserId(undefined) }}><X className="h-4 w-4" />清除筛选</button>}
          </div>
          <Btn variant="secondary" className="ml-auto h-9 shrink-0" onClick={refresh} disabled={isRefreshing}>
            <RefreshCcw className={`h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`} />
            {isRefreshing ? '刷新中…' : '刷新'}
          </Btn>
        </div>
      </SurfaceCard>

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4 min-[1800px]:grid-cols-8" aria-label="运行与区间统计" aria-busy={query.isFetching}>
        {[
          { title: '节点总数', value: data?.total_nodes ?? '—', hint: '当前全部节点', icon: Server },
          { title: '健康节点', value: data ? `${data.healthy_nodes} / ${data.enabled_nodes}` : '—', hint: '当前健康 / 启用节点', icon: Activity, color: data && data.healthy_nodes < data.enabled_nodes ? 'var(--warning)' : 'var(--success)' },
          { title: '启用用户', value: data ? `${data.enabled_users} / ${data.total_users}` : '—', hint: '当前启用 / 用户总数', icon: Users },
          { title: '区间总用量', value: formatBytes(total), hint: '所选区间 · 上行 + 下行', icon: Database },
          { title: '区间上行', value: formatBytes(data?.up_bytes), hint: '所选区间 · 用户上传', icon: ArrowUp },
          { title: '区间下行', value: formatBytes(data?.down_bytes), hint: '所选区间 · 用户下载', icon: ArrowDown, color: 'var(--success)' },
          { title: '区间活跃用户', value: data?.active_users ?? '—', hint: '所选区间 · 有流量的用户', icon: Users },
          { title: '有用量节点', value: data?.active_nodes ?? '—', hint: '所选区间 · 含已删除节点', icon: Network },
        ].map(({ title, value, hint, icon: Icon, color }) => <SurfaceCard key={title} className="min-w-0 p-4">
          <div className="flex items-center justify-between gap-2"><h3 className="text-sm text-soft">{title}</h3><Icon aria-hidden="true" className="h-4 w-4 shrink-0" style={{ color: color ?? 'var(--accent)' }} /></div>
          <p className="mt-3 break-words text-xl font-semibold tracking-tight tabular-nums sm:text-2xl">{value}</p>
          <p className="mt-2 text-xs leading-relaxed text-faint">{hint}</p>
        </SurfaceCard>)}
      </div>

      <div className="grid items-start gap-4 min-[1600px]:grid-cols-[minmax(0,7fr)_minmax(0,3fr)]">
        <div className="min-w-0 space-y-4">
      {query.isError ? (
        <SurfaceCard className="p-5">
          <p role="alert" className="mb-3 text-sm text-[var(--danger)]">统计加载失败：{query.error.message}</p>
          <Btn variant="secondary" onClick={() => void query.refetch()} disabled={query.isFetching}>重试</Btn>
        </SurfaceCard>
      ) : (
        <>
          {data?.truncated && <p role="status" className="flex gap-2 text-sm text-[var(--warning)]"><AlertTriangle className="h-4 w-4 shrink-0" />所选范围超出明细保留期限，结果不完整。当前保留起点：{new Date(data.retention_start!).toLocaleString('zh-CN', { timeZone: TRAFFIC_TIMEZONE })}。</p>}

          <div className="space-y-4">
            <SurfaceCard className="min-w-0 p-5">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <h3 className="font-semibold">用量趋势</h3>
                <div className="flex gap-2">
                  <select className={control} aria-label="统计维度" value={dimension} onChange={e => setDimension(e.target.value as TrafficDimension)}><option value="all">总览</option><option value="node">按节点</option><option value="user">按用户</option></select>
                  <select className={control} aria-label="流量方向" value={direction} onChange={e => setDirection(e.target.value as TrafficDirection)}><option value="total">总用量</option><option value="up">上行</option><option value="down">下行</option></select>
                </div>
              </div>
              {data ? <TrafficChart key={`${dimension}-${nodeId}-${userId}`} series={data.series} direction={direction} interval={data.interval} /> : <div className="flex h-80 items-center justify-center text-sm text-soft">正在加载趋势…</div>}
              <details className="mt-4 border-t border-[var(--border)] pt-3 text-sm text-soft">
                <summary className="w-fit cursor-pointer rounded hover:text-[var(--text)]">统计说明{data && data.failed_polls > 0 && <span className="ml-2 text-[var(--warning)]">{data.failed_polls} 次采集失败</span>}</summary>
                <div className="mt-3 space-y-2 leading-relaxed">
                  <p>按{data?.interval === 'hour' ? '小时' : '天'}汇总，展示总用量前 5 项及其他。点击图例可隐藏曲线。</p>
                  <p>北京时间（UTC+8），用量按采集时间计入区间。上行和下行以用户视角统计，活跃用户与节点指区间内实际产生流量的对象。</p>
                  <p>缺少成功采集依据的时段保留断点，历史缺采无法补算；缺采期间的用量可能计入后续采集周期，统计不代表实时速率。</p>
                  {data && <p>区间采集：成功 {data.successful_polls} 次，失败 {data.failed_polls} 次（按节点计）。</p>}
                  {data?.last_poll_at && <p>最近成功采集：{new Date(data.last_poll_at).toLocaleString('zh-CN', { timeZone: TRAFFIC_TIMEZONE })}</p>}
                </div>
              </details>
            </SurfaceCard>

            <SurfaceCard className="min-w-0">
              <div className="flex min-h-18 items-center justify-between gap-3 border-b border-[var(--border)] px-5 py-4">
                <h3 className="font-semibold">{dimension === 'user' ? '用户用量排名' : dimension === 'node' ? '节点用量排名' : '用量汇总'}</h3>
                <span className="shrink-0 text-sm text-soft">{ranking.length} 项</span>
              </div>
              <div className="p-4 sm:p-5">
                {data ? <TrafficRankingChart key={`${range.start}-${range.end}-${dimension}-${nodeId}-${userId}-${direction}`} rows={ranking} dimension={dimension} direction={direction} onSelect={id => { if (dimension === 'node') { setNodeId(id); setDimension('user') } else { setUserId(id); setDimension('node') } }} /> : <p className="flex h-80 items-center justify-center text-sm text-soft">正在加载排名…</p>}
              </div>
            </SurfaceCard>
          </div>
        </>
      )}
          {recentActivity}
        </div>
        <aside className="min-w-0" aria-label="当前节点健康">{nodeHealth}</aside>
      </div>
    </div>
  )
}
