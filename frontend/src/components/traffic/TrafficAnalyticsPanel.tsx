import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, ArrowDown, ArrowUp, RefreshCcw, X } from 'lucide-react'
import { trafficApi } from '@/lib/api'
import { formatBytes } from '@/lib/utils'
import { presetRange, TRAFFIC_TIMEZONE, type DateSelection } from '@/lib/trafficDates'
import type { TrafficDimension, TrafficDirection, TrafficOption } from '@/types/traffic'
import { SurfaceCard } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'
import { DateRangePicker } from './DateRangePicker'
import { TrafficChart } from './TrafficChart'

const PAGE_SIZE = 5
const control = 'h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm'

function ObjectFilter({ label, options, value, onChange }: { label: string; options: TrafficOption[]; value: number | undefined; onChange: (id: number | undefined) => void }) {
  const [search, setSearch] = useState('')
  const shown = options.filter(o => o.id === value || o.name.toLowerCase().includes(search.toLowerCase()))
  return (
    <div className="flex w-full min-w-0 items-center gap-2 sm:w-80">
      <span className="shrink-0 text-sm text-soft">{label}</span>
      <div className="flex min-w-0 flex-1 overflow-hidden rounded-md border border-[var(--border)] bg-[var(--panel-strong)] focus-within:border-[var(--accent)]">
        <input className="h-9 w-24 min-w-0 border-r border-[var(--border)] bg-transparent px-3 text-sm outline-none focus-visible:bg-[var(--panel-muted)]" type="search" aria-label={`搜索${label}`} placeholder="搜索" value={search} onChange={e => setSearch(e.target.value)} />
        <select className="h-9 min-w-0 flex-1 bg-transparent px-2 text-sm focus-visible:outline-[var(--accent)]" aria-label={`筛选${label}`} value={value ?? ''} onChange={e => onChange(e.target.value ? Number(e.target.value) : undefined)}>
          <option value="">全部{label}</option>
          {value && !options.some(o => o.id === value) && <option value={value}>已删除{label} #{value}</option>}
          {shown.map(o => <option key={o.id} value={o.id}>{o.name}</option>)}
        </select>
      </div>
    </div>
  )
}

export function TrafficAnalyticsPanel({ onRefresh, refreshing = false }: { onRefresh: () => void; refreshing?: boolean }) {
  // Keep the selected range fixed until the administrator changes it or refreshes.
  const [range, setRange] = useState<DateSelection>(() => presetRange('7d'))
  const [dimension, setDimension] = useState<TrafficDimension>('node')
  const [direction, setDirection] = useState<TrafficDirection>('total')
  const [nodeId, setNodeId] = useState<number>()
  const [userId, setUserId] = useState<number>()
  const [page, setPage] = useState(1)
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
  const pages = Math.max(1, Math.ceil(ranking.length / PAGE_SIZE))
  const currentPage = Math.min(page, pages)

  function changeRange(value: DateSelection) {
    setRange(value)
    setPage(1)
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
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] px-4 py-3 md:px-5">
          <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm" aria-label="当前运行概况">
            <span className="font-semibold">运行概况</span>
            <span className="text-soft">节点 <b className="ml-1 text-[var(--text)]">{data?.total_nodes ?? '—'}</b></span>
            <span className="text-soft" title="当前健康的启用节点 / 全部启用节点">健康 <b className={`ml-1 ${data && data.healthy_nodes < data.enabled_nodes ? 'text-[var(--warning)]' : 'text-[var(--success)]'}`}>{data ? `${data.healthy_nodes} / ${data.enabled_nodes}` : '—'}</b></span>
            <span className="text-soft" title="当前启用用户 / 全部用户">启用用户 <b className="ml-1 text-[var(--text)]">{data ? `${data.enabled_users} / ${data.total_users}` : '—'}</b></span>
          </div>
          <Btn variant="secondary" className="h-9 shrink-0" onClick={refresh} disabled={isRefreshing}>
            <RefreshCcw className={`h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`} />
            {isRefreshing ? '刷新中…' : '刷新'}
          </Btn>
        </div>
        <div className="space-y-4 p-4 md:px-5">
          <DateRangePicker value={range} onChange={changeRange} />
          <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
            <ObjectFilter label="节点" value={nodeId} options={data?.nodes ?? []} onChange={id => { setNodeId(id); setPage(1) }} />
            <ObjectFilter label="用户" value={userId} options={data?.users ?? []} onChange={id => { setUserId(id); setPage(1) }} />
            {(nodeId || userId) && <button className="inline-flex items-center gap-1 rounded py-1 text-sm text-[var(--accent)]" onClick={() => { setNodeId(undefined); setUserId(undefined); setPage(1) }}><X className="h-4 w-4" />清除筛选</button>}
          </div>
        </div>
      </SurfaceCard>

      {query.isError ? (
        <SurfaceCard className="p-5">
          <p role="alert" className="mb-3 text-sm text-[var(--danger)]">统计加载失败：{query.error.message}</p>
          <Btn variant="secondary" onClick={() => void query.refetch()} disabled={query.isFetching}>重试</Btn>
        </SurfaceCard>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-4 xl:grid-cols-4" aria-busy={query.isFetching}>
            <SurfaceCard className="min-w-0 p-4 sm:p-5">
              <p className="text-sm text-soft">区间总用量</p>
              <p className="mt-3 break-words text-xl font-semibold tracking-tight tabular-nums sm:text-3xl">{formatBytes(total)}</p>
            </SurfaceCard>
            <SurfaceCard className="min-w-0 p-4 sm:p-5">
              <p className="text-sm text-soft">上行 / 下行</p>
              <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm font-semibold tabular-nums sm:text-lg">
                <span className="inline-flex items-center gap-1.5" aria-label={`上行 ${formatBytes(data?.up_bytes)}`}><ArrowUp className="h-4 w-4 shrink-0 text-[var(--accent)]" />{formatBytes(data?.up_bytes)}</span>
                <span className="inline-flex items-center gap-1.5" aria-label={`下行 ${formatBytes(data?.down_bytes)}`}><ArrowDown className="h-4 w-4 shrink-0 text-[var(--success)]" />{formatBytes(data?.down_bytes)}</span>
              </div>
            </SurfaceCard>
            {[
              ['区间活跃用户', data?.active_users ?? '—'],
              ['有用量节点', data?.active_nodes ?? '—'],
            ].map(([title, value]) => (
              <SurfaceCard key={title} className="min-w-0 p-4 sm:p-5">
                <p className="text-sm text-soft">{title}</p>
                <p className="mt-3 text-3xl font-semibold tracking-tight tabular-nums">{value}</p>
              </SurfaceCard>
            ))}
          </div>

          {data?.truncated && <p role="status" className="flex gap-2 text-sm text-[var(--warning)]"><AlertTriangle className="h-4 w-4 shrink-0" />所选范围超出明细保留期限，结果不完整。当前保留起点：{new Date(data.retention_start!).toLocaleString('zh-CN', { timeZone: TRAFFIC_TIMEZONE })}。</p>}

          <div className="grid items-start gap-4 2xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
            <SurfaceCard className="min-w-0 p-5">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <h3 className="font-semibold">用量趋势</h3>
                <div className="flex gap-2">
                  <select className={control} aria-label="统计维度" value={dimension} onChange={e => { setDimension(e.target.value as TrafficDimension); setPage(1) }}><option value="all">总览</option><option value="node">按节点</option><option value="user">按用户</option></select>
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

            <SurfaceCard className="min-w-0 overflow-hidden">
              <div className="flex min-h-18 items-center justify-between gap-3 border-b border-[var(--border)] px-5 py-4">
                <h3 className="font-semibold">{dimension === 'user' ? '用户用量排名' : dimension === 'node' ? '节点用量排名' : '用量汇总'}</h3>
                <span className="shrink-0 text-sm text-soft">{ranking.length} 项</span>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm">
                  <thead className="bg-[var(--panel-muted)] text-xs text-soft"><tr><th className="px-5 py-3">{dimension === 'user' ? '用户' : dimension === 'node' ? '节点' : '范围'}</th>{['上行', '下行', '总用量', '占比'].map(h => <th key={h} className="whitespace-nowrap px-3 py-3 text-right">{h}</th>)}</tr></thead>
                  <tbody>
                    {ranking.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE).map(row => (
                      <tr key={row.id} className="border-t border-[var(--border)]">
                        <td className="px-5 py-4">{dimension === 'all' ? row.name : <button className="block min-w-20 max-w-36 truncate text-left text-[var(--accent)] hover:underline" title={`${row.name} · ${dimension === 'node' ? '查看此节点的用户趋势' : '查看此用户的节点趋势'}`} onClick={() => { if (dimension === 'node') { setNodeId(row.id); setDimension('user') } else { setUserId(row.id); setDimension('node') }; setPage(1) }}>{row.name}</button>}</td>
                        <td className="whitespace-nowrap px-3 py-4 text-right tabular-nums">{formatBytes(row.up_bytes)}</td>
                        <td className="whitespace-nowrap px-3 py-4 text-right tabular-nums">{formatBytes(row.down_bytes)}</td>
                        <td className="whitespace-nowrap px-3 py-4 text-right font-medium tabular-nums">{formatBytes(row.up_bytes + row.down_bytes)}</td>
                        <td className="px-3 py-4 text-right tabular-nums">{total ? ((row.up_bytes + row.down_bytes) / total * 100).toFixed(1) : '0'}%</td>
                      </tr>
                    ))}
                    {!ranking.length && <tr><td colSpan={5} className="p-8 text-center text-soft">{query.isFetching ? '正在加载…' : '该范围暂无流量记录'}</td></tr>}
                  </tbody>
                </table>
              </div>
              {pages > 1 && <div className="flex items-center justify-end gap-3 border-t border-[var(--border)] px-4 py-3 text-sm text-soft"><Btn variant="ghost" className="h-8 px-2" disabled={currentPage <= 1} onClick={() => setPage(currentPage - 1)}>上一页</Btn>{currentPage} / {pages}<Btn variant="ghost" className="h-8 px-2" disabled={currentPage >= pages} onClick={() => setPage(currentPage + 1)}>下一页</Btn></div>}
            </SurfaceCard>
          </div>
        </>
      )}
    </div>
  )
}
