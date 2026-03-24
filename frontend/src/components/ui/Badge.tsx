import { cn } from '@/lib/utils'

interface BadgeProps {
  label: string
  variant: 'green' | 'yellow' | 'red' | 'gray' | 'blue'
}

const variantClass: Record<BadgeProps['variant'], string> = {
  green:  'bg-green-100 text-green-700',
  yellow: 'bg-yellow-100 text-yellow-700',
  red:    'bg-red-100 text-red-700',
  gray:   'bg-gray-100 text-gray-600',
  blue:   'bg-blue-100 text-blue-700',
}

export function Badge({ label, variant }: BadgeProps) {
  return (
    <span className={cn('inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium', variantClass[variant])}>
      {label}
    </span>
  )
}
