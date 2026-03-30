import type { ReactNode } from 'react'

export function Tooltip({ content, children }: { content: ReactNode; children: ReactNode }) {
  return (
    <span className="group relative inline-flex">
      {children}
      <span className="pointer-events-none absolute left-1/2 top-full z-30 mt-2 hidden w-max max-w-[220px] -translate-x-1/2 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2.5 py-2 text-xs leading-5 text-[var(--text)] shadow-[var(--shadow-card)] group-hover:block group-focus-within:block">
        {content}
      </span>
    </span>
  )
}
