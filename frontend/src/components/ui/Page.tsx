import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function PageShell({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('space-y-5', className)}>{children}</section>
}

export function SurfaceCard({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] shadow-[var(--shadow-card)]', className)}>{children}</div>
}
