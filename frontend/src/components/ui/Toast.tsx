import { X } from 'lucide-react'

type ToastVariant = 'success' | 'error' | 'warning'

const toastClass: Record<ToastVariant, string> = {
  success: 'border-emerald-500/35 bg-[var(--panel-strong)] text-[var(--text)]',
  error: 'border-rose-500/35 bg-[var(--panel-strong)] text-[var(--text)]',
  warning: 'border-amber-500/35 bg-[var(--panel-strong)] text-[var(--text)]',
}

export function Toast({
  title,
  description,
  variant,
  onClose,
}: {
  title: string
  description?: string
  variant: ToastVariant
  onClose: () => void
}) {
  return (
    <div className={`w-full max-w-sm rounded-lg border p-4 shadow-[var(--shadow-card)] ${toastClass[variant]}`}>
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="text-sm font-semibold">{title}</div>
          {description && <p className="mt-1 text-xs leading-5 text-soft">{description}</p>}
        </div>
        <button
          type="button"
          onClick={onClose}
          className="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-[var(--border)] bg-[var(--panel-muted)] text-soft transition hover:text-[var(--text)]"
          aria-label="关闭提示"
        >
          <X className="h-4 w-4" />
        </button>
      </div>
    </div>
  )
}
