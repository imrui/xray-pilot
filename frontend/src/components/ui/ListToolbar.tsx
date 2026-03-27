import { Search, SlidersHorizontal, X } from 'lucide-react'
import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function ListToolbar({
  searchValue,
  searchPlaceholder,
  onSearchChange,
  filters,
  bulkBar,
  meta,
}: {
  searchValue: string
  searchPlaceholder: string
  onSearchChange: (value: string) => void
  filters?: ReactNode
  bulkBar?: ReactNode
  meta?: ReactNode
}) {
  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex flex-1 flex-col gap-3 sm:flex-row sm:items-center">
          <label className="relative block min-w-0 flex-1 sm:max-w-md">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-soft" />
            <input
              value={searchValue}
              onChange={(e) => onSearchChange(e.target.value)}
              placeholder={searchPlaceholder}
              className="h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] py-2.5 pl-10 pr-10 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]"
            />
            {searchValue && (
              <button
                type="button"
                onClick={() => onSearchChange('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-soft transition hover:text-[var(--text)]"
              >
                <X className="h-4 w-4" />
              </button>
            )}
          </label>
          {filters && (
            <div className="flex items-center gap-2 overflow-x-auto scrollbar-hide">
              <div className="inline-flex items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-2.5 py-2 text-xs font-medium text-faint">
                <SlidersHorizontal className="h-3.5 w-3.5" />
                筛选
              </div>
              {filters}
            </div>
          )}
        </div>
        {meta && <div className="text-sm text-soft">{meta}</div>}
      </div>
      {bulkBar}
    </div>
  )
}

export function FilterChip({
  active,
  children,
  onClick,
}: {
  active?: boolean
  children: ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'rounded-md border px-3 py-2 text-xs font-medium transition',
        active
          ? 'border-[var(--accent)]/20 bg-[var(--accent-soft)] text-[var(--accent)]'
          : 'border-[var(--border)] bg-[var(--panel-strong)] text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)]'
      )}
    >
      {children}
    </button>
  )
}

export function BulkBar({ children }: { children: ReactNode }) {
  return <div className="rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] p-3 shadow-[var(--shadow-card)]">{children}</div>
}
