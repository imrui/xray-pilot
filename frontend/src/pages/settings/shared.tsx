import { useId } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { SurfaceCard } from '@/components/ui/Page'

export function Section({
  title,
  description,
  children,
  actions,
}: {
  title: string
  description?: React.ReactNode
  children: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <SurfaceCard className="p-4 sm:p-5">
      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
        <div className="min-w-0">
          <h3 className="text-base font-semibold">{title}</h3>
          {description && <p className="mt-1 text-sm leading-6 text-soft">{description}</p>}
        </div>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      <div className="space-y-4 [&:not(:empty)]:mt-4 [&_label]:text-sm">{children}</div>
    </SurfaceCard>
  )
}

export function SecretField({
  label,
  value,
  onChange,
  placeholder,
  revealed,
  onToggleReveal,
}: {
  label: string
  value: string
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void
  placeholder?: string
  revealed: boolean
  onToggleReveal: () => void
}) {
  const id = useId()
  return (
    <div className="flex flex-col space-y-1.5">
      <label htmlFor={id} className="text-sm font-medium text-soft">{label}</label>
      <div className="relative">
        <input
          id={id}
          type={revealed ? 'text' : 'password'}
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 pr-11 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
        />
        <button
          type="button"
          onClick={onToggleReveal}
          className="absolute right-2 top-1/2 inline-flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-faint transition hover:bg-[var(--panel-muted)] hover:text-[var(--text)]"
          aria-label={revealed ? `隐藏${label}` : `显示${label}`}
        >
          {revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
    </div>
  )
}
