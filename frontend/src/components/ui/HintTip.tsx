import type { ReactNode } from 'react'
import { Info } from 'lucide-react'
import { Tooltip } from '@/components/ui/Tooltip'

// 页面级说明的收纳方式：一个信息图标，悬停显示全文。
// 用于替代工具栏下方的整段提示文字，避免占用第一屏又显得密集。
export function HintTip({ content, side = 'bottom' }: { content: ReactNode; side?: 'top' | 'right' | 'bottom' | 'left' }) {
  return (
    <Tooltip content={content} side={side} className="max-w-[360px]">
      <button
        type="button"
        aria-label="说明"
        className="inline-flex h-9 w-9 items-center justify-center rounded-md text-[var(--warning)] transition hover:bg-[var(--warning-soft)]"
      >
        <Info className="h-4 w-4" />
      </button>
    </Tooltip>
  )
}
