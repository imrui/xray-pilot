import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function PageShell({ children, className }: { children: ReactNode; className?: string }) {
  return <section className={cn('space-y-6', className)}>{children}</section>
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
    <div className="panel rounded-[28px] p-5 md:p-6">
      <div className="flex flex-col gap-5 lg:flex-row lg:items-start lg:justify-between">
        <div className="space-y-2">
          <div className="inline-flex items-center rounded-full border border-[var(--border-strong)] bg-[var(--panel-muted)] px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.24em] text-faint">
            Operations Console
          </div>
          <div>
            <h1 className="text-2xl font-semibold tracking-[-0.04em] md:text-[30px]">{title}</h1>
            <p className="mt-2 max-w-3xl text-sm leading-6 text-soft">{description}</p>
          </div>
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2.5">{actions}</div>}
      </div>
      {stats && stats.length > 0 && (
        <div className="mt-5 grid gap-3 md:grid-cols-3">
          {stats.map((stat) => (
            <div key={stat.label} className="panel-muted rounded-2xl px-4 py-3">
              <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-faint">{stat.label}</div>
              <div className="mt-2 text-lg font-semibold tracking-[-0.03em]">{stat.value}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export function SurfaceCard({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={cn('panel rounded-[24px] p-0', className)}>{children}</div>
}
