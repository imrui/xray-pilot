import type { ReactNode } from 'react'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'

export function Tooltip({
  content,
  children,
  side = 'bottom',
  className = '',
}: {
  content: ReactNode
  children: ReactNode
  side?: 'top' | 'right' | 'bottom' | 'left'
  className?: string
}) {
  return (
    <TooltipPrimitive.Provider delayDuration={120}>
      <TooltipPrimitive.Root>
        <TooltipPrimitive.Trigger asChild>
          <span className="inline-flex">{children}</span>
        </TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content
            side={side}
            sideOffset={8}
            collisionPadding={12}
            className={`z-50 w-max max-w-[280px] rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 py-2 text-xs leading-5 text-[var(--text)] shadow-[var(--shadow-card)] ${className}`}
          >
            {content}
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  )
}
