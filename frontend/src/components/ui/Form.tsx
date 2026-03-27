import { type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

interface FieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  error?: string
}

export function Field({ label, error, className, ...props }: FieldProps) {
  return (
    <div className="flex flex-col space-y-1.5">
      <label className="text-[12px] font-medium text-soft">{label}</label>
      <input
        className={cn(
          'h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]',
          error && 'border-red-400/40 focus:border-red-500 focus:ring-red-500/10',
          className
        )}
        {...props}
      />
      {error && <p className="mt-1 text-[11px] font-medium text-red-500">{error}</p>}
    </div>
  )
}

interface SelectFieldProps {
  label: string
  value: string | number
  onChange: (v: string) => void
  options: { value: string | number; label: string }[]
  placeholder?: string
}

export function SelectField({ label, value, onChange, options, placeholder }: SelectFieldProps) {
  return (
    <div className="flex flex-col space-y-1.5">
      <label className="text-[12px] font-medium text-soft">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 text-sm text-[var(--text)] transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
      >
        {placeholder && <option value="">{placeholder}</option>}
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  )
}

interface BtnProps {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost'
  loading?: boolean
  children: React.ReactNode
  onClick?: () => void
  type?: 'button' | 'submit'
  disabled?: boolean
  className?: string
}

const btnClass: Record<string, string> = {
  primary: 'border border-[var(--accent)] bg-[var(--accent)] text-white hover:bg-[var(--accent-strong)] focus-visible:ring-[var(--accent)]',
  secondary: 'border border-[var(--border-strong)] bg-[var(--panel-strong)] text-[var(--text)] hover:border-[var(--accent)]/40 hover:bg-[var(--panel-muted)] focus-visible:ring-slate-400',
  danger: 'border border-[var(--danger)] bg-[var(--danger-soft)] text-[var(--danger)] hover:bg-[var(--danger)] hover:text-white focus-visible:ring-[var(--danger)]',
  ghost: 'border border-transparent bg-transparent text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)] focus-visible:ring-slate-400',
}

export function Btn({ variant = 'primary', loading, children, className, ...props }: BtnProps) {
  return (
    <button
      className={cn(
        'inline-flex h-10 items-center justify-center gap-2 rounded-md px-4 text-sm font-medium transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-0 disabled:pointer-events-none disabled:opacity-50',
        btnClass[variant],
        className
      )}
      disabled={loading || props.disabled}
      {...props}
    >
      {loading && <span className="h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent" />}
      {children}
    </button>
  )
}

export function FieldGroup({
  title,
  description,
  children,
  className,
}: {
  title: string
  description?: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <section className={cn('rounded-lg border border-[var(--border)] bg-[var(--panel-muted)] p-4 md:p-5', className)}>
      <div className="mb-4">
        <h4 className="text-sm font-semibold tracking-[-0.02em]">{title}</h4>
        {description && <p className="mt-1 text-xs leading-5 text-soft">{description}</p>}
      </div>
      <div className="space-y-4">{children}</div>
    </section>
  )
}
