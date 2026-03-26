import { type InputHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

interface FieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: string
  error?: string
}

export function Field({ label, error, className, ...props }: FieldProps) {
  return (
    <div className="flex flex-col space-y-1.5">
      <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">{label}</label>
      <input
        className={cn(
          'w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 text-sm text-[var(--text)] placeholder:text-faint transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]',
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
      <label className="text-[12px] font-semibold uppercase tracking-[0.16em] text-faint">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="w-full rounded-2xl border bg-[var(--panel-muted)] px-4 py-3 text-sm text-[var(--text)] transition-all duration-200 focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-soft)]"
      >
        {placeholder && <option value="" disabled>{placeholder}</option>}
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
  primary: 'border border-[var(--accent)]/18 bg-[var(--accent)] text-white shadow-[0_10px_24px_rgba(45,111,105,0.14)] hover:bg-[var(--accent-strong)] focus-visible:ring-[var(--accent)]',
  secondary: 'border border-[var(--border-strong)] bg-[var(--panel-strong)] text-[var(--text)] hover:bg-[var(--panel-muted)] focus-visible:ring-slate-400',
  danger: 'border border-[var(--danger)]/14 bg-[var(--danger)] text-white shadow-[0_10px_24px_rgba(200,106,116,0.14)] hover:brightness-105 focus-visible:ring-[var(--danger)]',
  ghost: 'text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)] focus-visible:ring-slate-400',
}

export function Btn({ variant = 'primary', loading, children, className, ...props }: BtnProps) {
  return (
    <button
      className={cn(
        'inline-flex items-center justify-center gap-2 rounded-2xl px-4 py-3 text-sm font-semibold transition-all focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-0 disabled:pointer-events-none disabled:opacity-50',
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
    <section className={cn('rounded-[24px] border border-[var(--border)] bg-[var(--panel-muted)] p-4 md:p-5', className)}>
      <div className="mb-4">
        <h4 className="text-sm font-semibold tracking-[-0.02em]">{title}</h4>
        {description && <p className="mt-1 text-xs leading-5 text-soft">{description}</p>}
      </div>
      <div className="space-y-4">{children}</div>
    </section>
  )
}
