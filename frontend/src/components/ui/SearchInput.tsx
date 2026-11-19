import { useRef, type RefObject } from 'react'
import { Search, X } from 'lucide-react'
import { cn } from '@/lib/utils'

export function SearchInput({
  value, onChange, placeholder, label = placeholder, className, inputClassName, inputRef,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
  label?: string
  className?: string
  inputClassName?: string
  inputRef?: RefObject<HTMLInputElement | null>
}) {
  const localRef = useRef<HTMLInputElement>(null)
  const ref = inputRef ?? localRef
  return (
    <div className={cn('relative min-w-0', className)}>
      <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-soft" />
      <input
        ref={ref}
        aria-label={label}
        autoComplete="off"
        placeholder={placeholder}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className={cn('h-10 w-full rounded-md border border-[var(--border)] bg-[var(--panel-strong)] pl-10 pr-10 text-sm text-[var(--text)] placeholder:text-faint focus:border-[var(--accent)] focus:outline-none focus:ring-4 focus:ring-[var(--accent-ring)]', inputClassName)}
      />
      {value && (
        <button
          type="button"
          aria-label="清空搜索"
          title="清空搜索"
          onClick={() => {
            onChange('')
            ref.current?.focus()
          }}
          className="absolute right-1 top-1/2 inline-flex size-8 -translate-y-1/2 items-center justify-center rounded-md text-soft hover:bg-[var(--panel-muted)] hover:text-[var(--text)] focus-visible:outline-[var(--accent)]"
        >
          <X aria-hidden="true" className="size-4" />
        </button>
      )}
    </div>
  )
}
