import { type CSSProperties } from 'react'
import { cn } from '@/lib/utils'

interface BadgeProps {
  label: string
  variant: 'green' | 'yellow' | 'red' | 'gray' | 'blue'
}

const variantStyle: Record<BadgeProps['variant'], CSSProperties> = {
  green: {
    backgroundColor: 'var(--badge-green-bg)',
    color: 'var(--badge-green-text)',
    borderColor: 'var(--badge-green-border)',
  },
  yellow: {
    backgroundColor: 'var(--badge-yellow-bg)',
    color: 'var(--badge-yellow-text)',
    borderColor: 'var(--badge-yellow-border)',
  },
  red: {
    backgroundColor: 'var(--badge-red-bg)',
    color: 'var(--badge-red-text)',
    borderColor: 'var(--badge-red-border)',
  },
  gray: {
    backgroundColor: 'var(--badge-gray-bg)',
    color: 'var(--badge-gray-text)',
    borderColor: 'var(--badge-gray-border)',
  },
  blue: {
    backgroundColor: 'var(--badge-blue-bg)',
    color: 'var(--badge-blue-text)',
    borderColor: 'var(--badge-blue-border)',
  },
}

export function Badge({ label, variant }: BadgeProps) {
  return (
    <span
      className={cn('inline-flex min-h-6 shrink-0 items-center whitespace-nowrap rounded-md border px-2.5 py-1 text-xs font-semibold leading-none')}
      style={variantStyle[variant]}
    >
      {label}
    </span>
  )
}
