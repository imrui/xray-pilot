import * as Popover from '@radix-ui/react-popover'
import { Info, X } from 'lucide-react'
import { TRAFFIC_TIMEZONE } from '@/lib/trafficDates'
import type { TrafficAnalytics } from '@/types/traffic'

export function TrafficStatsInfo({ data }: { data?: TrafficAnalytics }) {
  return (
    <div className="mt-4 border-t border-[var(--border)] pt-3 text-sm text-soft">
      <Popover.Root>
        <Popover.Trigger className="inline-flex min-h-9 cursor-pointer flex-wrap items-center gap-2 rounded-md px-2 text-left transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)] focus-visible:outline-2 focus-visible:outline-[var(--accent)]">
          <Info aria-hidden="true" className="h-4 w-4 shrink-0" />
          <span>统计说明</span>
          {data && data.failed_polls > 0 && <span className="text-[var(--warning)]">{data.failed_polls} 次采集失败</span>}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content side="top" align="start" sideOffset={8} collisionPadding={12}
            className="z-50 max-h-[min(28rem,var(--radix-popover-content-available-height))] w-[min(26rem,calc(100vw-1.5rem))] overflow-y-auto rounded-xl border border-[var(--border)] bg-[var(--panel-strong)] p-4 text-sm text-[var(--text)] shadow-xl focus-visible:outline-2 focus-visible:outline-[var(--accent)]">
            <div className="mb-3 flex items-center justify-between gap-3">
              <Popover.Title className="font-semibold">统计说明</Popover.Title>
              <Popover.Close aria-label="关闭统计说明" className="flex h-9 w-9 shrink-0 cursor-pointer items-center justify-center rounded-md text-soft transition-colors hover:bg-[var(--panel-muted)] hover:text-[var(--text)] focus-visible:outline-2 focus-visible:outline-[var(--accent)]">
                <X aria-hidden="true" className="h-4 w-4" />
              </Popover.Close>
            </div>
            <div className="space-y-3 leading-relaxed text-soft">
              <Popover.Description>按{data?.interval === 'hour' ? '小时' : '天'}汇总，展示总用量前 5 项及其他。点击图例可隐藏曲线。</Popover.Description>
              <p>北京时间（UTC+8），用量按采集时间计入区间。上行和下行以用户视角统计，活跃用户与节点指区间内实际产生流量的对象。</p>
              <p>缺少成功采集依据的时段保留断点，历史缺采无法补算；缺采期间的用量可能计入后续采集周期，统计不代表实时速率。</p>
              {data && <div className="space-y-2 rounded-lg bg-[var(--panel-muted)] p-3">
                <p>区间采集：成功 {data.successful_polls} 次，失败 {data.failed_polls} 次（按节点计）。</p>
                {data.last_poll_at && <p>最近成功采集：{new Date(data.last_poll_at).toLocaleString('zh-CN', { timeZone: TRAFFIC_TIMEZONE })}</p>}
              </div>}
            </div>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
    </div>
  )
}
