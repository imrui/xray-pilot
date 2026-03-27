import { type ReactNode } from 'react'

export interface Column<T> {
  key: string
  label: string
  render?: (row: T) => ReactNode
}

interface TableProps<T> {
  columns: Column<T>[]
  data: T[]
  loading?: boolean
  keyField?: keyof T
}

export function Table<T extends { id?: unknown }>({
  columns,
  data,
  loading,
  keyField = 'id' as keyof T,
}: TableProps<T>) {
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] py-16 shadow-[var(--shadow-card)]">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-[var(--accent-soft)] border-t-[var(--accent)]" />
        <span className="text-sm font-medium text-soft">Loading data...</span>
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] py-16 shadow-[var(--shadow-card)]">
        <span className="text-sm font-medium text-soft">No records found.</span>
      </div>
    )
  }

  return (
    <div className="w-full">
      <div className="space-y-3 md:hidden">
        {data.map((row, i) => (
          <div key={String(row[keyField] ?? i)} className="rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] p-4 shadow-[var(--shadow-card)]">
            <div className="flex flex-col gap-3">
              {columns.map((col) => (
                <div key={col.key} className="flex items-start justify-between gap-4">
                  <span className="mt-0.5 shrink-0 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">{col.label}</span>
                  <div className="text-right text-sm font-medium text-[var(--text)] text-balance">
                    {col.render ? col.render(row) : String((row as Record<string, unknown>)[col.key] ?? '')}
                  </div>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <div className="hidden overflow-x-auto rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] shadow-[var(--shadow-card)] md:block">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-[var(--border)] bg-[var(--panel-muted)]">
              {columns.map((col) => (
                <th key={col.key} className="whitespace-nowrap px-4 py-3 text-[11px] font-medium uppercase tracking-[0.12em] text-faint">
                  {col.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--border)] transition-colors">
            {data.map((row, i) => (
              <tr key={String(row[keyField] ?? i)} className="group transition-colors hover:bg-[var(--panel-muted)]/80">
                {columns.map((col) => (
                  <td key={col.key} className="px-4 py-3.5 whitespace-nowrap text-sm font-medium text-[var(--text)] lg:whitespace-normal">
                    {col.render ? col.render(row) : String((row as Record<string, unknown>)[col.key] ?? '')}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

interface PaginationProps {
  page: number
  pageSize: number
  total: number
  onChange: (page: number) => void
}

export function Pagination({ page, pageSize, total, onChange }: PaginationProps) {
  const totalPages = Math.ceil(total / pageSize)
  if (totalPages <= 1) return null

  return (
    <div className="mt-3 flex flex-col items-center justify-between gap-4 px-1 py-3 sm:flex-row">
      <span className="text-[13px] font-medium text-soft">
        总计 <span className="font-semibold text-[var(--text)]">{total}</span> 条记录
      </span>
      <div className="flex items-center gap-2">
        <button
          disabled={page <= 1}
          onClick={() => onChange(page - 1)}
          className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-[13px] font-medium text-[var(--text)] transition-colors hover:bg-[var(--panel-muted)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          上一页
        </button>
        <div className="flex items-center gap-1 rounded-md border border-[var(--border)] bg-[var(--panel-muted)] px-3 py-2 text-[13px] font-medium">
          <span className="text-[var(--text)]">{page}</span>
          <span className="text-faint">/</span>
          <span className="text-soft">{totalPages}</span>
        </div>
        <button
          disabled={page >= totalPages}
          onClick={() => onChange(page + 1)}
          className="rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-3 py-2 text-[13px] font-medium text-[var(--text)] transition-colors hover:bg-[var(--panel-muted)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          下一页
        </button>
      </div>
    </div>
  )
}
