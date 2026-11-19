import { useEffect, type ReactNode } from 'react'
import { Btn } from './Form'

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
  labelClassName?: string
}

export function Table<T extends { id?: unknown }>({
  columns,
  data,
  loading,
  keyField = 'id' as keyof T,
  labelClassName = 'text-[11px] uppercase tracking-[0.12em] text-faint',
}: TableProps<T>) {
  if (loading) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] py-16 shadow-[var(--shadow-card)]">
        <div className="h-6 w-6 animate-spin rounded-full border-2 border-[var(--accent-soft)] border-t-[var(--accent)]" />
        <span className="text-sm font-medium text-soft">正在加载…</span>
      </div>
    )
  }

  if (data.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-[var(--border)] bg-[var(--panel-strong)] py-16 shadow-[var(--shadow-card)]">
        <span className="text-sm font-medium text-soft">暂无记录</span>
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
                  <span className={`mt-0.5 shrink-0 font-medium ${labelClassName}`}>{col.label}</span>
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
                <th key={col.key} className={`whitespace-nowrap px-4 py-3 font-medium ${labelClassName}`}>
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
  total?: number
  onChange: (page: number) => void
  onPageSizeChange: (size: number) => void
}

export function Pagination({ page, pageSize, total, onChange, onPageSizeChange }: PaginationProps) {
  const totalPages = total === undefined ? undefined : Math.max(1, Math.ceil(total / pageSize))
  useEffect(() => {
    if (totalPages !== undefined && page > totalPages) onChange(totalPages)
  }, [page, totalPages, onChange])

  return (
    <nav aria-label="列表分页" className="flex flex-wrap items-center justify-between gap-3 text-sm text-soft">
      <span>共 {total ?? '…'} 条</span>
      <div className="flex flex-wrap items-center gap-2">
        <select
          aria-label="每页条数"
          value={pageSize}
          onChange={(event) => {
            onPageSizeChange(Number(event.target.value))
            onChange(1)
          }}
          className="h-9 rounded-md border border-[var(--border)] bg-[var(--panel-strong)] px-2 text-[var(--text)] focus:border-[var(--accent)] focus:outline-none"
        >
          {[10, 20, 50, 100].map(size => <option key={size} value={size}>{size} / 页</option>)}
        </select>
        <Btn variant="secondary" className="h-9 px-3" disabled={page <= 1} onClick={() => onChange(page - 1)}>
          上一页
        </Btn>
        <span aria-label={`第 ${page} 页`} className="whitespace-nowrap tabular-nums">{page} / {totalPages ?? '…'}</span>
        <Btn
          variant="secondary"
          className="h-9 px-3"
          disabled={totalPages === undefined || page >= totalPages}
          onClick={() => onChange(page + 1)}
        >
          下一页
        </Btn>
      </div>
    </nav>
  )
}
