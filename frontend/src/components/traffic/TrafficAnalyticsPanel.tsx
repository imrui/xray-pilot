import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, RefreshCcw, X } from 'lucide-react'
import { trafficApi } from '@/lib/api'
import { formatBytes } from '@/lib/utils'
import { presetRange, type DateSelection } from '@/lib/trafficDates'
import type { TrafficDimension, TrafficDirection, TrafficOption } from '@/types/traffic'
import { SurfaceCard } from '@/components/ui/Page'
import { Btn } from '@/components/ui/Form'
import { DateRangePicker } from './DateRangePicker'
import { TrafficChart } from './TrafficChart'

function ObjectFilter({ label, options, value, onChange }: { label: string; options: TrafficOption[]; value: number | undefined; onChange: (id: number | undefined) => void }) {
  const [search, setSearch] = useState('')
  const shown = options.filter(o => o.id === value || o.name.toLowerCase().includes(search.toLowerCase()))
  return <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
    <label className="text-xs text-soft">{label}</label>
    <input className="h-9 w-28 rounded border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-sm" type="search" aria-label={`搜索${label}`} placeholder="搜索名称" value={search} onChange={e => setSearch(e.target.value)} />
    <select className="h-9 min-w-0 max-w-full flex-1 rounded border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-sm" aria-label={`筛选${label}`} value={value ?? ''} onChange={e => onChange(e.target.value ? Number(e.target.value) : undefined)}>
      <option value="">全部{label}</option>{value && !options.some(o => o.id === value) && <option value={value}>已删除{label} #{value}</option>}{shown.map(o => <option key={o.id} value={o.id}>{o.name}</option>)}
    </select>
  </div>
}

export function TrafficAnalyticsPanel() {
  const [selection, setSelection] = useState<DateSelection>(() => presetRange('7d'))
  const [clock, setClock] = useState(() => new Date())
  const [dimension, setDimension] = useState<TrafficDimension>('node')
  const [direction, setDirection] = useState<TrafficDirection>('total')
  const [nodeId, setNodeId] = useState<number>()
  const [userId, setUserId] = useState<number>()
  const [page, setPage] = useState(1)
  useEffect(() => { const timer = window.setInterval(() => setClock(new Date()), 60_000); return () => window.clearInterval(timer) }, [])
  const range = selection.preset === 'custom' ? selection : presetRange(selection.preset, clock)
  const query = useQuery({
    queryKey: ['traffic-analytics', range.start, range.end, dimension, nodeId, userId],
    queryFn: ({ signal }) => trafficApi.analytics({ start: range.start, end: range.end, dimension, node_id: nodeId, user_id: userId }, signal).then(r => r.data.data!),
    staleTime: 30_000,
  })
  const data = query.data
  const control = 'h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm'
  const total = data ? data.up_bytes + data.down_bytes : undefined
  const ranking = data?.ranking ?? []
  const pages = Math.max(1, Math.ceil(ranking.length / 10)), currentPage = Math.min(page, pages)
  function changeRange(value: DateSelection) { setSelection(value); setClock(new Date()); setPage(1) }
  return <div className="space-y-4">
    <SurfaceCard className="p-4">
      <div className="mb-4 flex flex-wrap gap-x-6 gap-y-2 text-sm" aria-label="当前运行概况">
        <span>当前节点 <b>{data?.total_nodes ?? '—'}</b></span><span>健康 <b>{data ? `${data.healthy_nodes} / ${data.total_nodes}` : '—'}</b></span><span>已启用用户 <b>{data?.enabled_users ?? '—'}</b> / {data?.total_users ?? '—'}</span><span className="text-xs text-soft">当前状态，不随时间筛选变化</span>
      </div>
      <DateRangePicker value={range} onChange={changeRange} />
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-soft"><span>台北时间（UTC+8）· 按采集时间归入统计区间</span><button className="inline-flex items-center gap-1 rounded p-1 hover:text-[var(--accent)]" onClick={() => { setClock(new Date()); if (selection.preset === 'custom' || selection.preset === 'yesterday') void query.refetch() }} disabled={query.isFetching}><RefreshCcw className={`h-3 w-3 ${query.isFetching ? 'animate-spin' : ''}`} />{query.isFetching ? '更新中…' : '刷新'}</button></div>
    </SurfaceCard>
    <SurfaceCard className="flex flex-wrap gap-4 p-4">
      <ObjectFilter label="节点" value={nodeId} options={data?.nodes ?? []} onChange={id => { setNodeId(id); setPage(1) }} />
      <ObjectFilter label="用户" value={userId} options={data?.users ?? []} onChange={id => { setUserId(id); setPage(1) }} />
      {(nodeId || userId) && <button className="inline-flex items-center gap-1 text-xs text-[var(--accent)]" onClick={() => { setNodeId(undefined); setUserId(undefined); setPage(1) }}><X className="h-3 w-3" />清除筛选</button>}
    </SurfaceCard>
    {query.isError ? <SurfaceCard className="p-5"><p role="alert" className="mb-3 text-sm text-[var(--danger)]">统计加载失败：{query.error.message}</p><Btn variant="secondary" onClick={() => void query.refetch()}>重试</Btn></SurfaceCard> : <>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-busy={query.isFetching}>
        {[
          ['区间总用量', formatBytes(total), '上行 + 下行'],
          ['上行 / 下行', data ? `${formatBytes(data.up_bytes)} / ${formatBytes(data.down_bytes)}` : '—', '用户视角 · 已采集记录合计'],
          ['区间活跃用户', data?.active_users ?? '—', '区间内实际产生流量的用户'],
          ['有用量节点', data?.active_nodes ?? '—', '区间内实际产生流量的节点'],
        ].map(([title, value, hint]) => <SurfaceCard key={title} className="p-5"><p className="text-sm text-soft">{title}</p><p className={`my-3 break-words font-semibold tabular-nums ${title === '上行 / 下行' ? 'text-lg' : 'text-2xl'}`}>{value}</p><p className="text-xs text-soft">{hint}</p></SurfaceCard>)}
      </div>
      {data?.truncated && <p role="status" className="flex gap-2 text-sm text-[var(--warning)]"><AlertTriangle className="h-4 w-4 shrink-0" />所选范围超出明细保留期限，结果不完整。当前保留起点：{new Date(data.retention_start!).toLocaleString('zh-CN', { timeZone: 'Asia/Taipei' })}。</p>}
      <SurfaceCard className="p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3"><div><h3 className="font-semibold">用量趋势</h3><p className="mt-1 text-xs text-soft">{data?.interval === 'hour' ? '按小时' : '按天'}汇总 · 按总用量排名 Top 5 + 其他</p></div><div className="flex gap-2"><select className={control} aria-label="统计维度" value={dimension} onChange={e => { setDimension(e.target.value as TrafficDimension); setPage(1) }}><option value="all">总览</option><option value="node">按节点</option><option value="user">按用户</option></select><select className={control} aria-label="流量方向" value={direction} onChange={e => setDirection(e.target.value as TrafficDirection)}><option value="total">总用量</option><option value="up">上行</option><option value="down">下行</option></select></div></div>
        {data ? <TrafficChart key={`${dimension}-${nodeId}-${userId}`} series={data.series} direction={direction} interval={data.interval} /> : <div className="flex h-64 items-center justify-center text-sm text-soft">正在加载趋势…</div>}
        <p className="mt-3 text-xs text-soft">{data ? `区间采集：成功 ${data.successful_polls} 次，失败 ${data.failed_polls} 次（按节点计）。` : ''}历史版本没有采集状态记录；缺采期间的用量可能延后归入后续采集周期，统计不代表实时速率。</p>
        {data?.last_poll_at && <p className="mt-1 text-xs text-soft">最近成功采集：{new Date(data.last_poll_at).toLocaleString('zh-CN', { timeZone: 'Asia/Taipei' })}</p>}
      </SurfaceCard>
      <SurfaceCard className="overflow-hidden">
        <div className="flex items-center justify-between border-b border-[var(--border)] p-5"><h3 className="font-semibold">{dimension === 'user' ? '用户用量排名' : dimension === 'node' ? '节点用量排名' : '用量汇总'}</h3><span className="text-xs text-soft">共 {ranking.length} 项 · 按总用量降序</span></div>
        <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead className="bg-[var(--panel-muted)] text-xs text-soft"><tr><th className="px-5 py-3">{dimension === 'user' ? '用户' : dimension === 'node' ? '节点' : '范围'}</th>{['上行', '下行', '总用量', '占比'].map(h => <th key={h} className="whitespace-nowrap px-4 py-3 text-right">{h}</th>)}</tr></thead><tbody>
          {ranking.slice((currentPage - 1) * 10, currentPage * 10).map(row => <tr key={row.id} className="border-t border-[var(--border)]"><td className="px-5 py-3">{dimension === 'all' ? row.name : <button className="text-left text-[var(--accent)] hover:underline" title={dimension === 'node' ? '查看此节点的用户趋势' : '查看此用户的节点趋势'} onClick={() => { if (dimension === 'node') { setNodeId(row.id); setDimension('user') } else { setUserId(row.id); setDimension('node') }; setPage(1) }}>{row.name}</button>}</td><td className="whitespace-nowrap px-4 py-3 text-right tabular-nums">{formatBytes(row.up_bytes)}</td><td className="whitespace-nowrap px-4 py-3 text-right tabular-nums">{formatBytes(row.down_bytes)}</td><td className="whitespace-nowrap px-4 py-3 text-right tabular-nums">{formatBytes(row.up_bytes + row.down_bytes)}</td><td className="px-4 py-3 text-right tabular-nums">{total ? ((row.up_bytes + row.down_bytes) / total * 100).toFixed(1) : '0'}%</td></tr>)}
          {!ranking.length && <tr><td colSpan={5} className="p-8 text-center text-soft">{query.isFetching ? '正在加载…' : '该范围暂无流量记录'}</td></tr>}
        </tbody></table></div>
        {pages > 1 && <div className="flex items-center justify-end gap-3 p-4 text-xs text-soft"><Btn variant="secondary" disabled={currentPage <= 1} onClick={() => setPage(currentPage - 1)}>上一页</Btn>{currentPage} / {pages}<Btn variant="secondary" disabled={currentPage >= pages} onClick={() => setPage(currentPage + 1)}>下一页</Btn></div>}
      </SurfaceCard>
    </>}
  </div>
}
