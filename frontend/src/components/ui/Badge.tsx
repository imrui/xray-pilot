import { cn } from '@/lib/utils'

interface BadgeProps {
  label: string
  variant: 'green' | 'yellow' | 'red' | 'gray' | 'blue'
}

const variantClass: Record<BadgeProps['variant'], string> = {
  green: 'bg-emerald-500/12 text-emerald-500 ring-1 ring-inset ring-emerald-500/16 dark:text-emerald-300',
  yellow: 'bg-amber-500/12 text-amber-500 ring-1 ring-inset ring-amber-500/16 dark:text-amber-300',
  red: 'bg-rose-500/12 text-rose-500 ring-1 ring-inset ring-rose-500/16 dark:text-rose-300',
  gray: 'bg-slate-500/10 text-slate-600 ring-1 ring-inset ring-slate-500/16 dark:text-slate-300',
  blue: 'bg-cyan-500/12 text-cyan-500 ring-1 ring-inset ring-cyan-500/16 dark:text-cyan-300',
}

export function Badge({ label, variant }: BadgeProps) {
  return (
    <span className={cn('inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em]', variantClass[variant])}>
      {label}
    </span>
  )
}
