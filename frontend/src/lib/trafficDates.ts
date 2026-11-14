// All calendar arithmetic uses UTC+8 explicitly, independent of the browser's zone.
export const TRAFFIC_TIMEZONE = 'Asia/Taipei'
export const DAY = 86_400_000
const OFFSET = 8 * 3_600_000
export const DATE_SHORTCUTS = [
  ['today', '今天'], ['yesterday', '昨天'], ['24h', '近 24 小时'],
  ['3d', '近 3 天'], ['7d', '近 7 天'], ['30d', '近 30 天'], ['90d', '近 90 天'],
] as const
export type DatePreset = typeof DATE_SHORTCUTS[number][0] | 'custom'
export interface DateSelection { preset: DatePreset; start: string; end: string }
export function toDateInput(date: Date): string {
  return new Date(date.getTime() + OFFSET).toISOString().slice(0, 16)
}
export function presetRange(preset: Exclude<DatePreset, 'custom'>, now = new Date()): DateSelection {
  const midnight = Math.floor((now.getTime() + OFFSET) / DAY) * DAY - OFFSET
  let start = midnight, end = now.getTime()
  if (preset === '24h') start = end - DAY
  else if (preset === 'yesterday') { start = midnight - DAY; end = midnight }
  else if (preset !== 'today') start = midnight - (Number(preset.slice(0, -1)) - 1) * DAY
  return { preset, start: new Date(start).toISOString(), end: new Date(end).toISOString() }
}
export function customRange(start: string, end: string, now = new Date()): DateSelection {
  const a = new Date(start + ':00+08:00'), b = new Date(end + ':00+08:00')
  if (!Number.isFinite(a.getTime()) || !Number.isFinite(b.getTime()) || toDateInput(a) !== start || toDateInput(b) !== end) throw new Error('请选择有效的开始与结束时间')
  if (b > now) throw new Error('结束时间不能晚于当前时间')
  // Minute inputs are inclusive in the UI; the API uses an exclusive end.
  const until = Math.min(b.getTime() + 60_000, now.getTime())
  if (a.getTime() >= until) throw new Error('结束时间必须晚于开始时间')
  if (until - a.getTime() > 90 * DAY) throw new Error('单次查询范围不能超过 90 天')
  return { preset: 'custom', start: a.toISOString(), end: new Date(until).toISOString() }
}
export function rangeLabel(range: DateSelection): string {
  return `${toDateInput(new Date(range.start)).replace('T', ' ')} → ${toDateInput(new Date(new Date(range.end).getTime() - 1)).replace('T', ' ')}`
}
