import { useEffect, useRef, useState } from 'react'
import { formatBytes } from '@/lib/utils'
import { TRAFFIC_TIMEZONE } from '@/lib/trafficDates'
import type { TrafficDirection, TrafficPoint, TrafficSeries } from '@/types/traffic'

const colors = ['var(--accent)', 'var(--success)', 'var(--warning)', 'var(--danger)', 'var(--text-soft)', 'var(--text-faint)']
function pointValue(point: TrafficPoint, direction: TrafficDirection): number | null {
  if (point.up_bytes === null || point.down_bytes === null) return null
  return direction === 'up' ? point.up_bytes : direction === 'down' ? point.down_bytes : point.up_bytes + point.down_bytes
}
function timeLabel(value: string, hourly: boolean) {
  return new Date(value).toLocaleString('zh-CN', { timeZone: TRAFFIC_TIMEZONE, month: '2-digit', day: '2-digit', ...(hourly ? { hour: '2-digit', minute: '2-digit' } : {}) })
}
export function TrafficChart({ series, direction, interval }: { series: TrafficSeries[]; direction: TrafficDirection; interval: 'hour' | 'day' }) {
  const container = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(600)
  const [selected, setSelected] = useState<number | null>(null)
  const [hidden, setHidden] = useState<Set<string>>(new Set())
  useEffect(() => {
    const el = container.current
    if (!el) return
    const observer = new ResizeObserver(entries => setWidth(Math.max(260, entries[0].contentRect.width)))
    observer.observe(el)
    return () => observer.disconnect()
  }, [])
  const visible = series.filter(s => !hidden.has(s.id))
  const count = series[0]?.points.length ?? 0
  const left = 70, right = width - 18, top = 20, bottom = 230
  const max = Math.max(1, ...visible.flatMap(s => s.points.map(p => pointValue(p, direction) ?? 0))) * 1.1
  const x = (index: number) => count <= 1 ? (left + right) / 2 : left + index / (count - 1) * (right - left)
  const y = (value: number) => bottom - value / max * (bottom - top)
  const selectedIndex = selected === null ? null : Math.min(selected, Math.max(0, count - 1))
  const ticks = [...new Set(width < 500 ? [0, count - 1] : [0, Math.floor((count - 1) / 2), count - 1])].filter(i => i >= 0 && i < count)
  const hasValues = series.some(s => s.points.some(p => p.up_bytes !== null))
  return <div ref={container} className="min-w-0">
    <div className="relative">
      <svg width="100%" height="270" role="img" aria-label="所选区间的用量趋势，缺少采集依据的时段断开显示；可使用下方时间滑块查看数值。" onPointerMove={e => { const pos = e.clientX - e.currentTarget.getBoundingClientRect().left; setSelected(Math.max(0, Math.min(count - 1, Math.round((pos - left) / (right - left) * (count - 1))))) }} onPointerLeave={() => setSelected(null)}>
        {[0, 1, 2, 3].map(i => <g key={i}><line x1={left} x2={right} y1={y(max * i / 3)} y2={y(max * i / 3)} stroke="var(--border)" /><text x={left - 8} y={y(max * i / 3) + 4} textAnchor="end" fontSize="11" fill="var(--text-soft)">{formatBytes(max * i / 3)}</text></g>)}
        {ticks.map((i, n) => <text key={i} x={x(i)} y={255} textAnchor={n === 0 ? 'start' : n === ticks.length - 1 ? 'end' : 'middle'} fontSize="11" fill="var(--text-soft)">{timeLabel(series[0].points[i].time, interval === 'hour')}</text>)}
        {series.map((s, index) => {
          if (hidden.has(s.id)) return null
          let drawing = false
          const path = s.points.map((p, i) => { const v = pointValue(p, direction); if (v === null) { drawing = false; return '' }; const command = drawing ? 'L' : 'M'; drawing = true; return `${command}${x(i)},${y(v)}` }).join(' ')
          return <g key={s.id}><path d={path} fill="none" stroke={colors[index % colors.length]} strokeWidth="2" strokeDasharray={s.id === 'other' ? '5 4' : undefined} />{s.points.map((p, i) => { const v = pointValue(p, direction); return v === null ? null : <circle key={p.time} cx={x(i)} cy={y(v)} r={i === selectedIndex ? 4 : 2} fill={colors[index % colors.length]} /> })}</g>
        })}
        {selectedIndex !== null && count > 0 && <line x1={x(selectedIndex)} x2={x(selectedIndex)} y1={top} y2={bottom} stroke="var(--border-strong)" strokeDasharray="3 3" />}
      </svg>
      {!hasValues && <div className="pointer-events-none absolute inset-0 flex items-center justify-center text-sm text-soft">该范围暂无可用采集记录</div>}
      {selectedIndex !== null && count > 0 && (
        <div className="pointer-events-none absolute right-2 top-2 max-w-[calc(100%-1rem)] rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-xs shadow-[var(--shadow-panel)]" aria-live="polite">
          <p className="mb-2 font-medium">{timeLabel(series[0].points[selectedIndex].time, true)}</p>
          <div className="space-y-1.5">{visible.map(s => <div key={s.id} className="flex justify-between gap-4"><span className="min-w-0 break-all text-soft">{s.name}</span><span className="shrink-0 tabular-nums">{pointValue(s.points[selectedIndex], direction) === null ? '无采集依据' : formatBytes(pointValue(s.points[selectedIndex], direction))}</span></div>)}</div>
        </div>
      )}
    </div>
    <div className="flex flex-wrap gap-x-3 gap-y-1">{series.map((s, i) => <button key={s.id} className="flex min-w-0 items-center gap-2 rounded px-1 py-1 text-xs text-soft" title={`${hidden.has(s.id) ? '显示' : '隐藏'} ${s.name}`} aria-pressed={!hidden.has(s.id)} onClick={() => setHidden(prev => { const next = new Set(prev); if (next.has(s.id)) next.delete(s.id); else next.add(s.id); return next })}><span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: colors[i % colors.length], opacity: hidden.has(s.id) ? .25 : 1 }} /><span className={`break-all ${hidden.has(s.id) ? 'line-through' : ''}`}>{s.name}</span></button>)}</div>
    {count > 0 && <label className="mt-3 flex items-center gap-3 text-xs text-soft">时间点<input aria-label="查看趋势时间点" aria-valuetext={timeLabel(series[0].points[selectedIndex ?? 0].time, true)} type="range" min="0" max={count - 1} value={selectedIndex ?? 0} onChange={e => setSelected(Number(e.target.value))} className="min-w-0 flex-1 accent-[var(--accent)]" /></label>}
  </div>
}
