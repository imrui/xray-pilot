import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function PageShell({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('space-y-5', className)}>{children}</section>
}

export function PageHeader({
  title,
  description,
  actions,
  stats,
}: {
  title: string
  description: ReactNode
  actions?: ReactNode
  stats?: { label: string; value: ReactNode }[]
}) {
  return (
    <div className="space-y-4">
      <h1 className="sr-only">{title}</h1>
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <p className="max-w-3xl text-sm leading-6 text-soft">{description}</p>
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>

      {stats && stats.length > 0 && (
        <div className="grid gap-3 md:grid-cols-3">
          {stats.map((stat) => (
            <div key={stat.label} className="rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] px-4 py-4 shadow-[var(--shadow-card)]">
              <div className="text-[11px] font-medium uppercase tracking-[0.14em] text-faint">{stat.label}</div>
              <div className="mt-2 text-2xl font-semibold tracking-[-0.04em]">{stat.value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function SurfaceCard({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] shadow-[var(--shadow-card)]', className)}>{children}</div>
}
