import { useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { CalendarDays, ChevronLeft, ChevronRight, X } from 'lucide-react'
import { Btn } from '@/components/ui/Form'
import { cn } from '@/lib/utils'
import { customRange, DATE_SHORTCUTS, DAY, presetRange, rangeLabel, toDateInput, type DatePreset, type DateSelection } from '@/lib/trafficDates'

export function DateRangePicker({ value, onChange }: { value: DateSelection; onChange: (range: DateSelection) => void }) {
  const [open, setOpen] = useState(false)
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [quick, setQuick] = useState<DatePreset>('custom')
  const [month, setMonth] = useState(new Date())
  const [pickingEnd, setPickingEnd] = useState(false)
  const [error, setError] = useState('')
  function load(range: DateSelection) {
    const from = toDateInput(new Date(range.start))
    setStart(from); setEnd(toDateInput(new Date(new Date(range.end).getTime() - 1)))
    setMonth(new Date(from.slice(0, 7) + '-01T00:00:00Z'))
    setQuick(range.preset); setPickingEnd(false); setError('')
  }
  function show() { load(value); setOpen(true) }
  function apply() {
    try {
      const range = quick === 'custom' ? customRange(start, end) : presetRange(quick)
      onChange(range); setOpen(false)
    } catch (e) { setError((e as Error).message) }
  }
  const today = toDateInput(new Date()).slice(0, 10)
  const control = 'rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-sm focus-visible:outline-2 focus-visible:outline-[var(--accent)]'
  return (
    <Dialog.Root open={open} onOpenChange={setOpen}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1" role="group" aria-label="时间范围">
          {(['24h', '7d', '30d'] as const).map((key) => <button key={key} className={cn(control, value.preset === key && 'border-[var(--accent)] bg-[var(--accent-soft)] text-[var(--accent)]')} aria-pressed={value.preset === key} onClick={() => onChange(presetRange(key))}>{DATE_SHORTCUTS.find(([p]) => p === key)![1]}</button>)}
          <button className={cn(control, !['24h', '7d', '30d'].includes(value.preset) && 'bg-[var(--accent-soft)] text-[var(--accent)]')} aria-pressed={!['24h', '7d', '30d'].includes(value.preset)} onClick={show}>自定义</button>
        </div>
        <button className={cn(control, 'flex min-w-0 items-center gap-2 text-left')} onClick={show} aria-label="选择日期范围" aria-expanded={open}><CalendarDays className="h-4 w-4 shrink-0" /><span className="break-words">{rangeLabel(value)}</span></button>
      </div>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-50 bg-[var(--overlay)]" />
        <Dialog.Content className="fixed left-1/2 top-1/2 z-50 max-h-[90vh] w-[calc(100%-2rem)] max-w-4xl -translate-x-1/2 -translate-y-1/2 overflow-auto rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] p-5 shadow-xl">
          <div className="mb-4 flex items-center justify-between"><Dialog.Title className="font-semibold">选择日期范围</Dialog.Title><Dialog.Close className="rounded p-2 hover:bg-[var(--panel-muted)]" aria-label="关闭日期选择"><X className="h-4 w-4" /></Dialog.Close></div>
          <Dialog.Description className="mb-4 text-xs text-soft">台北时间（UTC+8）。先选开始日期，再选结束日期；点击应用后更新统计，单次最多 90 天。</Dialog.Description>
          <div className="grid gap-5 md:grid-cols-[120px_minmax(0,1fr)]">
            <div className="flex flex-wrap gap-1 md:flex-col md:border-r md:border-[var(--border)] md:pr-4" role="group" aria-label="日期快捷选择">
              {DATE_SHORTCUTS.map(([key, label]) => <button key={key} aria-pressed={quick === key} className={cn('rounded-md px-3 py-2 text-left text-sm hover:bg-[var(--panel-muted)]', quick === key && 'bg-[var(--accent-soft)] text-[var(--accent)]')} onClick={() => load(presetRange(key))}>{label}</button>)}
            </div>
            <div className="min-w-0">
              <div className="flex justify-between"><button className="rounded p-2 hover:bg-[var(--panel-muted)]" aria-label="上个月" onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() - 1, 1)))}><ChevronLeft className="h-4 w-4" /></button><button className="rounded p-2 hover:bg-[var(--panel-muted)]" aria-label="下个月" onClick={() => setMonth(new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() + 1, 1)))}><ChevronRight className="h-4 w-4" /></button></div>
              <div className="grid gap-5 sm:grid-cols-2">
                {[0, 1].map(offset => {
                  const first = new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth() + offset, 1))
                  const count = new Date(Date.UTC(first.getUTCFullYear(), first.getUTCMonth() + 1, 0)).getUTCDate()
                  const skip = (first.getUTCDay() + 6) % 7
                  return <div key={offset} className={offset === 1 ? 'hidden sm:block' : undefined}><div className="mb-3 text-center text-sm font-medium">{first.getUTCFullYear()} 年 {first.getUTCMonth() + 1} 月</div><div className="grid grid-cols-7 gap-1 text-center text-sm">
                    {['一', '二', '三', '四', '五', '六', '日'].map(day => <span key={day} className="py-2 text-xs text-soft">{day}</span>)}
                    {Array.from({ length: skip }, (_, i) => <span key={`blank${i}`} />)}
                    {Array.from({ length: count }, (_, i) => {
                      const date = new Date(first.getTime() + i * DAY).toISOString().slice(0, 10)
                      const boundary = date === start.slice(0, 10) || date === end.slice(0, 10)
                      return <button key={date} aria-label={date} aria-pressed={boundary} disabled={date > today} className={cn('min-h-9 rounded py-2 hover:bg-[var(--accent-soft)] disabled:opacity-30', date >= start.slice(0, 10) && date <= end.slice(0, 10) && 'bg-[var(--accent-soft)]', boundary && 'bg-[var(--accent)] text-[var(--accent-contrast)]')} onClick={() => {
                        setQuick('custom'); setError('')
                        const until = date === today ? toDateInput(new Date()) : `${date}T23:59`
                        if (!pickingEnd || date < start.slice(0, 10)) { setStart(`${date}T00:00`); setEnd(until); setPickingEnd(true) }
                        else { setEnd(until); setPickingEnd(false) }
                      }}>{i + 1}</button>
                    })}
                  </div></div>
                })}
              </div>
              <div className="mt-4 grid gap-3 border-t border-[var(--border)] pt-4 sm:grid-cols-2">
                <label className="grid gap-1 text-xs text-soft">开始时间<input aria-label="开始时间" className={cn(control, 'min-w-0 w-full text-[var(--text)]')} type="datetime-local" value={start} max={toDateInput(new Date())} onChange={e => { setStart(e.target.value); setQuick('custom'); setError('') }} /></label>
                <label className="grid gap-1 text-xs text-soft">结束时间<input aria-label="结束时间" className={cn(control, 'min-w-0 w-full text-[var(--text)]')} type="datetime-local" value={end} max={toDateInput(new Date())} onChange={e => { setEnd(e.target.value); setQuick('custom'); setError('') }} /></label>
              </div>
              {error && <p role="alert" className="mt-3 text-sm text-[var(--danger)]">{error}</p>}
              <div className="mt-4 flex justify-end gap-2"><Btn variant="secondary" onClick={() => setOpen(false)}>取消</Btn><Btn onClick={apply}>应用</Btn></div>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
