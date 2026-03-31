import type { ReactNode } from 'react'

export function Tooltip({
  content,
  children,
  side = 'bottom',
  className = '',
}: {
  content: ReactNode
  children: ReactNode
  side?: 'bottom' | 'right'
  className?: string
}) {
  const placementClassName =
    side === 'right'
      ? 'left-full top-1/2 ml-3 -translate-y-1/2'
      : 'left-1/2 top-full mt-2 -translate-x-1/2'

  return (
    <span className="group relative inline-flex">
      {children}
      <span className={`pointer-events-none absolute z-30 hidden w-max max-w-[280px] rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 py-2 text-xs leading-5 text-[var(--text)] shadow-[var(--shadow-card)] group-hover:block group-focus-within:block ${placementClassName} ${className}`}>
        {content}
      </span>
    </span>
  )
}
