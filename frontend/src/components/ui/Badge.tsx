import { cn } from '@/lib/utils'

interface BadgeProps {
  label: string
  variant: 'green' | 'yellow' | 'red' | 'gray' | 'blue'
}

const variantClass: Record<BadgeProps['variant'], string> = {
  green: 'bg-emerald-600/8 text-emerald-700 ring-1 ring-inset ring-emerald-600/12 dark:bg-emerald-400/10 dark:text-emerald-200 dark:ring-emerald-400/14',
  yellow: 'bg-amber-600/8 text-amber-700 ring-1 ring-inset ring-amber-600/12 dark:bg-amber-400/10 dark:text-amber-200 dark:ring-amber-400/14',
  red: 'bg-rose-600/8 text-rose-700 ring-1 ring-inset ring-rose-600/12 dark:bg-rose-400/10 dark:text-rose-200 dark:ring-rose-400/14',
  gray: 'bg-slate-500/8 text-slate-600 ring-1 ring-inset ring-slate-500/12 dark:bg-slate-400/8 dark:text-slate-200 dark:ring-slate-400/12',
  blue: 'bg-sky-600/8 text-sky-700 ring-1 ring-inset ring-sky-600/12 dark:bg-sky-400/10 dark:text-sky-200 dark:ring-sky-400/14',
}

export function Badge({ label, variant }: BadgeProps) {
  return (
    <span className={cn('inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.18em]', variantClass[variant])}>
      {label}
    </span>
  )
}
