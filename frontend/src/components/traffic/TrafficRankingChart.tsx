import { useState } from 'react'
import { Btn } from '@/components/ui/Form'
import { formatBytes } from '@/lib/utils'
import { trafficName } from '@/types/traffic'
import type { TrafficAmount, TrafficDimension, TrafficDirection, TrafficOption } from '@/types/traffic'
import { useChartWidth } from './useChartWidth'

type Row = TrafficOption & TrafficAmount
const amount = (row: Row, direction: TrafficDirection) => direction === 'up' ? row.up_bytes : direction === 'down' ? row.down_bytes : row.up_bytes + row.down_bytes

function axisLabel(row: Row, slot: number) {
  const full = trafficName(row)
  const capacity = Math.max(4, Math.floor(slot / 12))
  if (full.length <= capacity) return full
  if (row.deleted && full !== row.name) return row.name.slice(0, Math.max(2, capacity - 3)) + '·已删除'
  return full.slice(0, capacity - 1) + '…'
}

export function TrafficRankingChart({ rows, dimension, direction, onSelect }: {
  rows: Row[]; dimension: TrafficDimension; direction: TrafficDirection; onSelect: (id: number) => void
}) {
  const { container, width } = useChartWidth()
  const [page, setPage] = useState(1)
  const [focused, setFocused] = useState<number | null>(null)
  const sorted = [...rows].sort((a, b) => amount(b, direction) - amount(a, direction) || a.id - b.id)
  const pageSize = Math.min(12, Math.max(3, Math.floor((width - 90) / 86)))
  const pages = Math.max(1, Math.ceil(sorted.length / pageSize))
  const current = Math.min(page, pages)
  const offset = (current - 1) * pageSize
  const shown = sorted.slice(offset, offset + pageSize)
  const left = 70, right = width - 16, top = 30, bottom = 270
  const max = Math.max(1, ...sorted.map(row => amount(row, direction))) * 1.15
  const total = sorted.reduce((sum, row) => sum + amount(row, direction), 0)
  const slot = (right - left) / Math.max(1, shown.length)
  const barWidth = Math.min(64, slot * .55)
  const x = (i: number) => left + (i + .5) * slot
  const y = (value: number) => bottom - value / max * (bottom - top)
  const selected = shown.find(row => row.id === focused)
  const label = dimension === 'user' ? '用户' : dimension === 'node' ? '节点' : '范围'
  const directionLabel = direction === 'up' ? '上行' : direction === 'down' ? '下行' : '总用量'
  const fullLabel = (row: Row) => `${trafficName(row)}，上行 ${formatBytes(row.up_bytes)}，下行 ${formatBytes(row.down_bytes)}，${directionLabel} ${formatBytes(amount(row, direction))}，占比 ${total ? (amount(row, direction) / total * 100).toFixed(1) : '0'}%`
  return <div ref={container} className="min-w-0">
    {!rows.length ? <p className="flex min-h-72 items-center justify-center text-sm text-soft">该范围暂无流量记录</p> : <>
      <div className="relative">
        <svg width="100%" height="335" role="group" aria-label={`${label}用量排名柱状图，纵轴为用量，横轴为${label}`}>
          {[0, 1, 2, 3].map(i => <g key={i}><line x1={left} x2={right} y1={y(max * i / 3)} y2={y(max * i / 3)} stroke="var(--border)" /><text x={left - 8} y={y(max * i / 3) + 4} textAnchor="end" fontSize="11" fill="var(--text-soft)">{formatBytes(max * i / 3)}</text></g>)}
          {shown.map((row, i) => <g key={row.id} role={dimension === 'all' ? 'img' : 'button'} tabIndex={0}
            aria-label={fullLabel(row) + (dimension === 'all' ? '' : `，查看此${label}的${dimension === 'node' ? '用户' : '节点'}趋势`)}
            className="cursor-pointer outline-none [&:focus-visible_.bar-focus]:stroke-[var(--accent)] [&:focus-visible_.bar-focus]:stroke-2"
            onPointerEnter={() => setFocused(row.id)} onPointerLeave={() => setFocused(null)} onFocus={() => setFocused(row.id)} onBlur={() => setFocused(null)}
            onClick={() => dimension !== 'all' && onSelect(row.id)} onKeyDown={e => { if (dimension !== 'all' && (e.key === 'Enter' || e.key === ' ')) { e.preventDefault(); onSelect(row.id) } }}>
            <title>{fullLabel(row)}</title>
            <rect className="bar-focus" x={left + i * slot + 2} y={top - 20} width={Math.max(1, slot - 4)} height="310" rx="5" fill={focused === row.id ? 'var(--panel-muted)' : 'transparent'} />
            {direction === 'total' ? <>
              <rect x={x(i) - barWidth / 2} y={y(row.down_bytes)} width={barWidth} height={bottom - y(row.down_bytes)} fill="var(--success)" />
              <rect x={x(i) - barWidth / 2} y={y(amount(row, direction))} width={barWidth} height={y(row.down_bytes) - y(amount(row, direction))} fill="var(--accent)" />
            </> : <rect x={x(i) - barWidth / 2} y={y(amount(row, direction))} width={barWidth} height={bottom - y(amount(row, direction))} fill={direction === 'up' ? 'var(--accent)' : 'var(--success)'} rx="3" />}
            <text x={x(i)} y={y(amount(row, direction)) - 9} textAnchor="middle" fontSize="11" fill="var(--text)">{formatBytes(amount(row, direction))}</text>
            <text x={x(i)} y="294" textAnchor="middle" fontSize="12" fill="var(--text-soft)">{axisLabel(row, slot)}</text>
            <text x={x(i)} y="313" textAnchor="middle" fontSize="10" fill="var(--text-faint)">第 {offset + i + 1} 名</text>
          </g>)}
        </svg>
        {selected && <div role="status" className="pointer-events-none absolute right-0 top-0 max-w-[min(320px,100%)] break-words rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] p-3 text-xs leading-6 shadow-[var(--shadow-panel)]">{fullLabel(selected)}</div>}
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-soft">
        <div className="flex flex-wrap gap-4">{direction !== 'down' && <span className="flex items-center gap-2"><i className="h-2.5 w-2.5 rounded-sm bg-[var(--accent)]" />上行</span>}{direction !== 'up' && <span className="flex items-center gap-2"><i className="h-2.5 w-2.5 rounded-sm bg-[var(--success)]" />下行</span>}<span>按{directionLabel}降序{dimension !== 'all' && ' · 点击柱形查看明细趋势'}</span></div>
        <div className="flex items-center gap-2"><span>{offset + 1}–{offset + shown.length} / {rows.length} 项</span>{pages > 1 && <><Btn variant="ghost" className="h-8 px-2" aria-label="排名上一页" disabled={current === 1} onClick={() => { setPage(current - 1); setFocused(null) }}>上一页</Btn><Btn variant="ghost" className="h-8 px-2" aria-label="排名下一页" disabled={current === pages} onClick={() => { setPage(current + 1); setFocused(null) }}>下一页</Btn></>}</div>
      </div>
    </>}
  </div>
}
